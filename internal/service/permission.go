package service

import (
	"context"
	"database/sql"
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
	entTemp "github.com/whg517/sqlflow/internal/db/ent/temppolicy"
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

// sqliteAdapter implements persist.Adapter using database/sql for SQLite.
// RAW_SQL: casbin_rule has no ent schema — the Casbin adapter contract requires
// direct table access for LoadPolicy/SavePolicy/AddPolicy/RemovePolicy.
type sqliteAdapter struct {
	db *sql.DB
}

func newSQLiteAdapter(db *sql.DB) *sqliteAdapter {
	return &sqliteAdapter{db: db}
}

func (a *sqliteAdapter) loadPolicyData(ctx context.Context) ([]CasbinRule, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, ptype, v0, v1, v2, v3, v4, v5 FROM casbin_rule ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var rules []CasbinRule
	for rows.Next() {
		var r CasbinRule
		if err := rows.Scan(&r.ID, &r.PType, &r.V0, &r.V1, &r.V2, &r.V3, &r.V4, &r.V5); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (a *sqliteAdapter) LoadPolicy(model casbinModel.Model) error {
	rules, err := a.loadPolicyData(context.Background())
	if err != nil {
		return err
	}
	for _, r := range rules {
		line := r.PType
		parts := []string{r.V0, r.V1, r.V2, r.V3, r.V4, r.V5}
		for _, p := range parts {
			if p != "" {
				line += ", " + p
			}
		}
		_ = persist.LoadPolicyLine(line, model)
	}
	return nil
}

func (a *sqliteAdapter) SavePolicy(model casbinModel.Model) error {
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(context.Background(), `DELETE FROM casbin_rule`)
	if err != nil {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			slog.Warn("failed to rollback", "error", err)
		}
		return err
	}

	for ptype, ast := range model["p"] {
		for _, rule := range ast.Policy {
			_, err := tx.ExecContext(context.Background(),
				`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				ptype, getArg(rule, 0), getArg(rule, 1), getArg(rule, 2), getArg(rule, 3), getArg(rule, 4), getArg(rule, 5),
			)
			if err != nil {
				if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
					slog.Warn("failed to rollback", "error", err)
				}
				return err
			}
		}
	}

	for ptype, ast := range model["g"] {
		for _, rule := range ast.Policy {
			_, err := tx.ExecContext(context.Background(),
				`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				ptype, getArg(rule, 0), getArg(rule, 1), getArg(rule, 2), getArg(rule, 3), getArg(rule, 4), getArg(rule, 5),
			)
			if err != nil {
				if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
					slog.Warn("failed to rollback", "error", err)
				}
				return err
			}
		}
	}

	return tx.Commit()
}

func (a *sqliteAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ptype, getArg(rule, 0), getArg(rule, 1), getArg(rule, 2), getArg(rule, 3), getArg(rule, 4), getArg(rule, 5),
	)
	return err
}

func (a *sqliteAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	_, err := a.db.ExecContext(context.Background(),
		`DELETE FROM casbin_rule WHERE ptype=? AND v0=? AND v1=? AND v2=? AND v3=? AND v4=? AND v5=?`,
		ptype, getArg(rule, 0), getArg(rule, 1), getArg(rule, 2), getArg(rule, 3), getArg(rule, 4), getArg(rule, 5),
	)
	return err
}

func (a *sqliteAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	// Not needed for our use case; satisfy interface.
	return errors.New("RemoveFilteredPolicy not implemented")
}

func getArg(rule []string, idx int) string {
	if idx < len(rule) {
		return rule[idx]
	}
	return ""
}

// PermissionService handles permission management logic.
type PermissionService struct {
	database *db.DB
	enforcer *casbin.Enforcer
	adapter  *sqliteAdapter
}

