package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

var (
	ErrTemplateNotFound = errors.New("SQL template not found")
	ErrTemplateNameExists = errors.New("template name already exists for this user")
	ErrSQLContentTooLarge = errors.New("SQL content exceeds 10KB limit")
)

const maxSQLContentBytes = 10 * 1024 // 10KB

// placeholderRegex matches {{param_name}} and {{param_name:default}}
var placeholderRegex = regexp.MustCompile(`\{\{(\w+)(?::([^}]*))?\}\}`)

// TemplateService provides CRUD and render operations for SQL templates.
type TemplateService struct {
	db *sql.DB
}

// NewSQLTemplateService creates a new TemplateService.
func NewSQLTemplateService(db *sql.DB) *TemplateService {
	return &TemplateService{db: db}
}

// CreateTemplate creates a new SQL template for the given user.
func (s *TemplateService) CreateTemplate(ctx context.Context, userID int64, name, description, sqlContent, dbType, category string, isPublic bool) (*model.SQLTemplate, error) {
	if len(sqlContent) > maxSQLContentBytes {
		return nil, ErrSQLContentTooLarge
	}

	paramsJSON, err := extractParamsJSON(sqlContent)
	if err != nil {
		return nil, fmt.Errorf("extract params: %w", err)
	}

	now := time.Now()
	pub := 0
	if isPublic {
		pub = 1
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO sql_templates (user_id, name, description, sql_content, db_type, category, params_json, is_public, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, name, description, sqlContent, dbType, category, paramsJSON, pub, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, ErrTemplateNameExists
		}
		return nil, fmt.Errorf("insert template: %w", err)
	}

	id, _ := result.LastInsertId()
	return &model.SQLTemplate{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Description: description,
		SQLContent:  sqlContent,
		DBType:      dbType,
		Category:    category,
		ParamsJSON:  paramsJSON,
		IsPublic:    isPublic,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetTemplate returns a single template by ID.
func (s *TemplateService) GetTemplate(ctx context.Context, id int64) (*model.SQLTemplate, error) {
	t := &model.SQLTemplate{}
	var pub int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, description, sql_content, db_type, category, params_json, is_public, created_at, updated_at
		 FROM sql_templates WHERE id = ?`, id,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.Description, &t.SQLContent, &t.DBType, &t.Category, &t.ParamsJSON, &pub, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}
	t.IsPublic = pub == 1
	return t, nil
}

// ListTemplates returns paginated templates. userID=0 returns all public + user's own.
func (s *TemplateService) ListTemplates(ctx context.Context, userID int64, category string, page, pageSize int) ([]*model.SQLTemplate, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var rows *sql.Rows
	var err error
	var total int64

	if userID == 0 {
		// All public templates
		countQuery := "SELECT COUNT(*) FROM sql_templates WHERE is_public = 1"
		args := []interface{}{}
		listQuery := "SELECT id, user_id, name, description, sql_content, db_type, category, params_json, is_public, created_at, updated_at FROM sql_templates WHERE is_public = 1"
		if category != "" {
			countQuery += " AND category = ?"
			listQuery += " AND category = ?"
			args = append(args, category)
		}
		listQuery += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"

		err = s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count templates: %w", err)
		}

		listArgs := append(args, pageSize, offset)
		rows, err = s.db.QueryContext(ctx, listQuery, listArgs...)
	} else {
		// User's own + all public
		countQuery := "SELECT COUNT(*) FROM sql_templates WHERE user_id = ? OR is_public = 1"
		args := []interface{}{userID}
		listQuery := "SELECT id, user_id, name, description, sql_content, db_type, category, params_json, is_public, created_at, updated_at FROM sql_templates WHERE user_id = ? OR is_public = 1"
		if category != "" {
			countQuery += " AND category = ?"
			listQuery += " AND category = ?"
			args = append(args, category)
		}
		listQuery += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"

		err = s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count templates: %w", err)
		}

		listArgs := append(args, pageSize, offset)
		rows, err = s.db.QueryContext(ctx, listQuery, listArgs...)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []*model.SQLTemplate
	for rows.Next() {
		t := &model.SQLTemplate{}
		var pub int
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Description, &t.SQLContent, &t.DBType, &t.Category, &t.ParamsJSON, &pub, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan template: %w", err)
		}
		t.IsPublic = pub == 1
		templates = append(templates, t)
	}

	return templates, total, nil
}

// UpdateTemplate updates an existing template (only the creator can update).
func (s *TemplateService) UpdateTemplate(ctx context.Context, id, userID int64, name, description, sqlContent, dbType, category string, isPublic bool) error {
	if len(sqlContent) > maxSQLContentBytes {
		return ErrSQLContentTooLarge
	}

	paramsJSON, err := extractParamsJSON(sqlContent)
	if err != nil {
		return fmt.Errorf("extract params: %w", err)
	}

	pub := 0
	if isPublic {
		pub = 1
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE sql_templates SET name=?, description=?, sql_content=?, db_type=?, category=?, params_json=?, is_public=?, updated_at=?
		 WHERE id = ? AND user_id = ?`,
		name, description, sqlContent, dbType, category, paramsJSON, pub, time.Now(), id, userID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrTemplateNameExists
		}
		return fmt.Errorf("update template: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// DeleteTemplate deletes a template (only the creator can delete).
func (s *TemplateService) DeleteTemplate(ctx context.Context, id, userID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sql_templates WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// RenderResult holds the output of RenderTemplate.
type RenderResult struct {
	RenderedSQL string        `json:"rendered_sql"`
	ParamValues []interface{} `json:"param_values"`
	SQL         string        `json:"sql"`
}

// RenderTemplate replaces placeholders in a template with parameter values.
// MySQL: ?, PostgreSQL: $1,$2,..., MongoDB: original template.
func (s *TemplateService) RenderTemplate(ctx context.Context, id int64, params map[string]string) (*RenderResult, error) {
	t, err := s.GetTemplate(ctx, id)
	if err != nil {
		return nil, err
	}

	matches := placeholderRegex.FindAllStringSubmatchIndex(t.SQLContent, -1)
	if len(matches) == 0 {
		// No placeholders, return as-is
		return &RenderResult{
			RenderedSQL: t.SQLContent,
			ParamValues: nil,
			SQL:         t.SQLContent,
		}, nil
	}

	var paramValues []interface{}
	var rendered strings.Builder
	lastIdx := 0

	for _, match := range matches {
		// match: [fullStart, fullEnd, nameStart, nameEnd, defaultStart, defaultEnd]
		paramName := t.SQLContent[match[2]:match[3]]
		defaultVal := ""
		if match[4] != -1 {
			defaultVal = t.SQLContent[match[4]:match[5]]
		}

		// Use provided param value or default
		val, ok := params[paramName]
		if !ok {
			val = defaultVal
		}

		switch t.DBType {
		case "mysql":
			rendered.WriteString(t.SQLContent[lastIdx:match[0]])
			rendered.WriteString("?")
			paramValues = append(paramValues, val)
		case "postgresql":
			rendered.WriteString(t.SQLContent[lastIdx:match[0]])
			fmt.Fprintf(&rendered, "$%d", len(paramValues)+1)
			paramValues = append(paramValues, val)
		case "mongodb":
			rendered.WriteString(t.SQLContent[lastIdx:match[0]])
			rendered.WriteString(url.QueryEscape(val))
		default:
			rendered.WriteString(t.SQLContent[lastIdx:match[0]])
			rendered.WriteString("?")
			paramValues = append(paramValues, val)
		}

		lastIdx = match[1]
	}

	rendered.WriteString(t.SQLContent[lastIdx:])

	renderedSQL := rendered.String()

	// For MongoDB, the SQL field contains the JSON representation
	sqlOut := renderedSQL
	if t.DBType == "mongodb" {
		// Build a simple JSON params object
		jsonParams, _ := json.Marshal(params)
		sqlOut = string(jsonParams)
	}

	return &RenderResult{
		RenderedSQL: renderedSQL,
		ParamValues: paramValues,
		SQL:         sqlOut,
	}, nil
}

// extractParamsJSON extracts all placeholder names and defaults into a JSON array.
func extractParamsJSON(sqlContent string) (string, error) {
	matches := placeholderRegex.FindAllStringSubmatch(sqlContent, -1)

	type paramInfo struct {
		Name    string `json:"name"`
		Default string `json:"default"`
	}

	// Deduplicate while preserving order
	seen := map[string]bool{}
	var params []paramInfo
	for _, m := range matches {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		def := ""
		if len(m) > 2 && m[2] != "" {
			def = m[2]
		}
		params = append(params, paramInfo{Name: name, Default: def})
	}

	if len(params) == 0 {
		return "[]", nil
	}

	// Ensure deterministic JSON output
	sort.Slice(params, func(i, j int) bool {
		return params[i].Name < params[j].Name
	})

	data, err := json.Marshal(params)
	if err != nil {
		return "[]", nil
	}
	return string(data), nil
}

// parseParamsJSON parses the stored params_json into a slice.
func parseParamsJSON(paramsJSON string) ([]paramNameAndDefault, error) {
	var params []paramNameAndDefault
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return nil, err
	}
	return params, nil
}

// type alias used internally
type (
	paramNameAndDefault = struct {
		Name    string `json:"name"`
		Default string `json:"default"`
	}
)

// ParseExtractedParams is a convenience function that returns placeholder info from a template.
func ParseExtractedParams(paramsJSON string) ([]map[string]string, error) {
	var raw []struct {
		Name    string `json:"name"`
		Default string `json:"default"`
	}
	if err := json.Unmarshal([]byte(paramsJSON), &raw); err != nil {
		return nil, err
	}
	result := make([]map[string]string, len(raw))
	for i, p := range raw {
		result[i] = map[string]string{"name": p.Name, "default": p.Default}
	}
	return result, nil
}

// unused import guard
var _ = strconv.Itoa
