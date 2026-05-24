package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/pkg/mask"
)

var (
	// ErrMaskRuleNotFound indicates the mask rule does not exist.
	ErrMaskRuleNotFound = errors.New("脱敏规则不存在")
	// ErrMaskRuleFieldRequired indicates the field name is required.
	ErrMaskRuleFieldRequired = errors.New("字段名不能为空")
	// ErrMaskRuleTableRequired indicates the table name is required.
	ErrMaskRuleTableRequired = errors.New("表名不能为空")
	// ErrMaskRuleTypeInvalid indicates the mask type is invalid.
	ErrMaskRuleTypeInvalid = errors.New("无效的脱敏类型")
	// ErrMaskRuleCustomRegexRequired indicates custom type requires a regex.
	ErrMaskRuleCustomRegexRequired = errors.New("自定义脱敏类型必须提供正则表达式")
	// ErrMaskRuleDuplicate indicates a rule already exists for this field.
	ErrMaskRuleDuplicate = errors.New("该字段已存在脱敏规则")
	// ErrSensitiveTableNotFound indicates the sensitive table record does not exist.
	ErrSensitiveTableNotFound = errors.New("敏感表记录不存在")
	// ErrSensitiveTableDuplicate indicates the table is already marked as sensitive.
	ErrSensitiveTableDuplicate = errors.New("该表已标记为敏感表")
	// ErrSensitiveTableRequired indicates the table name is required.
	ErrSensitiveTableRequired = errors.New("表名不能为空")
	// ErrInvalidSensitivityLevel indicates an invalid sensitivity level.
	ErrInvalidSensitivityLevel = errors.New("无效的敏感等级，可选: low, medium, high")
)

var validSensitivityLevels = map[string]bool{"low": true, "medium": true, "high": true}

// MaskRuleService handles mask rule CRUD operations.
type MaskRuleService struct {
	db       *sql.DB
	permSvc  *PermissionService
	auditSvc *AuditService
}

// NewMaskRuleService creates a new MaskRuleService.
func NewMaskRuleService(db *sql.DB, permSvc *PermissionService, auditSvc *AuditService) *MaskRuleService {
	return &MaskRuleService{db: db, permSvc: permSvc, auditSvc: auditSvc}
}

// --- Mask Rules CRUD ---

// CreateMaskRule creates a new mask rule and records an audit log for the given userID.
func (s *MaskRuleService) CreateMaskRule(ctx context.Context, userID int64, datasourceID int64, database, tableName, field, maskType, customRegex, customTemplate string) (*model.MaskRule, error) {
	if strings.TrimSpace(tableName) == "" {
		return nil, ErrMaskRuleTableRequired
	}
	if strings.TrimSpace(field) == "" {
		return nil, ErrMaskRuleFieldRequired
	}
	if !mask.IsValidMaskType(maskType) {
		return nil, ErrMaskRuleTypeInvalid
	}
	if maskType == string(mask.MaskCustom) && strings.TrimSpace(customRegex) == "" {
		return nil, ErrMaskRuleCustomRegexRequired
	}

	// Check for duplicate rule
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mask_rules WHERE datasource_id = ? AND database = ? AND table_name = ? AND field = ?`,
		datasourceID, database, tableName, field,
	).Scan(&count)
	if err == nil && count > 0 {
		return nil, ErrMaskRuleDuplicate
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		datasourceID, database, tableName, field, maskType, customRegex, customTemplate, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("创建脱敏规则失败: %w", err)
	}

	id, _ := result.LastInsertId()
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "mask_rule_create",
		SQLContent: "mask_rule:" + tableName + "." + field,
		SQLSummary: "创建脱敏规则: " + tableName + "." + field,
	})
	return &model.MaskRule{
		ID:             id,
		DatasourceID:   datasourceID,
		Database:       database,
		TableName:      tableName,
		Field:          field,
		MaskType:       maskType,
		CustomRegex:    customRegex,
		CustomTemplate: customTemplate,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// GetMaskRule retrieves a mask rule by ID.
func (s *MaskRuleService) GetMaskRule(ctx context.Context, id int64) (*model.MaskRule, error) {
	r := &model.MaskRule{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at
		 FROM mask_rules WHERE id = ?`, id,
	).Scan(&r.ID, &r.DatasourceID, &r.Database, &r.TableName, &r.Field, &r.MaskType, &r.CustomRegex, &r.CustomTemplate, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMaskRuleNotFound
		}
		return nil, fmt.Errorf("获取脱敏规则失败: %w", err)
	}
	return r, nil
}