// NewPermissionService creates a new PermissionService with a Casbin enforcer.
func NewPermissionService(database *db.DB) (*PermissionService, error) {
	adapter := newSQLiteAdapter(database.DB)

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

	svc := &PermissionService{
		database: database,
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

func (s *PermissionService) ensureBuiltinPlatformPolicies() error {
	_, err := s.enforcer.AddPolicy("dba", "system", "audit", "view")
	return err
}

// seedIfEmpty loads initial policies from policy.csv if casbin_rule table is empty.
// RAW_SQL: casbin_rule table — no ent schema, raw SQL required.
func (s *PermissionService) seedIfEmpty(ctx context.Context) error {
	var count int
	err := s.database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM casbin_rule`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
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
func (s *PermissionService) Enforce(sub, dom, obj, act string) (bool, error) {
	sub, dom, obj, act, err := authz.NormalizeTuple(sub, dom, obj, act)
	if err != nil {
		return false, err
	}
	return s.enforcer.Enforce(sub, dom, obj, act)
}

// EnforceActor evaluates both the actor's role policy and individual user
// policy. Temporary user policies are checked against their expiry at decision
// time, so an unavailable cleanup job can never extend a grant.
func (s *PermissionService) EnforceActor(ctx context.Context, userID int64, role, dom, obj, act string) (bool, error) {
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

	var expiresAt time.Time
	err = s.database.DB.QueryRowContext(ctx,
		`SELECT expires_at FROM temp_policies
		 WHERE sub = ? AND dom = ? AND obj = ? AND act = ?`,
		userSub, dom, obj, act,
	).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		// A non-temporary individual policy is a permanent explicit grant.
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("check temporary policy expiry: %w", err)
	}
	if !expiresAt.After(time.Now().UTC()) {
		_, _ = s.enforcer.RemovePolicy(userSub, dom, obj, act)
		return false, nil
	}
	return true, nil
}

// CanViewObject controls metadata visibility. A dedicated metadata:view grant
// can expose schema metadata without data access; select also implies that the
// object must be discoverable to query tooling.
func (s *PermissionService) CanViewObject(ctx context.Context, userID int64, role, dom, obj string) (bool, error) {
	allowed, err := s.EnforceActor(ctx, userID, role, dom, obj, "metadata:view")
	if err != nil || allowed {
		return allowed, err
	}
	return s.EnforceActor(ctx, userID, role, dom, obj, "select")
}

// LoadPolicy reloads policies from the database into memory.
func (s *PermissionService) LoadPolicy() error {
	return s.enforcer.LoadPolicy()
}

// SavePolicy saves in-memory policies to the database.
func (s *PermissionService) SavePolicy() error {
	return s.enforcer.SavePolicy()
}

// AddPolicy adds a new policy rule.
func (s *PermissionService) AddPolicy(sub, dom, obj, act string) error {
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
// RAW_SQL: casbin_rule table — no ent schema.
func (s *PermissionService) RemovePolicy(ctx context.Context, id int64) error {
	var r CasbinRule
	err := s.database.DB.QueryRowContext(ctx,
		`SELECT id, ptype, v0, v1, v2, v3, v4, v5 FROM casbin_rule WHERE id = ?`, id,
	).Scan(&r.ID, &r.PType, &r.V0, &r.V1, &r.V2, &r.V3, &r.V4, &r.V5)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("策略不存在")
		}
		return err
	}

	_, err = s.enforcer.RemovePolicy(r.V0, r.V1, r.V2, r.V3)
	return err
}

// GetPolicies returns all policy rules with pagination and optional filtering.
// RAW_SQL: casbin_rule table — no ent schema.
func (s *PermissionService) GetPolicies(ctx context.Context, page, pageSize int64, ptype, sub string) ([]Policy, int64, error) {
	p := ParsePagination(int(page), int(pageSize))

	var filters []FilterClause
	filters = append(filters, FilterClause{Condition: "ptype = 'p'"})
	if ptype != "" {
		filters = append(filters, FilterClause{Condition: "ptype = ?", Args: []interface{}{ptype}})
	}
	if sub != "" {
		filters = append(filters, FilterClause{Condition: "v0 = ?", Args: []interface{}{sub}})
	}

	whereClause, args := BuildWhereClause(filters)

	var total int64
	countSQL := PaginatedCountSQL("casbin_rule", whereClause)
	if err := s.database.DB.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	querySQL := fmt.Sprintf(
		`SELECT id, ptype, v0, v1, v2, v3 FROM casbin_rule %s ORDER BY id LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)
	rows, err := s.database.DB.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var policies []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.PType, &p.Sub, &p.Dom, &p.Obj, &p.Act); err != nil {
			return nil, 0, err
		}
		policies = append(policies, p)
	}
	return policies, total, rows.Err()
}

