package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
)

// readPurpose is why the platform is reading a target database.
//
// It selects the row cap, the Casbin actions and the audit action strings — and
// nothing else. A purpose must never select which gates run; that is the whole
// reason it is data rather than three hand-written prologues. ExplainQuery is
// the counterexample this type is written against: it was a third copy that
// omitted five of the seven gates its siblings applied, and one of the
// omissions let EXPLAIN ANALYZE DELETE reach the database and delete the rows.
type readPurpose int

const (
	purposeInteractive readPurpose = iota // POST /api/query/execute
	purposeExport                         // POST /api/query/export
	purposePlan                           // POST /api/query/explain
)

// readPolicy is everything a purpose is allowed to vary.
type readPolicy struct {
	actions    []string // Casbin actions every target must satisfy
	okAction   string   // audit action for a completed read
	failAction string   // audit action for a refused or failed one
	// nonSelect is the error a non-SELECT statement is refused with. The
	// verdict itself is not negotiable; only how it is reported to the user is.
	nonSelect error
}

var readPolicies = map[readPurpose]readPolicy{
	purposeInteractive: {
		actions:    []string{"select"},
		okAction:   "query",
		failAction: "query_failed",
		nonSelect:  ErrSQLOperationForbidden,
	},
	purposeExport: {
		actions:    []string{"select", "export"},
		okAction:   "export",
		failAction: "export_failed",
		nonSelect:  ErrSQLOperationForbidden,
	},
	purposePlan: {
		actions:    []string{"select"},
		okAction:   "explain",
		failAction: "explain_failed",
		nonSelect:  ErrExplainNonSelect,
	},
}

// readRequest is one authorized read of a target database.
type readRequest struct {
	UserID       int64
	Role         string
	DatasourceID int64
	Database     string // requested; ResolveQueryScope has the last word
	SQL          string
	Purpose      readPurpose
}

// readGrant is what survived the gates: permission to run this statement, in
// this scope, against this datasource.
//
// Callers must use Scope rather than the database they asked for. The two are
// the same only when the request named the datasource's own database, and a
// request that named another one never gets here.
type readGrant struct {
	DatasourceName string
	Datasource     *datasource.Adapter
	Config         *driver.Config
	Scope          string
	DBType         string
	Targets        []string
	SQL            string // the statement to run, after any purpose-specific rewriting
}

