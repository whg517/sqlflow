package security

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	casbinModel "github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entcasbinrule "github.com/whg517/sqlflow/internal/db/ent/casbinrule"
	entrole "github.com/whg517/sqlflow/internal/db/ent/role"
	enttemppolicy "github.com/whg517/sqlflow/internal/db/ent/temppolicy"
	entuser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
)

//go:embed casbin_model.conf policy_seed.csv
var casbinModelFS embed.FS

// CasbinRule represents a row in the casbin_rule table.
type CasbinRule struct {
	ID    int64  `json:"id"`
	PType string `json:"ptype"`
	V0    string `json:"v0"`
	V1    string `json:"v1"`
	V2    string `json:"v2"`
	V3    string `json:"v3"`
	V4    string `json:"v4"`
	V5    string `json:"v5"`
}

// Policy represents a Casbin policy line for API responses.
type Policy struct {
	ID    int64  `json:"id"`
	PType string `json:"ptype"`
	Sub   string `json:"sub"`
	Dom   string `json:"dom"`
	Obj   string `json:"obj"`
	Act   string `json:"act"`
}

// RoleInfo represents a role with its associated policies.
type RoleInfo struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	IsBuiltin   bool      `json:"is_builtin"`
	Status      string    `json:"status"`
	UserCount   int64     `json:"user_count"`
	PolicyCount int64     `json:"policy_count"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Policies    []Policy  `json:"policies"`
}

var builtInRoles = []string{"admin", "dba", "developer"}

var roleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

var (
	ErrRoleNotFound    = errors.New("角色不存在")
	ErrRoleExists      = errors.New("角色名称已存在")
	ErrRoleInUse       = errors.New("角色仍有用户使用")
	ErrBuiltinRole     = errors.New("内置角色不能删除或禁用")
	ErrInvalidRoleName = errors.New("角色名称必须以小写字母开头，且只能包含小写字母、数字和下划线")
)

// Role is a persisted RBAC role definition.
type Role struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	IsBuiltin   bool      `json:"is_builtin"`
	Status      string    `json:"status"`
	UserCount   int64     `json:"user_count"`
	PolicyCount int64     `json:"policy_count"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var platformPermissions = map[string][2]string{
	"users:manage":       {"users", "manage"},
	"rbac:manage":        {"rbac", "manage"},
	"datasources:manage": {"datasources", "manage"},
	"security:manage":    {"security", "manage"},
	"audit:view":         {"audit", "view"},
	"settings:manage":    {"settings", "manage"},
}