// GetPoliciesForRole returns all policies for a given role (v0 matches).
// RAW_SQL: casbin_rule table — no ent schema.
func (s *PermissionService) GetPoliciesForRole(ctx context.Context, role string) ([]Policy, error) {
	rows, err := s.database.DB.QueryContext(ctx,
		`SELECT id, ptype, v0, v1, v2, v3 FROM casbin_rule WHERE ptype = 'p' AND v0 = ? ORDER BY id`, role,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var policies []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.PType, &p.Sub, &p.Dom, &p.Obj, &p.Act); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// GetRoles returns all known roles (built-in list).
func (s *PermissionService) GetRoles() []string {
	return builtInRoles
}

// ListRoles returns persisted roles with membership and policy counts.
func (s *PermissionService) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.database.DB.QueryContext(ctx, `
		SELECT r.id, r.name, r.display_name, r.description, r.is_builtin, r.status,
		       r.created_at, r.updated_at,
		       (SELECT COUNT(*) FROM users u WHERE u.role = r.name) AS user_count,
		       (SELECT COUNT(*) FROM casbin_rule c WHERE c.ptype = 'p' AND c.v0 = r.name) AS policy_count
		FROM roles r
		ORDER BY r.is_builtin DESC, r.id`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	roles := make([]Role, 0)
	for rows.Next() {
		var role Role
		if err := rows.Scan(
			&role.ID, &role.Name, &role.DisplayName, &role.Description,
			&role.IsBuiltin, &role.Status, &role.CreatedAt, &role.UpdatedAt,
			&role.UserCount, &role.PolicyCount,
		); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, role)
	}
	for i := range roles {
		roles[i].Permissions, err = s.GetPlatformPermissions(roles[i].Name)
		if err != nil {
			return nil, err
		}
	}
	return roles, rows.Err()
}

// GetRole returns a persisted role with counts.
func (s *PermissionService) GetRole(ctx context.Context, name string) (*Role, error) {
	var role Role
	err := s.database.DB.QueryRowContext(ctx, `
		SELECT r.id, r.name, r.display_name, r.description, r.is_builtin, r.status,
		       r.created_at, r.updated_at,
		       (SELECT COUNT(*) FROM users u WHERE u.role = r.name),
		       (SELECT COUNT(*) FROM casbin_rule c WHERE c.ptype = 'p' AND c.v0 = r.name)
		FROM roles r WHERE r.name = ?`, name).
		Scan(
			&role.ID, &role.Name, &role.DisplayName, &role.Description,
			&role.IsBuiltin, &role.Status, &role.CreatedAt, &role.UpdatedAt,
			&role.UserCount, &role.PolicyCount,
		)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get role: %w", err)
	}
	role.Permissions, err = s.GetPlatformPermissions(role.Name)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// GetPlatformPermissions resolves effective platform permissions for a role.