// authorizeRead is the only place internal/query decides that a statement may
// touch a target database.
//
// The order is fixed and not negotiable by the caller: datasource fetch,
// disabled check, internal-datasource gate, secret decrypt, parse, blocked,
// operation, risk, Casbin sweep over every target, scope resolution. Every
// refusal from the point the datasource row is in hand writes a <purpose>_failed
// audit record before returning, because the reads that are refused are exactly
// the ones an operator needs evidence of.
//
// It exists because these eleven steps were written out three times and the
// third copy omitted five of them. Purpose varies the row cap, the Casbin
// actions and the audit strings; it does not vary this list.
func (s *Service) authorizeRead(ctx context.Context, req readRequest) (*readGrant, error) {
	policy, ok := readPolicies[req.Purpose]
	if !ok {
		return nil, fmt.Errorf("未知的读取用途: %d", req.Purpose)
	}

	if strings.TrimSpace(req.SQL) == "" {
		return nil, ErrEmptySQL
	}

	ds, err := s.dsSvc.GetDataSource(ctx, req.DatasourceID)
	if err != nil {
		// No datasource row, so no scope to name and nothing to audit against.
		return nil, fmt.Errorf("获取数据源失败: %w", err)
	}

	// From here on a refusal is auditable, so every return goes through refuse.
	refuse := func(cause error) (*readGrant, error) {
		s.auditReadFailure(ctx, policy.failAction, req, ds.Database, cause)
		return nil, cause
	}

	if ds.Status == "disabled" {
		return refuse(datasource.ErrDatasourceDisabled)
	}
	if datasource.IsInternal(ds) && req.Role != "admin" {
		return refuse(ErrInternalDatasourceOnly)
	}

	secrets, err := datasource.DecryptSecrets(ds, s.encryptionKey)
	if err != nil {
		return refuse(fmt.Errorf("解密数据源凭据失败: %w", err))
	}

	// The datasource's own driver interprets the statement. Operation, risk and
	// target semantics belong to the data source, not to a switch here.
	parseResult, err := driver.ParseFor(ds.Type, req.SQL)
	if err != nil {
		return refuse(fmt.Errorf("SQL解析失败: %w", err))
	}
	if parseResult.IsBlocked {
		return refuse(fmt.Errorf("%w: %s", ErrSQLBlocked, parseResult.BlockReason))
	}
	if parseResult.Operation != driver.OpSelect {
		return refuse(policy.nonSelect)
	}
	if parseResult.RiskLevel == driver.RiskHigh {
		return refuse(ErrSQLHighRisk)
	}

	for _, table := range parseResult.Targets {
		for _, action := range policy.actions {
			allowed, err := s.permSvc.EnforceActor(ctx, req.UserID, req.Role,
				authz.DatasourceDomain(req.DatasourceID), table, action)
			if err != nil {
				return refuse(fmt.Errorf("权限校验失败: %w", err))
			}
			if !allowed {
				return refuse(fmt.Errorf("没有表 %s 的%s权限", table, actionLabel(action)))
			}
		}
	}

	// Settle which database this read runs in before anything consumes the
	// answer. The scope is the datasource's — the pool keys on datasource ID and
	// the DSN pins the database — so a request naming another one is refused
	// rather than honored in name only.
	scope, err := datasource.ResolveQueryScope(ds.Database, req.Database)
	if err != nil {
		return refuse(err)
	}

	adapter := datasource.NewAdapter(ds)
	cfg, err := driver.BuildConfigFromDataSource(adapter, secrets)
	if err != nil {
		return refuse(err)
	}

	return &readGrant{
		DatasourceName: ds.Name,
		Datasource:     adapter,
		Config:         cfg,
		Scope:          scope,
		DBType:         ds.Type,
		Targets:        parseResult.Targets,
		SQL:            req.SQL,
	}, nil
}

// actionLabel names a Casbin action in the language the refusal is written in.
func actionLabel(action string) string {
	switch action {
	case "export":
		return "导出"
	default:
		return "查询"
	}
}

// auditReadFailure records a read that was refused or failed.
//
// Invariant 3: the reads that do not happen are the ones an operator most needs
// evidence of. ExplainQuery wrote no failure record at all — a refused plan left
// no trace anywhere — and ExportQuery repeated the same record literal five
// times, which had already drifted into three variants.
func (s *Service) auditReadFailure(ctx context.Context, action string, req readRequest, database string, cause error) {
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:       req.UserID,
		Action:       action,
		DatasourceID: req.DatasourceID,
		Database:     database,
		SQLContent:   req.SQL,
		SQLSummary:   auditlog.Summarize(req.SQL),
		ErrorMessage: cause.Error(),
	})
}

// runRead connects and executes under one deadline.
//
// Connect and execute share a single budget because the sum is what the user
// waits for and what the pooled connection is occupied for. The plan path used
// to have no budget at all, on the theory that the driver imposes one — three of
// the five drivers do not.
func (s *Service) runRead(ctx context.Context, g *readGrant, args []interface{}, limit int) (*driver.QueryResult, error) {
	if s.poolMgr == nil {
		return nil, ErrQueryUnavailable
	}

	drvCtx, cancel := context.WithTimeout(ctx, s.limits.Timeout)
	defer cancel()

	d, err := s.poolMgr.Get(drvCtx, g.Config)
	if err != nil {
		// A datasource that cannot be reached is reported as a timeout, which is
		// what it is from the caller's side, and lets the handler answer 400.
		return nil, ErrSQLTimeout
	}

	result, err := executeDriverQuery(drvCtx, d, g.SQL, args, limit)
	if err != nil {
		if drvCtx.Err() == context.DeadlineExceeded {
			return nil, ErrSQLTimeout
		}
		return nil, err
	}
	return result, nil
}