// ListMaskRules returns a paginated list of mask rules with optional filtering.
func (s *MaskRuleService) ListMaskRules(ctx context.Context, page, pageSize int, datasourceIDStr, database, tableName string) ([]model.MaskRule, int64, error) {
	p := ParsePagination(page, pageSize)

	var filters []FilterClause
	if datasourceIDStr != "" {
		filters = append(filters, FilterClause{Condition: "datasource_id = ?", Args: []interface{}{datasourceIDStr}})
	}
	if database != "" {
		filters = append(filters, FilterClause{Condition: "database = ?", Args: []interface{}{database}})
	}
	if tableName != "" {
		filters = append(filters, FilterClause{Condition: "table_name = ?", Args: []interface{}{tableName}})
	}

	whereClause, args := BuildWhereClause(filters)

	var total int64
	countSQL := PaginatedCountSQL("mask_rules", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计脱敏规则失败: %w", err)
	}

	querySQL := fmt.Sprintf(
		`SELECT id, datasource_id, database, table_name, field, mask_type, custom_regex, custom_template, created_at, updated_at
		 FROM mask_rules %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询脱敏规则失败: %w", err)
	}
	defer rows.Close()

	rules := make([]model.MaskRule, 0)
	for rows.Next() {
		var r model.MaskRule
		if err := rows.Scan(&r.ID, &r.DatasourceID, &r.Database, &r.TableName, &r.Field, &r.MaskType, &r.CustomRegex, &r.CustomTemplate, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		rules = append(rules, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历脱敏规则失败: %w", err)
	}

	return rules, total, nil
}

// UpdateMaskRule updates an existing mask rule and records an audit log for the given userID.
func (s *MaskRuleService) UpdateMaskRule(ctx context.Context, userID, id int64, tableName, field, maskType, customRegex, customTemplate string) (*model.MaskRule, error) {
	existing, err := s.GetMaskRule(ctx, id)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(tableName) == "" {
		tableName = existing.TableName
	}
	if strings.TrimSpace(field) == "" {
		field = existing.Field
	}
	if maskType == "" {
		maskType = existing.MaskType
	}
	if !mask.IsValidMaskType(maskType) {
		return nil, ErrMaskRuleTypeInvalid
	}
	if maskType == string(mask.MaskCustom) && strings.TrimSpace(customRegex) == "" && strings.TrimSpace(existing.CustomRegex) == "" {
		return nil, ErrMaskRuleCustomRegexRequired
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE mask_rules SET table_name = ?, field = ?, mask_type = ?, custom_regex = ?, custom_template = ?, updated_at = ? WHERE id = ?`,
		tableName, field, maskType, customRegex, customTemplate, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("更新脱敏规则失败: %w", err)
	}

	existing.TableName = tableName
	existing.Field = field
	existing.MaskType = maskType
	existing.CustomRegex = customRegex
	existing.CustomTemplate = customTemplate
	existing.UpdatedAt = now
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "mask_rule_update",
		SQLContent: "mask_rule:" + tableName + "." + field,
		SQLSummary: "更新脱敏规则: " + tableName + "." + field,
	})
	return existing, nil
}

// DeleteMaskRule deletes a mask rule by ID and records an audit log for the given userID.
func (s *MaskRuleService) DeleteMaskRule(ctx context.Context, userID, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM mask_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除脱敏规则失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrMaskRuleNotFound
	}
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "mask_rule_delete",
		SQLContent: "mask_rule_id:" + fmt.Sprintf("%d", id),
		SQLSummary: "删除脱敏规则",
	})
	return nil
}

// --- Sensitive Tables CRUD ---

