package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entmaskrule "github.com/whg517/sqlflow/internal/db/ent/maskrule"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/crypto"
	"github.com/whg517/sqlflow/internal/platform/mask"
	pkgmetrics "github.com/whg517/sqlflow/internal/platform/metrics"
	"github.com/whg517/sqlflow/internal/security"
)

const (
	defaultRowLimit = 1000
	queryTimeout    = 30 * time.Second
)

var (
	ErrSQLOperationForbidden  = errors.New("该操作需要提交工单，仅允许 SELECT 查询")
	ErrSQLHighRisk            = errors.New("高风险操作被拦截，请提交工单")
	ErrSQLBlocked             = errors.New("SQL操作被拦截")
	ErrSQLTimeout             = errors.New("查询超时（30秒）")
	ErrEmptySQL               = errors.New("SQL 不能为空")
	ErrQueryParamsUnsupported = errors.New("当前数据源不支持参数化查询")
	ErrInternalDatasourceOnly = errors.New("SQLFlow 元数据库仅允许管理员访问")
	// ErrQueryUnavailable indicates the service was built without a connection
	// pool, so no datasource can be reached. This is a wiring fault, not a user
	// error — app.Container always provides one.
	ErrQueryUnavailable = errors.New("查询服务未正确初始化")

	// ErrAggregationMaskingUnsupported is returned when an aggregation result
	// would have to be masked. Aggregation payloads are driver-native and
	// arbitrarily nested, so the row masker cannot reach into them; refusing is
	// the only option that does not leak protected fields through bucket keys.
	ErrAggregationMaskingUnsupported = errors.New("该索引存在脱敏规则，暂不支持聚合查询（聚合结果无法脱敏）")
)

// QueryResult holds the result of a query execution.
type QueryResult struct {
	// Shape tells the client how to render this result. Empty means table, so
	// existing relational callers are unaffected.
	Shape              driver.ResultShape       `json:"shape,omitempty"`
	Columns            []string                 `json:"columns"`
	Rows               []map[string]interface{} `json:"rows"`
	Total              int64                    `json:"total"`
	ExecutionTime      int64                    `json:"execution_time_ms"`
	AffectedRows       int64                    `json:"affected_rows"`
	Desensitized       bool                     `json:"desensitized"`
	DesensitizedFields []string                 `json:"desensitized_fields,omitempty"`
	Warnings           []string                 `json:"warnings,omitempty"`
	HistoryID          int64                    `json:"history_id,omitempty"`

	// Aggregations carries a driver-native aggregation payload verbatim when
	// Shape is ShapeAggregation.
	Aggregations json.RawMessage `json:"aggregations,omitempty"`
}

// Service handles SQL query execution logic.
type Service struct {
	database      *db.DB
	client        *ent.Client
	dsSvc         *datasource.Service
	historySvc    *HistoryService
	permSvc       *security.Service
	auditSvc      auditlog.Writer
	encryptionKey string
	poolMgr       *driver.PoolManager
}

// NewService creates a new Service.
func NewService(database *db.DB, dsSvc *datasource.Service, historySvc *HistoryService, permSvc *security.Service, auditSvc auditlog.Writer, encryptionKey string, poolMgr *driver.PoolManager) *Service {
	return &Service{
		database:      database,
		client:        database.Client(),
		dsSvc:         dsSvc,
		historySvc:    historySvc,
		permSvc:       permSvc,
		auditSvc:      auditlog.OrDiscard(auditSvc),
		encryptionKey: encryptionKey,
		poolMgr:       poolMgr,
	}
}

func executeDriverQuery(
	ctx context.Context,
	d driver.Driver,
	database,
	query string,
	args []interface{},
	limit int,
) (*driver.QueryResult, error) {
	if len(args) == 0 {
		return d.ExecuteQuery(ctx, database, query, limit)
	}
	parameterized, ok := d.(driver.ParameterizedQueryExecutor)
	if !ok {
		return nil, ErrQueryParamsUnsupported
	}
	return parameterized.ExecuteQueryWithArgs(ctx, database, query, args, limit)
}

