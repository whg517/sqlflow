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
	// ErrMaskRulesUnavailable means the rules protecting a result could not be
	// read, so nothing can be said about what the result exposes. Refusing is the
	// only answer that does not risk serving protected values in the clear.
	ErrMaskRulesUnavailable = errors.New("无法读取脱敏规则，结果已拒绝返回")
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
	query string,
	args []interface{},
	limit int,
) (*driver.QueryResult, error) {
	if len(args) == 0 {
		return d.ExecuteQuery(ctx, query, limit)
	}
	parameterized, ok := d.(driver.ParameterizedQueryExecutor)
	if !ok {
		return nil, ErrQueryParamsUnsupported
	}
	return parameterized.ExecuteQueryWithArgs(ctx, query, args, limit)
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

	secrets, err := datasource.DecryptSecrets(ds, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密数据源凭据失败: %w", err)
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

	// Settle which database this query runs in before anything consumes the
	// answer. The scope is the datasource's — the pool keys on datasource ID and
	// the DSN pins the database — so a request naming another one is refused
	// rather than honored in name only. It used to be honored in name only in
	// the worst possible place: the driver dropped it, while loadMaskRules below
	// used it to decide which rules to load.
	scope, err := datasource.ResolveQueryScope(ds.Database, database)
	if err != nil {
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:       userID,
			Action:       "query_failed",
			DatasourceID: datasourceID,
			Database:     ds.Database,
			SQLContent:   sqlContent,
			SQLSummary:   auditlog.Summarize(sqlContent),
			ErrorMessage: err.Error(),
		})
		return nil, err
	}

	queryStart := time.Now()

	if s.poolMgr == nil {
		return nil, ErrQueryUnavailable
	}
	adapter := datasource.NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, secrets)
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
		var drvResult *driver.QueryResult
		drvResult, err = executeDriverQuery(ctx, d, sqlContent, queryParams, defaultRowLimit)
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
			Database:        scope,
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

	if err := s.refuseUnmaskableShape(ctx, result.Shape, userID, role, datasourceID, scope, parseResult.Targets); err != nil {
		s.auditRelease(ctx, userID, datasourceID, scope, sqlContent, err)
		return nil, err
	}

	// Apply desensitization
	desensitized, maskedFields, err := s.applyDesensitizationForActor(ctx, result, userID, role, datasourceID, scope, parseResult.Targets)
	if err != nil {
		s.auditRelease(ctx, userID, datasourceID, scope, sqlContent, err)
		return nil, err
	}
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
		Database:      scope,
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
		Database:           scope,
		SQLContent:         sqlContent,
		SQLSummary:         summary,
		ResultRows:         result.Total,
		AffectedRows:       result.AffectedRows,
		ExecutionTimeMs:    result.ExecutionTime,
		DesensitizedFields: strings.Join(maskedFields, ","),
	})

	return result, nil
}

// actorMayUnmask reports whether this actor holds a grant that lets protected
// fields through in the clear.
//
// The sweep was written out twice, verbatim — once here and once in
// maskingApplies — which is two chances for an authorization decision to drift.
// A third copy in internal/security already had drifted: HasDesensitizeBypass
// calls Enforce rather than EnforceActor, so it cannot see individual user
// grants at all.
//
// An enforcer that cannot answer has granted nothing, so masking stays on. That
// is the safe direction and is what both hand-written copies did by discarding
// the error.
func (s *Service) actorMayUnmask(ctx context.Context, userID int64, role string, datasourceID int64, tables []string) bool {
	if s.permSvc == nil {
		return false
	}

	// The canonical action is "unmask"; "desensitize:bypass" is kept so existing
	// installations do not silently lose access.
	granted := func(object string) bool {
		for _, action := range []string{"unmask", "desensitize:bypass"} {
			ok, err := s.permSvc.EnforceActor(ctx, userID, role,
				authz.DatasourceDomain(datasourceID), object, action)
			if err == nil && ok {
				return true
			}
		}
		return false
	}

	for _, table := range tables {
		if table == "" {
			continue
		}
		if granted(table) {
			return true
		}
	}
	return granted("*")
}

func (s *Service) applyDesensitizationForActor(ctx context.Context, result *QueryResult, userID int64, role string, datasourceID int64, database string, tables []string) (bool, []string, error) {
	if s.actorMayUnmask(ctx, userID, role, datasourceID, tables) {
		return false, nil, nil
	}

	rules, err := loadMaskRules(ctx, s.client, datasourceID, database, tables)
	if err != nil {
		return false, nil, err
	}
	if len(rules) == 0 {
		return false, nil, nil
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
		return false, nil, nil
	}
	return true, allMaskedFields, nil
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
	applies, err := s.maskingApplies(ctx, userID, role, datasourceID, database, tables)
	if err != nil {
		return err
	}
	if applies {
		return ErrAggregationMaskingUnsupported
	}
	return nil
}

