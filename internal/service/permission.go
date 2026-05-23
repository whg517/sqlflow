package service

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
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
	ID    int64    `json:"id"`
	PType string   `json:"ptype"`
	Sub   string   `json:"sub"`
	Dom   string   `json:"dom"`
	Obj   string   `json:"obj"`
	Act   string   `json:"act"`
}

// RoleInfo represents a role with its associated policies.
type RoleInfo struct {
	Name     string   `json:"name"`
	Policies []Policy `json:"policies"`
}

var builtInRoles = []string{"admin", "dba", "developer"}

// sqliteAdapter implements persist.Adapter using database/sql for SQLite.
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
	defer rows.Close()

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

func (a *sqliteAdapter) LoadPolicy(model model.Model) error {
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
		persist.LoadPolicyLine(line, model)
	}
	return nil
}

func (a *sqliteAdapter) SavePolicy(model model.Model) error {
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(context.Background(), `DELETE FROM casbin_rule`)
	if err != nil {
		tx.Rollback()
		return err
	}

	for ptype, ast := range model["p"] {
		for _, rule := range ast.Policy {
			_, err := tx.ExecContext(context.Background(),
				`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				ptype, getArg(rule, 0), getArg(rule, 1), getArg(rule, 2), getArg(rule, 3), getArg(rule, 4), getArg(rule, 5),
			)
			if err != nil {
				tx.Rollback()
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
				tx.Rollback()
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
	db       *sql.DB
	enforcer *casbin.Enforcer
	adapter  *sqliteAdapter
}

// NewPermissionService creates a new PermissionService with a Casbin enforcer.
func NewPermissionService(db *sql.DB) (*PermissionService, error) {
	adapter := newSQLiteAdapter(db)

	// Load model from embedded FS
	modelData, err := fs.ReadFile(casbinModelFS, "casbin_model.conf")
	if err != nil {
		return nil, fmt.Errorf("read casbin model: %w", err)
	}

	m, err := model.NewModelFromString(string(modelData))
	if err != nil {
		return nil, fmt.Errorf("parse casbin model: %w", err)
	}

	enforcer, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer: %w", err)
	}

	svc := &PermissionService{
		db:       db,
		enforcer: enforcer,
		adapter:  adapter,
	}

	// Seed initial policies if table is empty
	if err := svc.seedIfEmpty(context.Background()); err != nil {
		return nil, fmt.Errorf("seed policies: %w", err)
	}

	return svc, nil
}

// seedIfEmpty loads initial policies from policy.csv if casbin_rule table is empty.
func (s *PermissionService) seedIfEmpty(ctx context.Context) error {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM casbin_rule`).Scan(&count)
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
		// e.g. "p, admin, *, *, *"
		if len(parts) < 2 {
			continue
		}
		ptype := parts[0]
		rule := parts[1:]
		if _, err := s.enforcer.AddPolicy(toInterfaceSlice(rule)...); err != nil {
			return fmt.Errorf("add seed policy %v: %w", rule, err)
		}
		_ = ptype // p is the default
	}
	return nil
}

// Enforce checks if a subject has permission to perform an action.
func (s *PermissionService) Enforce(sub, dom, obj, act string) (bool, error) {
	return s.enforcer.Enforce(sub, dom, obj, act)
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
func (s *PermissionService) RemovePolicy(ctx context.Context, id int64) error {
	var r CasbinRule
	err := s.db.QueryRowContext(ctx,
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
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	querySQL := fmt.Sprintf(
		`SELECT id, ptype, v0, v1, v2, v3 FROM casbin_rule %s ORDER BY id LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)
	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

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
func (s *PermissionService) GetPoliciesForRole(ctx context.Context, role string) ([]Policy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ptype, v0, v1, v2, v3 FROM casbin_rule WHERE ptype = 'p' AND v0 = ? ORDER BY id`, role,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

// Enforcer returns the underlying Casbin enforcer (for middleware use).
func (s *PermissionService) Enforcer() *casbin.Enforcer {
	return s.enforcer
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