// CreateSensitiveTable marks a table as sensitive and records an audit log for the given userID.
func (s *MaskRuleService) CreateSensitiveTable(ctx context.Context, userID, datasourceID int64, database, tableName, sensitivityLevel string) (*model.SensitiveTable, error) {
	if strings.TrimSpace(tableName) == "" {
		return nil, ErrSensitiveTableRequired
	}
	if !validSensitivityLevels[sensitivityLevel] {
		return nil, ErrInvalidSensitivityLevel
	}

	// Check for duplicate
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sensitive_tables WHERE datasource_id = ? AND database = ? AND table_name = ?`,
		datasourceID, database, tableName,
	).Scan(&count)
	if err == nil && count > 0 {
		return nil, ErrSensitiveTableDuplicate
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO sensitive_tables (datasource_id, database, table_name, sensitivity_level, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		datasourceID, database, tableName, sensitivityLevel, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("创建敏感表记录失败: %w", err)
	}

	id, _ := result.LastInsertId()
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "sensitive_table_create",
		SQLContent: "sensitive_table:" + tableName,
		SQLSummary: "标记敏感表: " + tableName,
	})
	return &model.SensitiveTable{
		ID:               id,
		DatasourceID:     datasourceID,
		Database:         database,
		TableName:        tableName,
		SensitivityLevel: sensitivityLevel,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

// ListSensitiveTables returns a paginated list of sensitive tables with optional filtering.
func (s *MaskRuleService) ListSensitiveTables(ctx context.Context, page, pageSize int, datasourceIDStr, database, tableName string) ([]model.SensitiveTable, int64, error) {
	p := ParsePagination(page, pageSize)

	var filters []FilterClause
	if datasourceIDStr != "" {
		filters = append(filters, FilterClause{Condition: "datasource_id = ?", Args: []interface{}{datasourceIDStr}})
	}
	if database != "" {
		filters = append(filters, FilterClause{Condition: "database = ?", Args: []interface{}{database}})
	}
	if tableName != "" {
		filters = append(filters, FilterClause{Condition: "table_name LIKE ?", Args: []interface{}{"%" + tableName + "%"}})
	}

	whereClause, args := BuildWhereClause(filters)

	var total int64
	countSQL := PaginatedCountSQL("sensitive_tables", whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计敏感表失败: %w", err)
	}

	querySQL := fmt.Sprintf(
		`SELECT id, datasource_id, database, table_name, sensitivity_level, created_at, updated_at
		 FROM sensitive_tables %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		whereClause,
	)
	queryArgs := AppendLimitArgs(args, p)

	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询敏感表失败: %w", err)
	}
	defer rows.Close()

	tables := make([]model.SensitiveTable, 0)
	for rows.Next() {
		var t model.SensitiveTable
		if err := rows.Scan(&t.ID, &t.DatasourceID, &t.Database, &t.TableName, &t.SensitivityLevel, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		tables = append(tables, t)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历敏感表失败: %w", err)
	}

	return tables, total, nil
}

// DeleteSensitiveTable removes a sensitive table marking and records an audit log for the given userID.
func (s *MaskRuleService) DeleteSensitiveTable(ctx context.Context, userID, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sensitive_tables WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除敏感表记录失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrSensitiveTableNotFound
	}
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     userID,
		Action:     "sensitive_table_delete",
		SQLContent: "sensitive_table_id:" + fmt.Sprintf("%d", id),
		SQLSummary: "取消敏感表标记",
	})
	return nil
}

// --- Desensitize Bypass ---

// HasDesensitizeBypass checks if a user's role has desensitize:bypass permission
// for the given datasource and tables.
func (s *MaskRuleService) HasDesensitizeBypass(role string, datasourceID int64, tables []string) bool {
	if s.permSvc == nil {
		return false
	}
	dom := fmt.Sprintf("ds_%d", datasourceID)

	for _, table := range tables {
		if table == "" {
			continue
		}
		allowed, err := s.permSvc.Enforce(role, dom, table, "desensitize:bypass")
		if err == nil && allowed {
			return true
		}
	}
	// Also check wildcard
	allowed, err := s.permSvc.Enforce(role, dom, "*", "desensitize:bypass")
	if err == nil && allowed {
		return true
	}
	return false
}

// GetSensitiveTablesForDatasource returns the list of sensitive table names for a given datasource.
func (s *MaskRuleService) GetSensitiveTablesForDatasource(ctx context.Context, datasourceID int64, database string) ([]model.SensitiveTable, error) {
	query := `SELECT id, datasource_id, database, table_name, sensitivity_level, created_at, updated_at
			  FROM sensitive_tables WHERE datasource_id = ?`
	args := []interface{}{datasourceID}

	if database != "" {
		query += ` AND (database = ? OR database = '')`
		args = append(args, database)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询敏感表失败: %w", err)
	}
	defer rows.Close()

	tables := make([]model.SensitiveTable, 0)
	for rows.Next() {
		var t model.SensitiveTable
		if err := rows.Scan(&t.ID, &t.DatasourceID, &t.Database, &t.TableName, &t.SensitivityLevel, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历敏感表失败: %w", err)
	}
	return tables, nil
}