// ExecuteQuery executes a SQL query on the specified datasource.
func (s *Service) ExecuteQuery(ctx context.Context, userID int64, username, role string, datasourceID int64, database, sqlContent, dbType string, queryParams ...interface{}) (*QueryResult, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrEmptySQL
	}

	// Get datasource
	ds, err := s.dsSvc.GetDataSource(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("获取数据源失败: %w", err)
	}
	if ds.Status == "disabled" {
		return nil, datasource.ErrDatasourceDisabled
	}
	if datasource.IsInternal(ds) && role != "admin" {
		return nil, ErrInternalDatasourceOnly
	}

	// Use datasource type if dbType not explicitly provided
	if dbType == "" {
		dbType = ds.Type
	}

	// Decrypt password
	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密密码失败: %w", err)
	}

	// Ask the driver to interpret the query. Parsing needs no connection, and
	// routing it through the driver keeps operation/risk/target semantics owned
	// by the data source rather than by a switch in this function.
	parseResult, err := driver.ParseFor(dbType, sqlContent)
	if err != nil {
		return nil, fmt.Errorf("SQL解析失败: %w", err)
	}

	// Check if blocked by static rules
	if parseResult.IsBlocked {
		return nil, fmt.Errorf("%w: %s", ErrSQLBlocked, parseResult.BlockReason)
	}

	// Only allow SELECT for direct execution
	if parseResult.Operation != driver.OpSelect {
		return nil, ErrSQLOperationForbidden
	}

	// Check high risk
	if parseResult.RiskLevel == driver.RiskHigh {
		return nil, ErrSQLHighRisk
	}

	// Check table-level permissions via Casbin
	for _, table := range parseResult.Targets {
		allowed, err := s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "select")
		if err != nil {
			return nil, fmt.Errorf("权限校验失败: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("没有表 %s 的查询权限", table)
		}
	}

	queryStart := time.Now()

	if s.poolMgr == nil {
		return nil, ErrQueryUnavailable
	}
	adapter := datasource.NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
	if err != nil {
		return nil, err
	}
	// Everything from here on funnels into the shared error handling below, so
	// that a failure at any stage still produces a query_failed audit record.
	// A connection failure used to bypass it, which lost the evidence for
	// exactly the queries an operator most wants to see.
	var result *QueryResult

	// Bound the connection attempt; the driver bounds the execution itself.
	drvCtx, drvCancel := context.WithTimeout(ctx, queryTimeout)
	defer drvCancel()
	d, connErr := s.poolMgr.Get(drvCtx, cfg)
	if connErr != nil {
		// 连接失败：统一映射为 ErrSQLTimeout（数据源不可达/超时，对用户来说语义一致），
		// 让 handler 返回 400（客户端可重试）而非 500。
		err = ErrSQLTimeout
	} else {
		// Pass the requested scope through as-is. What an empty scope means is
		// the driver's business: SQL connections are already bound to a database
		// by the DSN, MongoDB falls back to its configured database, and
		// Elasticsearch to its configured index pattern.
		dbName := database
		if dbName == "" {
			dbName = ds.Database
		}

		var drvResult *driver.QueryResult
		drvResult, err = executeDriverQuery(ctx, d, dbName, sqlContent, queryParams, defaultRowLimit)
		switch {
		case err == nil:
			result = &QueryResult{
				Shape:         drvResult.Shape,
				Columns:       drvResult.Columns,
				Rows:          drvResult.Rows,
				Total:         drvResult.Total,
				ExecutionTime: drvResult.ExecutionTime,
				AffectedRows:  drvResult.AffectedRows,
				Aggregations:  drvResult.Aggregations,
			}
		case ctx.Err() == context.DeadlineExceeded:
			err = ErrSQLTimeout
		}
	}

	// Record Prometheus metrics for external datasource queries
	queryDuration := time.Since(queryStart).Seconds()
	pkgmetrics.DBQueryDuration.WithLabelValues(dbType).Observe(queryDuration)
	pkgmetrics.DBQueriesTotal.WithLabelValues(ds.Name).Inc()

	if err != nil {
		// Write audit log for failed query
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:          userID,
			Action:          "query_failed",
			DatasourceID:    datasourceID,
			Database:        database,
			SQLContent:      sqlContent,
			SQLSummary:      auditlog.Summarize(sqlContent),
			ErrorMessage:    err.Error(),
			ExecutionTimeMs: 0,
		})
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrSQLTimeout
		}
		return nil, err
	}

	if err := s.refuseUnmaskableShape(ctx, result.Shape, userID, role, datasourceID, database, parseResult.Targets); err != nil {
		return nil, err
	}

	// Apply desensitization
	desensitized, maskedFields := s.applyDesensitizationForActor(ctx, result, userID, role, datasourceID, database, parseResult.Targets)
	result.Desensitized = desensitized
	result.DesensitizedFields = maskedFields
	result.Warnings = parseResult.Warnings

	// Write query history (async, best-effort)
	summary := auditlog.Summarize(sqlContent)
	paramsJSON := []byte("[]")
	if len(queryParams) > 0 {
		paramsJSON, _ = json.Marshal(queryParams)
	}
	history := &model.QueryHistory{
		UserID:        userID,
		DatasourceID:  datasourceID,
		Database:      database,
		SQLContent:    sqlContent,
		ParamsJSON:    string(paramsJSON),
		SQLSummary:    summary,
		DBType:        dbType,
		ExecutionTime: result.ExecutionTime,
		ResultRows:    result.Total,
		AffectedRows:  result.AffectedRows,
	}
	if historyID, err := s.historySvc.CreateHistory(ctx, history); err != nil {
		log.Printf("create query history: %v", err)
	} else {
		result.HistoryID = historyID
	}

	// Write audit log for successful query
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:             userID,
		Action:             "query",
		DatasourceID:       datasourceID,
		Database:           database,
		SQLContent:         sqlContent,
		SQLSummary:         summary,
		ResultRows:         result.Total,
		AffectedRows:       result.AffectedRows,
		ExecutionTimeMs:    result.ExecutionTime,
		DesensitizedFields: strings.Join(maskedFields, ","),
	})

	return result, nil
}