// PlatformPermissionKeys returns the stable set of delegable platform permissions.
func PlatformPermissionKeys() []string {
	keys := make([]string, 0, len(platformPermissions))
	for key := range platformPermissions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// entAdapter implements persist.Adapter over the casbin_rule table.
//
// Casbin owns the shape of that table, so the adapter is the one place that
// reads and writes it. The name says ent rather than sqlite because the store
// changed; the previous name outlived the database it described.
type entAdapter struct {
	client *ent.Client
}

func newEntAdapter(client *ent.Client) *entAdapter {
	return &entAdapter{client: client}
}

func (a *entAdapter) loadPolicyData(ctx context.Context) ([]CasbinRule, error) {
	rows, err := a.client.CasbinRule.Query().
		Order(ent.Asc(entcasbinrule.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]CasbinRule, 0, len(rows))
	for _, r := range rows {
		rules = append(rules, CasbinRule{
			ID:    int64(r.ID),
			PType: r.Ptype,
			V0:    r.V0, V1: r.V1, V2: r.V2,
			V3: r.V3, V4: r.V4, V5: r.V5,
		})
	}
	return rules, nil
}

func (a *entAdapter) LoadPolicy(model casbinModel.Model) error {
	rules, err := a.loadPolicyData(context.Background())
	if err != nil {
		return err
	}
	for _, r := range rules {
		line := r.PType
		for _, p := range []string{r.V0, r.V1, r.V2, r.V3, r.V4, r.V5} {
			if p != "" {
				line += ", " + p
			}
		}
		_ = persist.LoadPolicyLine(line, model)
	}
	return nil
}

// SavePolicy replaces the whole policy set.
//
// In one transaction, because the delete and the reinsert must not be
// separately visible: a request arriving between them would be evaluated
// against an empty policy, which denies everything — or, for a rule that
// defaults open, allows it.
func (a *entAdapter) SavePolicy(model casbinModel.Model) error {
	ctx := context.Background()
	tx, err := a.client.Tx(ctx)
	if err != nil {
		return err
	}

	if _, err := tx.CasbinRule.Delete().Exec(ctx); err != nil {
		return rollbackPolicyTx(tx, err)
	}

	for _, section := range []string{"p", "g"} {
		for ptype, ast := range model[section] {
			for _, rule := range ast.Policy {
				if err := createPolicyRow(ctx, tx.CasbinRule, ptype, rule); err != nil {
					return rollbackPolicyTx(tx, err)
				}
			}
		}
	}

	return tx.Commit()
}

// rollbackPolicyTx unwinds a failed policy write, reporting the original cause.
func rollbackPolicyTx(tx *ent.Tx, cause error) error {
	if err := tx.Rollback(); err != nil {
		slog.Warn("failed to rollback", "error", err)
	}
	return cause
}

// createPolicyRow writes one Casbin rule.
//
// The six value columns are positional and Casbin supplies however many the
// rule uses, so the missing ones are stored as empty strings — which is what
// loadPolicyData skips when rebuilding the line.
func createPolicyRow(ctx context.Context, rules *ent.CasbinRuleClient, ptype string, rule []string) error {
	return rules.Create().
		SetPtype(ptype).
		SetV0(getArg(rule, 0)).
		SetV1(getArg(rule, 1)).
		SetV2(getArg(rule, 2)).
		SetV3(getArg(rule, 3)).
		SetV4(getArg(rule, 4)).
		SetV5(getArg(rule, 5)).
		Exec(ctx)
}

func (a *entAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	return createPolicyRow(context.Background(), a.client.CasbinRule, ptype, rule)
}

func (a *entAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	_, err := a.client.CasbinRule.Delete().
		Where(
			entcasbinrule.PtypeEQ(ptype),
			entcasbinrule.V0EQ(getArg(rule, 0)),
			entcasbinrule.V1EQ(getArg(rule, 1)),
			entcasbinrule.V2EQ(getArg(rule, 2)),
			entcasbinrule.V3EQ(getArg(rule, 3)),
			entcasbinrule.V4EQ(getArg(rule, 4)),
			entcasbinrule.V5EQ(getArg(rule, 5)),
		).
		Exec(context.Background())
	return err
}

func (a *entAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	// Not needed for our use case; satisfy interface.
	return errors.New("RemoveFilteredPolicy not implemented")
}

func getArg(rule []string, idx int) string {
	if idx < len(rule) {
		return rule[idx]
	}
	return ""
}

// Service handles permission management logic.
type Service struct {
	client   *ent.Client
	enforcer *casbin.Enforcer
	adapter  *entAdapter
}

// NewService creates a new Service with a Casbin enforcer.
func NewService(database *db.DB) (*Service, error) {
	adapter := newEntAdapter(database.Client())

	// Load model from embedded FS
	modelData, err := fs.ReadFile(casbinModelFS, "casbin_model.conf")
	if err != nil {
		return nil, fmt.Errorf("read casbin model: %w", err)
	}

	m, err := casbinModel.NewModelFromString(string(modelData))
	if err != nil {
		return nil, fmt.Errorf("parse casbin model: %w", err)
	}

	enforcer, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer: %w", err)
	}

	svc := &Service{
		client:   database.Client(),
		enforcer: enforcer,
		adapter:  adapter,
	}

	// Seed initial policies if table is empty
	if err := svc.seedIfEmpty(context.Background()); err != nil {
		return nil, fmt.Errorf("seed policies: %w", err)
	}
	if err := svc.ensureBuiltinPlatformPolicies(); err != nil {
		return nil, fmt.Errorf("seed platform policies: %w", err)
	}

	return svc, nil
}

func (s *Service) ensureBuiltinPlatformPolicies() error {
	_, err := s.enforcer.AddPolicy("dba", "system", "audit", "view")
	return err
}