func (s *PermissionService) GetPlatformPermissions(role string) ([]string, error) {
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
func (s *PermissionService) SetPlatformPermissions(ctx context.Context, roleName string, permissions []string) error {
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

	tx, err := s.database.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM casbin_rule WHERE ptype = 'p' AND v0 = ? AND v1 = 'system'`,
		roleName,
	); err != nil {
		return err
	}
	for _, key := range PlatformPermissionKeys() {
		if _, ok := seen[key]; !ok {
			continue
		}
		definition := platformPermissions[key]
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES ('p', ?, 'system', ?, ?)`,
			roleName, definition[0], definition[1],
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.enforcer.LoadPolicy()
}

// IsRoleActive checks whether a role exists and can be assigned or authenticated.
func (s *PermissionService) IsRoleActive(ctx context.Context, name string) (bool, error) {
	var status string
	err := s.database.DB.QueryRowContext(ctx, `SELECT status FROM roles WHERE name = ?`, name).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "active", nil
}

// CreateRole creates a custom role. Role names are immutable identifiers.
func (s *PermissionService) CreateRole(ctx context.Context, name, displayName, description string) (*Role, error) {
	name = strings.TrimSpace(name)
	displayName = strings.TrimSpace(displayName)
	if !roleNamePattern.MatchString(name) {
		return nil, ErrInvalidRoleName
	}
	if displayName == "" {
		return nil, errors.New("角色显示名称不能为空")
	}
	_, err := s.database.DB.ExecContext(ctx, `
		INSERT INTO roles (name, display_name, description, is_builtin, status)
		VALUES (?, ?, ?, 0, 'active')`, name, displayName, strings.TrimSpace(description))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrRoleExists
		}
		return nil, fmt.Errorf("create role: %w", err)
	}
	return s.GetRole(ctx, name)
}

// UpdateRole changes mutable role metadata and status.
func (s *PermissionService) UpdateRole(ctx context.Context, name, displayName, description, status string) (*Role, error) {
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
	result, err := s.database.DB.ExecContext(ctx, `
		UPDATE roles
		SET display_name = ?, description = ?, status = ?, updated_at = datetime('now')
		WHERE name = ?`, displayName, strings.TrimSpace(description), status, name)
	if err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, ErrRoleNotFound
	}
	return s.GetRole(ctx, name)
}

// DeleteRole removes an unused custom role and all of its policies atomically.
func (s *PermissionService) DeleteRole(ctx context.Context, name string) error {
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

	tx, err := s.database.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM casbin_rule WHERE v0 = ? AND ptype IN ('p', 'g')`, name); err != nil {
		return fmt.Errorf("delete role policies: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE name = ?`, name); err != nil {
		return fmt.Errorf("delete role: %w", err)
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
func (s *PermissionService) Enforcer() *casbin.Enforcer {
	return s.enforcer
}

// AddTemporaryPolicy adds a policy and tracks it with an expiry time for auto-cleanup.
func (s *PermissionService) AddTemporaryPolicy(ctx context.Context, sub, dom, obj, act string, expiresAt time.Time) error {
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

	_, err = s.database.Client().TempPolicy.Create().
		SetSub(sub).
		SetDom(dom).
		SetObj(obj).
		SetAct(act).
		SetExpiresAt(expiresAt).
		Save(ctx)
	return err
}

// RemoveTemporaryPolicy removes a policy and its tracking record.
func (s *PermissionService) RemoveTemporaryPolicy(ctx context.Context, sub, dom, obj, act string) error {
	sub, dom, obj, act, err := authz.NormalizeTuple(sub, dom, obj, act)
	if err != nil {
		return err
	}
	_, err = s.enforcer.RemovePolicy(sub, dom, obj, act)
	if err != nil {
		return err
	}
	_, _ = s.database.Client().TempPolicy.Delete().
		Where(
			entTemp.SubEQ(sub),
			entTemp.DomEQ(dom),
			entTemp.ObjEQ(obj),
			entTemp.ActEQ(act),
		).
		Exec(ctx)
	return nil
}

// PurgeExpiredPolicies removes all temporary policies past their expiry.
func (s *PermissionService) PurgeExpiredPolicies(ctx context.Context) (int64, error) {
	now := time.Now().UTC()

	expired, err := s.database.Client().TempPolicy.Query().
		Where(entTemp.ExpiresAtLTE(now)).
		All(ctx)
	if err != nil {
		return 0, err
	}

	var count int64
	for _, tp := range expired {
		_, _ = s.enforcer.RemovePolicy(tp.Sub, tp.Dom, tp.Obj, tp.Act)
		count++
	}

	deleted, err := s.database.Client().TempPolicy.Delete().
		Where(entTemp.ExpiresAtLTE(now)).
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

// entClient returns the ent client (for use by other methods in this package).
func (s *PermissionService) entClient() *ent.Client {
	return s.database.Client()
}