// applyDesensitization checks if the user has desensitize:bypass permission.
// If not, applies masking rules to the result set.
func (s *Service) applyDesensitization(ctx context.Context, result *QueryResult, role string, datasourceID int64, database string, tables []string) (bool, []string) {
	return s.applyDesensitizationForActor(ctx, result, 0, role, datasourceID, database, tables)
}

func (s *Service) applyDesensitizationForActor(ctx context.Context, result *QueryResult, userID int64, role string, datasourceID int64, database string, tables []string) (bool, []string) {
	// Check the canonical unmask permission, retaining the legacy action during
	// migration so existing installations do not unexpectedly lose access.
	for _, table := range tables {
		if table == "" {
			continue
		}
		bypass, err := s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "unmask")
		if err == nil && !bypass {
			bypass, err = s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "desensitize:bypass")
		}
		if err == nil && bypass {
			return false, nil
		}
	}
	// Also check wildcard
	bypass, _ := s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), "*", "unmask")
	if !bypass {
		bypass, _ = s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), "*", "desensitize:bypass")
	}
	if bypass {
		return false, nil
	}

	// Load mask rules for this datasource/database/tables
	rules := s.loadMaskRules(ctx, datasourceID, database, tables)
	if len(rules) == 0 {
		return false, nil
	}

	// Apply masking to all rows for all matching tables
	var allMaskedFields []string
	for _, table := range tables {
		tableRules := mask.MatchRules(rules, table)
		if len(tableRules) == 0 {
			continue
		}
		// Use ApplyToMongoRows which supports dot-notation paths for nested documents.
		// For flat SQL results, behaves identically to ApplyToRows.
		masked := mask.ApplyToMongoRows(result.Rows, tableRules)
		allMaskedFields = append(allMaskedFields, masked...)
	}

	if len(allMaskedFields) == 0 {
		return false, nil
	}
	return true, allMaskedFields
}