// seedIfEmpty loads initial policies from policy.csv if casbin_rule table is empty.
func (s *Service) seedIfEmpty(ctx context.Context) error {
	seeded, err := s.client.CasbinRule.Query().Exist(ctx)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}

	// Read seed policies from embedded file
	data, err := fs.ReadFile(casbinModelFS, "policy_seed.csv")
	if err != nil {
		// Try direct file read as fallback (for development)
		seedPath := filepath.Join("internal", "pkg", "casbin", "policy.csv")
		data, err = osReadFile(seedPath)
		if err != nil {
			return fmt.Errorf("read seed policy: %w", err)
		}
	}

	lines := splitLines(string(data))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) < 5 {
			continue
		}
		ptype := parts[0]
		rule := parts[1:]
		switch {
		case strings.HasPrefix(ptype, "p"):
			sub, dom, obj, act, err := authz.NormalizeTuple(rule[0], rule[1], rule[2], rule[3])
			if err != nil {
				return fmt.Errorf("normalize seed policy %v: %w", rule, err)
			}
			if _, err := s.enforcer.AddNamedPolicy(ptype, sub, dom, obj, act); err != nil {
				return fmt.Errorf("add seed policy %v: %w", rule, err)
			}
		case strings.HasPrefix(ptype, "g"):
			if _, err := s.enforcer.AddNamedGroupingPolicy(ptype, toInterfaceSlice(rule)...); err != nil {
				return fmt.Errorf("add seed grouping policy %v: %w", rule, err)
			}
		default:
			return fmt.Errorf("unsupported seed policy type %q", ptype)
		}
	}
	return nil
}

// Enforce checks if a subject has permission to perform an action.
func (s *Service) Enforce(sub, dom, obj, act string) (bool, error) {
	sub, dom, obj, act, err := authz.NormalizeTuple(sub, dom, obj, act)
	if err != nil {
		return false, err
	}
	return s.enforcer.Enforce(sub, dom, obj, act)
}

// EnforceActor evaluates both the actor's role policy and individual user
// policy. Temporary user policies are checked against their expiry at decision
// time, so an unavailable cleanup job can never extend a grant.
func (s *Service) EnforceActor(ctx context.Context, userID int64, role, dom, obj, act string) (bool, error) {
	allowed, err := s.Enforce(role, dom, obj, act)
	if err != nil || allowed || userID <= 0 {
		return allowed, err
	}

	userSub := authz.UserSubject(userID)
	allowed, err = s.Enforce(userSub, dom, obj, act)
	if err != nil || !allowed {
		return allowed, err
	}

	userSub, dom, obj, act, err = authz.NormalizeTuple(userSub, dom, obj, act)
	if err != nil {
		return false, err
	}

	temp, err := s.client.TempPolicy.Query().
		Where(
			enttemppolicy.SubEQ(userSub),
			enttemppolicy.DomEQ(dom),
			enttemppolicy.ObjEQ(obj),
			enttemppolicy.ActEQ(act),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		// A non-temporary individual policy is a permanent explicit grant.
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("check temporary policy expiry: %w", err)
	}
	if !temp.ExpiresAt.After(time.Now().UTC()) {
		_, _ = s.enforcer.RemovePolicy(userSub, dom, obj, act)
		return false, nil
	}
	return true, nil
}

// CanViewObject controls metadata visibility. A dedicated metadata:view grant
// can expose schema metadata without data access; select also implies that the
// object must be discoverable to query tooling.
func (s *Service) CanViewObject(ctx context.Context, userID int64, role, dom, obj string) (bool, error) {
	allowed, err := s.EnforceActor(ctx, userID, role, dom, obj, "metadata:view")
	if err != nil || allowed {
		return allowed, err
	}
	return s.EnforceActor(ctx, userID, role, dom, obj, "select")
}

// LoadPolicy reloads policies from the database into memory.
func (s *Service) LoadPolicy() error {
	return s.enforcer.LoadPolicy()
}

// SavePolicy saves in-memory policies to the database.
func (s *Service) SavePolicy() error {
	return s.enforcer.SavePolicy()
}

// AddPolicy adds a new policy rule.
func (s *Service) AddPolicy(sub, dom, obj, act string) error {
	sub, dom, obj, act, err := authz.NormalizeTuple(sub, dom, obj, act)
	if err != nil {
		return err
	}
	added, err := s.enforcer.AddPolicy(sub, dom, obj, act)
	if err != nil {
		return err
	}
	if !added {
		return errors.New("策略已存在")
	}
	return nil
}