// maskingApplies reports whether masking would alter a result for this actor:
// the actor lacks an unmask grant and at least one rule matches the targets.
//
// It is the precondition check for result shapes the row masker cannot process.
func (s *Service) maskingApplies(ctx context.Context, userID int64, role string, datasourceID int64, database string, tables []string) (bool, error) {
	if s.actorMayUnmask(ctx, userID, role, datasourceID, tables) {
		return false, nil
	}

	rules, err := loadMaskRules(ctx, s.client, datasourceID, database, tables)
	if err != nil {
		return false, err
	}
	for _, table := range tables {
		if len(mask.MatchRules(rules, table)) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// loadMaskRules loads the rules protecting these targets.
//
// It is a package-level function taking the client rather than a method,
// because the share path needs the same rules and has no query Service: a
// published result is read by an anonymous party, so every matching rule
// applies to it unconditionally.
//
// It fails closed. The signature used to be `[]mask.Rule` with no error and it
// answered nil when the read failed — and the comment sitting where this one is
// conceded the consequence, because both callers treat an empty rule set as
// "nothing to protect". One failed read therefore served the rows in the clear
// and, on an aggregation, also stopped the refusal that exists because the
// masker cannot reach inside a driver-native payload. Invariant 4 has no
// availability exception: a result that cannot be shown to be safe is refused.
func loadMaskRules(ctx context.Context, client *ent.Client, datasourceID int64, database string, tables []string) ([]mask.Rule, error) {
	q := client.MaskRule.Query().Where(entmaskrule.DatasourceIDEQ(datasourceID))

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
		return nil, fmt.Errorf("%w: %v", ErrMaskRulesUnavailable, err)
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
	return rules, nil
}

// ResolveShareScope answers where a result the caller wants to publish came
// from, and refuses if they may not read it.
//
// It satisfies ShareScopeResolver. The scope has to be derived rather than
// accepted for the same reason the risk level is: a client that named its own
// targets would publish rows no mask rule could match, which is the difference
// between masking being applied and masking being unrepresentable.
//
// The permission sweep is deliberately repeated here rather than assumed from
// the query that produced the rows. Publishing is its own entrance and grants
// can have been withdrawn since; invariant 1 gives it no exemption.
func (s *Service) ResolveShareScope(ctx context.Context, userID int64, role string, datasourceID int64, sqlContent string) (ShareScope, error) {
	if strings.TrimSpace(sqlContent) == "" {
		return ShareScope{}, ErrEmptySQL
	}

	ds, err := s.dsSvc.GetDataSource(ctx, datasourceID)
	if err != nil {
		return ShareScope{}, fmt.Errorf("获取数据源失败: %w", err)
	}
	if datasource.IsInternal(ds) && role != "admin" {
		return ShareScope{}, ErrInternalDatasourceOnly
	}

	parseResult, err := driver.ParseFor(ds.Type, sqlContent)
	if err != nil {
		return ShareScope{}, fmt.Errorf("SQL解析失败: %w", err)
	}
	for _, table := range parseResult.Targets {
		allowed, err := s.permSvc.EnforceActor(ctx, userID, role,
			authz.DatasourceDomain(datasourceID), table, "select")
		if err != nil {
			return ShareScope{}, fmt.Errorf("权限校验失败: %w", err)
		}
		if !allowed {
			return ShareScope{}, fmt.Errorf("没有表 %s 的查询权限", table)
		}
	}

	// The scope is the datasource's, exactly as it is for a query.
	scope, err := datasource.ResolveQueryScope(ds.Database, "")
	if err != nil {
		return ShareScope{}, err
	}
	return ShareScope{Database: scope, Targets: parseResult.Targets}, nil
}

// auditRelease records a result that was produced but refused on the way out.
//
// Invariant 3: the queries that never reached the caller are exactly the ones an
// operator needs evidence of. A refusal here means the target database was read
// and the rows were then withheld, which is a different event from a query that
// failed to run, and it would otherwise leave no trace at all.
func (s *Service) auditRelease(ctx context.Context, userID, datasourceID int64, scope, sqlContent string, cause error) {
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:       userID,
		Action:       "query_failed",
		DatasourceID: datasourceID,
		Database:     scope,
		SQLContent:   sqlContent,
		SQLSummary:   auditlog.Summarize(sqlContent),
		ErrorMessage: cause.Error(),
	})
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
	// The same gate ExecuteQuery and ExportQuery apply, which this entrance did
	// not. The shipped seed policy grants `developer` select on every object, so
	// the Casbin loop below denies nothing and this was the only thing standing
	// between a non-admin and the platform's own metadata store.
	if datasource.IsInternal(ds) && role != "admin" {
		return nil, ErrInternalDatasourceOnly
	}

	secrets, err := datasource.DecryptSecrets(ds, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密数据源凭据失败: %w", err)
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

	// A plan is produced by the same connection that would run the query, so it
	// answers for the same scope and is held to the same rule.
	scope, err := datasource.ResolveQueryScope(ds.Database, database)
	if err != nil {
		return nil, err
	}

	if s.poolMgr == nil {
		return nil, ErrQueryUnavailable
	}
	adapter := datasource.NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, secrets)
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
	drvResult, err := explainer.ExplainQuery(ctx, explainSQL, queryParams)
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
		Database:     scope,
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