// refuseUnmaskableShape rejects a result whose shape the masker cannot process
// when masking rules apply to the actor.
//
// Aggregation payloads are driver-native and arbitrarily nested, so the
// row-oriented masker cannot reach inside them — applyDesensitizationForActor
// only ever walks result.Rows. Returning one unmasked turns an aggregation into
// a way to read protected fields through bucket keys and aggregate values.
//
// Every entrance that returns query results must call this. It is one function
// rather than a condition repeated per caller because the export path was
// missing that condition: a user refused a protected field at the query
// entrance could obtain it by exporting an aggregation over the same target.
func (s *Service) refuseUnmaskableShape(ctx context.Context, shape driver.ResultShape, userID int64, role string, datasourceID int64, database string, tables []string) error {
	if shape != driver.ShapeAggregation {
		return nil
	}
	if s.maskingApplies(ctx, userID, role, datasourceID, database, tables) {
		return ErrAggregationMaskingUnsupported
	}
	return nil
}

// maskingApplies reports whether masking would alter a result for this actor:
// the actor lacks an unmask grant and at least one rule matches the targets.
//
// It is the precondition check for result shapes the row masker cannot process.
func (s *Service) maskingApplies(ctx context.Context, userID int64, role string, datasourceID int64, database string, tables []string) bool {
	if s.permSvc == nil {
		return false
	}
	for _, table := range tables {
		if table == "" {
			continue
		}
		bypass, err := s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "unmask")
		if err == nil && !bypass {
			bypass, err = s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "desensitize:bypass")
		}
		if err == nil && bypass {
			return false
		}
	}
	bypass, _ := s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), "*", "unmask")
	if !bypass {
		bypass, _ = s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), "*", "desensitize:bypass")
	}
	if bypass {
		return false
	}

	rules := s.loadMaskRules(ctx, datasourceID, database, tables)
	for _, table := range tables {
		if len(mask.MatchRules(rules, table)) > 0 {
			return true
		}
	}
	return false
}

// loadMaskRules loads mask rules from the database for the given context.
func (s *Service) loadMaskRules(ctx context.Context, datasourceID int64, database string, tables []string) []mask.Rule {
	q := s.client.MaskRule.Query().Where(entmaskrule.DatasourceIDEQ(datasourceID))

	// An empty database or table on a rule means "any": a rule scoped to the
	// datasource applies wherever it is not overridden by a narrower one.
	if database != "" {
		q = q.Where(entmaskrule.Or(
			entmaskrule.DatabaseEQ(database),
			entmaskrule.DatabaseEQ(""),
		))
	}
	if len(tables) > 0 {
		q = q.Where(entmaskrule.Or(
			entmaskrule.TableNameIn(tables...),
			entmaskrule.TableNameEQ("*"),
		))
	}

	found, err := q.All(ctx)
	if err != nil {
		// Returning nil rules would silently unmask. Callers treat an empty rule
		// set as "nothing to protect", so a failure here must be loud in the log
		// even though it cannot be returned.
		log.Printf("load mask rules: %v", err)
		return nil
	}

	rules := make([]mask.Rule, 0, len(found))
	for _, r := range found {
		rules = append(rules, mask.Rule{
			DatasourceID:   r.DatasourceID,
			Database:       r.Database,
			TableName:      r.TableName,
			Field:          r.Field,
			MaskType:       mask.MaskType(r.MaskType),
			CustomRegex:    r.CustomRegex,
			CustomTemplate: r.CustomTemplate,
		})
	}
	return rules
}

// parseMongoBody parses the MongoDB command body into a map.
func parseMongoBody(body string) map[string]interface{} {
	var m map[string]interface{}
	if err := bson.UnmarshalExtJSON([]byte(body), false, &m); err != nil {
		return nil
	}
	return m
}

// ExplainRow represents a single row from MySQL EXPLAIN output.
type ExplainRow struct {
	ID           int64   `json:"id"`
	SelectType   string  `json:"select_type"`
	Table        string  `json:"table"`
	Partitions   *string `json:"partitions"`
	Type         string  `json:"type"`
	PossibleKeys *string `json:"possible_keys"`
	Key          *string `json:"key"`
	KeyLen       *string `json:"key_len"`
	Ref          *string `json:"ref"`
	Rows         int64   `json:"rows"`
	Filtered     float64 `json:"filtered"`
	Extra        *string `json:"extra"`
}