// RemovePolicy removes a policy rule by its database ID.
//
// The row is read first so the removal goes through the enforcer, which drops
// it from the in-memory model as well. Deleting the row directly would leave
// the policy in force until the next reload.
func (s *Service) RemovePolicy(ctx context.Context, id int64) error {
	r, err := s.client.CasbinRule.Get(ctx, int(id))
	if ent.IsNotFound(err) {
		return errors.New("策略不存在")
	}
	if err != nil {
		return err
	}
	_, err = s.enforcer.RemovePolicy(r.V0, r.V1, r.V2, r.V3)
	return err
}

// GetPolicies returns all policy rules with pagination and optional filtering.
// RAW_SQL: casbin_rule table — no ent schema.
func (s *Service) GetPolicies(ctx context.Context, page, pageSize int64, ptype, sub string) ([]Policy, int64, error) {
	p := sqlutil.ParsePagination(int(page), int(pageSize))

	// The caller's ptype replaces the default rather than being added to it.
	// Both conditions used to be applied, so asking for "g" produced
	// ptype = 'p' AND ptype = 'g' and always returned nothing.
	wanted := "p"
	if ptype != "" {
		wanted = ptype
	}
	q := s.client.CasbinRule.Query().Where(entcasbinrule.PtypeEQ(wanted))
	if sub != "" {
		q = q.Where(entcasbinrule.V0EQ(sub))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := q.
		Order(ent.Asc(entcasbinrule.FieldID)).
		Limit(p.PageSize).
		Offset(p.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return casbinRulesToPolicies(rows), int64(total), nil
}

// casbinRulesToPolicies maps stored rules onto the API shape.
//
// Casbin's columns are positional: v0..v3 are subject, domain, object and
// action for a "p" rule. The names live here so the callers do not each have to
// remember the order.
func casbinRulesToPolicies(rows []*ent.CasbinRule) []Policy {
	policies := make([]Policy, 0, len(rows))
	for _, r := range rows {
		policies = append(policies, Policy{
			ID:    int64(r.ID),
			PType: r.Ptype,
			Sub:   r.V0,
			Dom:   r.V1,
			Obj:   r.V2,
			Act:   r.V3,
		})
	}
	return policies
}

// GetPoliciesForRole returns all policies for a given role (v0 matches).
// RAW_SQL: casbin_rule table — no ent schema.
func (s *Service) GetPoliciesForRole(ctx context.Context, role string) ([]Policy, error) {
	rows, err := s.client.CasbinRule.Query().
		Where(entcasbinrule.PtypeEQ("p"), entcasbinrule.V0EQ(role)).
		Order(ent.Asc(entcasbinrule.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return casbinRulesToPolicies(rows), nil
}

// GetRoles returns all known roles (built-in list).
func (s *Service) GetRoles() []string {
	return builtInRoles
}

// ListRoles returns persisted roles with membership and policy counts.
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.client.Role.Query().
		Order(ent.Desc(entrole.FieldIsBuiltin), ent.Asc(entrole.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	roles := make([]Role, 0, len(rows))
	for _, r := range rows {
		role, err := s.roleWithCounts(ctx, r)
		if err != nil {
			return nil, err
		}
		roles = append(roles, *role)
	}
	return roles, nil
}

// roleWithCounts fills in the membership and policy counts a role is shown
// with.
//
// They were correlated subqueries in the listing statement. As separate counts
// they cost one query each, which is what makes the whole listing expressible
// in typed predicates — and the list is a handful of roles, not a page of
// tickets.
func (s *Service) roleWithCounts(ctx context.Context, r *ent.Role) (*Role, error) {
	userCount, err := s.client.User.Query().
		Where(entuser.RoleEQ(r.Name)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count role members: %w", err)
	}
	policyCount, err := s.client.CasbinRule.Query().
		Where(entcasbinrule.PtypeEQ("p"), entcasbinrule.V0EQ(r.Name)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count role policies: %w", err)
	}
	permissions, err := s.GetPlatformPermissions(r.Name)
	if err != nil {
		return nil, err
	}
	return &Role{
		ID:          int64(r.ID),
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		IsBuiltin:   r.IsBuiltin,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		UserCount:   int64(userCount),
		PolicyCount: int64(policyCount),
		Permissions: permissions,
	}, nil
}

// GetRole returns a persisted role with counts.
func (s *Service) GetRole(ctx context.Context, name string) (*Role, error) {
	r, err := s.client.Role.Query().
		Where(entrole.NameEQ(name)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get role: %w", err)
	}
	return s.roleWithCounts(ctx, r)
}

// GetPlatformPermissions resolves effective platform permissions for a role.
func (s *Service) GetPlatformPermissions(role string) ([]string, error) {
	result := make([]string, 0)
	for _, key := range PlatformPermissionKeys() {
		definition := platformPermissions[key]
		allowed, err := s.Enforce(role, "system", definition[0], definition[1])
		if err != nil {
			return nil, err
		}
		if allowed {
			result = append(result, key)
		}
	}
	return result, nil
}

// SetPlatformPermissions replaces a custom role's explicit system policies.
func (s *Service) SetPlatformPermissions(ctx context.Context, roleName string, permissions []string) error {
	role, err := s.GetRole(ctx, roleName)
	if err != nil {
		return err
	}
	if role.IsBuiltin {
		return ErrBuiltinRole
	}
	seen := make(map[string]struct{}, len(permissions))
	for _, key := range permissions {
		if _, ok := platformPermissions[key]; !ok {
			return fmt.Errorf("未知平台权限: %s", key)
		}
		seen[key] = struct{}{}
	}

	// One transaction: between the delete and the reinsert the role holds no
	// platform permissions at all, and a request evaluated in that window would
	// be denied outright.
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.CasbinRule.Delete().
		Where(
			entcasbinrule.PtypeEQ("p"),
			entcasbinrule.V0EQ(roleName),
			entcasbinrule.V1EQ("system"),
		).
		Exec(ctx); err != nil {
		return rollbackPolicyTx(tx, err)
	}
	for _, key := range PlatformPermissionKeys() {
		if _, ok := seen[key]; !ok {
			continue
		}
		definition := platformPermissions[key]
		if err := tx.CasbinRule.Create().
			SetPtype("p").
			SetV0(roleName).
			SetV1("system").
			SetV2(definition[0]).
			SetV3(definition[1]).
			Exec(ctx); err != nil {
			return rollbackPolicyTx(tx, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// The enforcer holds the policy in memory; without a reload the change is
	// on disk only and takes effect at the next restart.
	return s.enforcer.LoadPolicy()
}

// IsRoleActive checks whether a role exists and can be assigned or authenticated.
func (s *Service) IsRoleActive(ctx context.Context, name string) (bool, error) {
	status, err := s.client.Role.Query().
		Where(entrole.NameEQ(name)).
		Select(entrole.FieldStatus).
		String(ctx)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "active", nil
}

// CreateRole creates a custom role. Role names are immutable identifiers.
func (s *Service) CreateRole(ctx context.Context, name, displayName, description string) (*Role, error) {
	name = strings.TrimSpace(name)
	displayName = strings.TrimSpace(displayName)
	if !roleNamePattern.MatchString(name) {
		return nil, ErrInvalidRoleName
	}
	if displayName == "" {
		return nil, errors.New("角色显示名称不能为空")
	}
	err := s.client.Role.Create().
		SetName(name).
		SetDisplayName(displayName).
		SetDescription(strings.TrimSpace(description)).
		SetIsBuiltin(false).
		SetStatus("active").
		Exec(ctx)
	if err != nil {
		// ent classifies the constraint violation, replacing a substring match
		// on the driver's message text.
		if ent.IsConstraintError(err) {
			return nil, ErrRoleExists
		}
		return nil, fmt.Errorf("create role: %w", err)
	}
	return s.GetRole(ctx, name)
}

// UpdateRole changes mutable role metadata and status.
func (s *Service) UpdateRole(ctx context.Context, name, displayName, description, status string) (*Role, error) {
	role, err := s.GetRole(ctx, name)
	if err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, errors.New("角色显示名称不能为空")
	}
	if status != "active" && status != "disabled" {
		return nil, errors.New("角色状态必须是 active 或 disabled")
	}
	if role.IsBuiltin && status == "disabled" {
		return nil, ErrBuiltinRole
	}
	if status == "disabled" && role.UserCount > 0 {
		return nil, ErrRoleInUse
	}
	affected, err := s.client.Role.Update().
		Where(entrole.NameEQ(name)).
		SetDisplayName(displayName).
		SetDescription(strings.TrimSpace(description)).
		SetStatus(status).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}
	if affected == 0 {
		return nil, ErrRoleNotFound
	}
	return s.GetRole(ctx, name)
}

// DeleteRole removes an unused custom role and all of its policies atomically.
func (s *Service) DeleteRole(ctx context.Context, name string) error {
	role, err := s.GetRole(ctx, name)
	if err != nil {
		return err
	}
	if role.IsBuiltin {
		return ErrBuiltinRole
	}
	if role.UserCount > 0 {
		return ErrRoleInUse
	}

	// One transaction: a role removed while its policies survive would leave
	// grants addressed to a subject that no longer exists, which the enforcer
	// still matches.
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.CasbinRule.Delete().
		Where(
			entcasbinrule.V0EQ(name),
			entcasbinrule.PtypeIn("p", "g"),
		).
		Exec(ctx); err != nil {
		return rollbackPolicyTx(tx, fmt.Errorf("delete role policies: %w", err))
	}
	if _, err := tx.Role.Delete().
		Where(entrole.NameEQ(name)).
		Exec(ctx); err != nil {
		return rollbackPolicyTx(tx, fmt.Errorf("delete role: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := s.enforcer.LoadPolicy(); err != nil {
		return fmt.Errorf("reload policies: %w", err)
	}
	return nil
}

// Enforcer returns the underlying Casbin enforcer (for middleware use).
func (s *Service) Enforcer() *casbin.Enforcer {
	return s.enforcer
}

// AddTemporaryPolicy adds a policy and tracks it with an expiry time for auto-cleanup.
func (s *Service) AddTemporaryPolicy(ctx context.Context, sub, dom, obj, act string, expiresAt time.Time) error {
	sub, dom, obj, act, err := authz.NormalizeTuple(sub, dom, obj, act)
	if err != nil {
		return err
	}
	added, err := s.enforcer.AddPolicy(sub, dom, obj, act)
	if err != nil {
		return err
	}
	if !added {
		return nil // already exists
	}

	_, err = s.client.TempPolicy.Create().
		SetSub(sub).
		SetDom(dom).
		SetObj(obj).
		SetAct(act).
		SetExpiresAt(expiresAt).
		Save(ctx)
	return err
}

// RemoveTemporaryPolicy removes a policy and its tracking record.
func (s *Service) RemoveTemporaryPolicy(ctx context.Context, sub, dom, obj, act string) error {
	sub, dom, obj, act, err := authz.NormalizeTuple(sub, dom, obj, act)
	if err != nil {
		return err
	}
	_, err = s.enforcer.RemovePolicy(sub, dom, obj, act)
	if err != nil {
		return err
	}
	_, _ = s.client.TempPolicy.Delete().
		Where(
			enttemppolicy.SubEQ(sub),
			enttemppolicy.DomEQ(dom),
			enttemppolicy.ObjEQ(obj),
			enttemppolicy.ActEQ(act),
		).
		Exec(ctx)
	return nil
}

// PurgeExpiredPolicies removes all temporary policies past their expiry.
func (s *Service) PurgeExpiredPolicies(ctx context.Context) (int64, error) {
	now := time.Now().UTC()

	expired, err := s.client.TempPolicy.Query().
		Where(enttemppolicy.ExpiresAtLTE(now)).
		All(ctx)
	if err != nil {
		return 0, err
	}

	var count int64
	for _, tp := range expired {
		_, _ = s.enforcer.RemovePolicy(tp.Sub, tp.Dom, tp.Obj, tp.Act)
		count++
	}

	deleted, err := s.client.TempPolicy.Delete().
		Where(enttemppolicy.ExpiresAtLTE(now)).
		Exec(ctx)
	if err != nil {
		return count, err
	}
	_ = deleted
	return count, nil
}

// osReadFile reads a file from disk.
func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// splitLines splits text into lines.
func splitLines(text string) []string {
	return strings.Split(text, "\n")
}

// toInterfaceSlice converts []string to []interface{}.
func toInterfaceSlice(s []string) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}