// hasMySQLExplainColumns reports whether a plan uses MySQL's step-table shape,
// which is what the typed ExplainRow view understands.
func hasMySQLExplainColumns(columns []string) bool {
	for _, c := range columns {
		if strings.EqualFold(c, "select_type") {
			return true
		}
	}
	return false
}

// ExplainResult holds the result of an EXPLAIN query.
type ExplainResult struct {
	Query        string       `json:"query"`
	DatasourceID int64        `json:"datasource_id"`
	Plan         []ExplainRow `json:"plan"`
	Formatted    string       `json:"formatted"`

	// Columns and Rows carry the driver's plan verbatim. Plan is only populated
	// for engines whose plan fits MySQL's step table; PostgreSQL reports a
	// single text column, so clients should prefer these.
	Columns []string                 `json:"columns,omitempty"`
	Rows    []map[string]interface{} `json:"rows,omitempty"`
}

var (
	ErrExplainNotSupported = errors.New("该数据源不支持 EXPLAIN")
	ErrExplainNonSelect    = errors.New("EXPLAIN 仅支持 SELECT 语句")
)

// ExplainQuery executes EXPLAIN for a SQL query and returns structured results.
func (s *Service) ExplainQuery(ctx context.Context, userID int64, role string, datasourceID int64, database, sqlContent string, queryParams ...interface{}) (*ExplainResult, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return nil, ErrEmptySQL
	}

	// Only allow SELECT statements
	upper := strings.TrimSpace(strings.ToUpper(sqlContent))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") && !strings.HasPrefix(upper, "EXPLAIN") {
		return nil, ErrExplainNonSelect
	}

	// Strip leading EXPLAIN if user already prefixed it
	explainSQL := sqlContent
	if strings.HasPrefix(upper, "EXPLAIN") {
		explainSQL = strings.TrimSpace(sqlContent[len("EXPLAIN"):])
	}

	// Get datasource
	ds, err := s.dsSvc.GetDataSource(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("获取数据源失败: %w", err)
	}
	if ds.Status == "disabled" {
		return nil, datasource.ErrDatasourceDisabled
	}

	// Decrypt password
	password, err := crypto.Decrypt(ds.PasswordEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密密码失败: %w", err)
	}

	// Parse for the table-level permission check. The datasource type is
	// checked above, so the driver here is the datasource's own.
	parseResult, err := driver.ParseFor(ds.Type, explainSQL)
	if err != nil {
		return nil, fmt.Errorf("SQL解析失败: %w", err)
	}

	for _, table := range parseResult.Targets {
		allowed, err := s.permSvc.EnforceActor(ctx, userID, role, authz.DatasourceDomain(datasourceID), table, "select")
		if err != nil {
			return nil, fmt.Errorf("权限校验失败: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("没有表 %s 的查询权限", table)
		}
	}

	// Get database name
	dbName := database
	if dbName == "" {
		dbName = ds.Database
		if dbName == "" {
			dbName = "information_schema"
		}
	}

	if s.poolMgr == nil {
		return nil, ErrQueryUnavailable
	}
	adapter := datasource.NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, password, "")
	if err != nil {
		return nil, err
	}
	d, err := s.poolMgr.Get(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("连接数据源失败: %w", err)
	}

	// Only drivers that declare the optional QueryExplainer can produce a
	// plan; the dialect and the plan's shape belong to the driver.
	explainer, ok := d.(driver.QueryExplainer)
	if !ok {
		return nil, ErrExplainNotSupported
	}
	drvResult, err := explainer.ExplainQuery(ctx, dbName, explainSQL, queryParams)
	if err != nil {
		return nil, fmt.Errorf("执行 EXPLAIN 失败: %w", err)
	}

	// MySQL-style step tables also populate the typed Plan for the existing
	// tree view; other engines are carried through as generic rows.
	plan := make([]ExplainRow, 0, len(drvResult.Rows))
	if hasMySQLExplainColumns(drvResult.Columns) {
		for _, row := range drvResult.Rows {
			plan = append(plan, explainRowFromMap(row))
		}
	}

	explainColumns := []string{"id", "select_type", "table", "partitions", "type", "possible_keys", "key", "key_len", "ref", "rows", "filtered", "Extra"}
	formatted := formatExplainTable(explainColumns, plan)

	// Write audit log (best-effort)
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:       userID,
		Action:       "explain",
		DatasourceID: datasourceID,
		Database:     database,
		SQLContent:   sqlContent,
		SQLSummary:   auditlog.Summarize(sqlContent),
	})

	return &ExplainResult{
		Query:        sqlContent,
		DatasourceID: datasourceID,
		Plan:         plan,
		Formatted:    formatted,
		Columns:      drvResult.Columns,
		Rows:         drvResult.Rows,
	}, nil
}

func formatExplainTable(columns []string, rows []ExplainRow) string {
	// Build row data as string slices
	strRows := make([][]string, len(rows))
	for i, r := range rows {
		strRows[i] = []string{
			fmt.Sprintf("%d", r.ID),
			r.SelectType,
			r.Table,
			derefOrNull(r.Partitions),
			r.Type,
			derefOrNull(r.PossibleKeys),
			derefOrNull(r.Key),
			derefOrNull(r.KeyLen),
			derefOrNull(r.Ref),
			fmt.Sprintf("%d", r.Rows),
			fmt.Sprintf("%.2f", r.Filtered),
			derefOrNull(r.Extra),
		}
	}

	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}
	for _, row := range strRows {
		for i, val := range row {
			if i < len(widths) && len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	var b strings.Builder
	b.WriteString("EXPLAIN\n")

	// Build separator line
	sep := func() {
		b.WriteByte('+')
		for _, w := range widths {
			b.WriteString(strings.Repeat("-", w+2))
			b.WriteByte('+')
		}
		b.WriteByte('\n')
	}

	sep()

	// Header row
	b.WriteByte('|')
	for i, col := range columns {
		fmt.Fprintf(&b, " %-*s |", widths[i], col)
	}
	b.WriteByte('\n')

	sep()

	// Data rows
	for _, row := range strRows {
		b.WriteByte('|')
		for i, val := range row {
			if i < len(widths) {
				fmt.Fprintf(&b, " %-*s |", widths[i], val)
			}
		}
		b.WriteByte('\n')
	}

	sep()

	return b.String()
}

func derefOrNull(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}

// explainRowFromMap converts a map[string]interface{} (from driver.QueryResult) to ExplainRow.
func explainRowFromMap(row map[string]interface{}) ExplainRow {
	r := ExplainRow{}
	if v, ok := row["id"]; ok {
		r.ID = toInt64(v)
	}
	if v, ok := row["select_type"]; ok {
		r.SelectType = toString(v)
	}
	if v, ok := row["table"]; ok {
		if s := toString(v); s != "" && s != "NULL" {
			r.Table = s
		}
	}
	if v, ok := row["partitions"]; ok {
		if s := toString(v); s != "" && s != "NULL" {
			r.Partitions = &s
		}
	}
	if v, ok := row["type"]; ok {
		r.Type = toString(v)
	}
	if v, ok := row["possible_keys"]; ok {
		if s := toString(v); s != "" && s != "NULL" {
			r.PossibleKeys = &s
		}
	}
	if v, ok := row["key"]; ok {
		if s := toString(v); s != "" && s != "NULL" {
			r.Key = &s
		}
	}
	if v, ok := row["key_len"]; ok {
		if s := toString(v); s != "" && s != "NULL" {
			r.KeyLen = &s
		}
	}
	if v, ok := row["ref"]; ok {
		if s := toString(v); s != "" && s != "NULL" {
			r.Ref = &s
		}
	}
	if v, ok := row["rows"]; ok {
		r.Rows = toInt64(v)
	}
	if v, ok := row["filtered"]; ok {
		r.Filtered = toFloat64Val(v)
	}
	if v, ok := row["Extra"]; ok {
		if s := toString(v); s != "" && s != "NULL" {
			r.Extra = &s
		}
	}
	return r
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case []byte:
		var i int64
		fmt.Sscanf(string(n), "%d", &i)
		return i
	case string:
		var i int64
		fmt.Sscanf(n, "%d", &i)
		return i
	default:
		return 0
	}
}

func toString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toFloat64Val(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case []byte:
		var f float64
		fmt.Sscanf(string(n), "%f", &f)
		return f
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	default:
		return 0
	}
}
