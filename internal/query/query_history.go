package query

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db"

	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/db/ent"
	entqueryhistory "github.com/whg517/sqlflow/internal/db/ent/queryhistory"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
)

const maxHistoryPerUser = 200

// HistoryService handles query history logic.
type HistoryService struct {
	database *db.DB
	client   *ent.Client

	// permSvc answers whether an actor may see beyond their own rows.
	//
	// A nil value narrows every read to the caller, which is the safe end of
	// this decision: without a policy engine there is no way to establish a
	// platform-wide grant, and granting by default is the defect this field
	// exists to close.
	permSvc ActorEnforcer
}

// ActorEnforcer answers whether an actor holds a grant.
//
// Declared here rather than taking security.Service whole, because this is the
// only method the history boundary needs.
type ActorEnforcer interface {
	EnforceActor(ctx context.Context, userID int64, role, domain, object, action string) (bool, error)
}

// NewHistoryService creates a new HistoryService.
func NewHistoryService(database *db.DB) *HistoryService {
	return &HistoryService{database: database, client: database.Client()}
}

// NewHistoryServiceWithPerms creates a HistoryService that can widen a read
// beyond the caller's own rows when the actor holds the platform grant.
func NewHistoryServiceWithPerms(database *db.DB, permSvc ActorEnforcer) *HistoryService {
	return &HistoryService{database: database, client: database.Client(), permSvc: permSvc}
}

// mayReadEveryHistory reports whether this actor holds the platform-wide view.
//
// The alternative — the one this replaced — was for the slow-query entrance to
// simply apply no owner predicate, while the sibling entrance ListHistory
// scoped to the caller. Both read query_history, and it carries SQLContent
// verbatim: statement text with its WHERE-clause literals in it. Making the
// wide view a grant means widening it takes a policy rather than an omission.
func (s *HistoryService) mayReadEveryHistory(ctx context.Context, actor ExportActor) (bool, error) {
	if s.permSvc == nil {
		return false, nil
	}
	allowed, err := s.permSvc.EnforceActor(ctx, actor.UserID, actor.Role,
		authz.SystemDomain, "query_history", "view")
	if err != nil {
		return false, fmt.Errorf("查询历史权限校验失败: %w", err)
	}
	return allowed, nil
}

// CreateHistory inserts a new query history record and auto-cleans old records.
func (s *HistoryService) CreateHistory(ctx context.Context, h *model.QueryHistory) (int64, error) {
	if h.ParamsJSON == "" {
		h.ParamsJSON = "[]"
	}
	// Compute SQL hash for deduplication (normalized: trimmed + lowercase + collapsed whitespace)
	normalized := strings.TrimSpace(strings.ToLower(h.SQLContent))
	// Collapse multiple spaces into one for robust dedup
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}
	hash := sha256.Sum256([]byte(normalized))
	sqlHash := fmt.Sprintf("%x", hash[:])

	saved, err := s.client.QueryHistory.Create().
		SetUserID(h.UserID).
		SetDatasourceID(h.DatasourceID).
		SetDatabase(h.Database).
		SetSQLContent(h.SQLContent).
		SetSQLHash(sqlHash).
		SetParamsJSON(h.ParamsJSON).
		SetSQLSummary(h.SQLSummary).
		SetDbType(h.DBType).
		SetExecutionTime(h.ExecutionTime).
		SetResultRows(h.ResultRows).
		SetAffectedRows(h.AffectedRows).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("insert query history: %w", err)
	}

	h.ID = int64(saved.ID)

	// Auto-cleanup: keep only the latest 200 records per user
	go s.cleanupOldRecords(h.UserID)

	return h.ID, nil
}

// ListHistory returns paginated query history for a user.
func (s *HistoryService) ListHistory(ctx context.Context, userID int64, page, pageSize int, keyword string) ([]model.QueryHistory, int, error) {
	p := sqlutil.ParsePagination(page, pageSize)

	q := s.client.QueryHistory.Query().Where(entqueryhistory.UserIDEQ(userID))
	if keyword != "" {
		// ContainsFold renders as ILIKE and escapes the pattern itself, which
		// replaces a hand-built LIKE with its own ESCAPE clause.
		q = q.Where(entqueryhistory.Or(
			entqueryhistory.SQLContentContainsFold(keyword),
			entqueryhistory.SQLSummaryContainsFold(keyword),
		))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count query history: %w", err)
	}

	rows, err := q.
		Order(ent.Desc(entqueryhistory.FieldID)).
		Limit(p.PageSize).
		Offset(p.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query history: %w", err)
	}

	list := make([]model.QueryHistory, 0, len(rows))
	for _, h := range rows {
		list = append(list, model.QueryHistory{
			ID:            int64(h.ID),
			UserID:        h.UserID,
			DatasourceID:  h.DatasourceID,
			Database:      h.Database,
			SQLContent:    h.SQLContent,
			ParamsJSON:    h.ParamsJSON,
			SQLSummary:    h.SQLSummary,
			DBType:        h.DbType,
			ExecutionTime: h.ExecutionTime,
			ResultRows:    h.ResultRows,
			AffectedRows:  h.AffectedRows,
			CreatedAt:     h.CreatedAt,
		})
	}
	return list, total, nil
}

// DeleteHistory deletes a single query history record (only if it belongs to the user).
func (s *HistoryService) DeleteHistory(ctx context.Context, id, userID int64) error {
	n, err := s.client.QueryHistory.Delete().
		Where(entqueryhistory.ID(int(id)), entqueryhistory.UserID(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete query history: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("记录不存在或无权删除")
	}
	return nil
}

// ClearHistory deletes all query history for a user.
func (s *HistoryService) ClearHistory(ctx context.Context, userID int64) error {
	_, err := s.client.QueryHistory.Delete().
		Where(entqueryhistory.UserID(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("clear query history: %w", err)
	}
	return nil
}

// FrequentQuery represents a deduplicated frequent SQL template.
type FrequentQuery struct {
	SQLContent     string `json:"sql_content"`
	SQLHash        string `json:"sql_hash"`
	Snippet        string `json:"snippet"`
	ExecutionCount int    `json:"execution_count"`
	// time.Time rather than the driver's text: the field held whatever string
	// the SQLite driver produced for a datetime. The frontend parses it with
	// new Date(), which RFC3339 satisfies and the old format did not always.
	LastExecutedAt time.Time `json:"last_executed_at"`
}

const (
	frequentQueryLimit    = 20
	frequentQueryCacheTTL = 5 * time.Minute
)

// frequentQueryCache stores per-user frequent query results.
var (
	fqCache   = make(map[int64][]FrequentQuery)
	fqCacheAt = make(map[int64]time.Time)
	fqMu      sync.RWMutex
)

// GetFrequentQueries returns top 20 frequent SQL templates for the given user.
// Results are deduplicated by SQL hash and cached for 5 minutes.
func (s *HistoryService) GetFrequentQueries(ctx context.Context, userID int64) ([]FrequentQuery, error) {
	// Check cache
	fqMu.RLock()
	if cached, ok := fqCache[userID]; ok && time.Since(fqCacheAt[userID]) < frequentQueryCacheTTL {
		fqMu.RUnlock()
		return cached, nil
	}
	fqMu.RUnlock()

	// Grouped by hash, so every row in a group holds the same SQL and MIN picks
	// one. PostgreSQL requires that choice to be explicit where SQLite returned
	// an arbitrary row's value.
	var rows []struct {
		SQLHash        string    `json:"sql_hash"`
		SQLContent     string    `json:"sql_content"`
		ExecutionCount int       `json:"execution_count"`
		LastExecutedAt time.Time `json:"last_executed_at"`
	}
	err := s.client.QueryHistory.Query().
		Where(
			entqueryhistory.UserIDEQ(userID),
			entqueryhistory.SQLHashNEQ(""),
		).
		Modify(func(sel *entsql.Selector) {
			// ent's typed GroupBy cannot order by an aggregate or limit the
			// result, so this drops to the query builder — the escape hatch
			// ADR-0010 allows, because the typed API has no expression for it.
			sel.Select(
				sel.C(entqueryhistory.FieldSQLHash),
				entsql.As("MIN("+sel.C(entqueryhistory.FieldSQLContent)+")", "sql_content"),
				entsql.As("COUNT(*)", "execution_count"),
				entsql.As("MAX("+sel.C(entqueryhistory.FieldCreatedAt)+")", "last_executed_at"),
			).
				GroupBy(sel.C(entqueryhistory.FieldSQLHash)).
				OrderExpr(entsql.Expr("execution_count DESC")).
				Limit(frequentQueryLimit)
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("query frequent templates: %w", err)
	}

	results := make([]FrequentQuery, 0, len(rows))
	for _, r := range rows {
		results = append(results, FrequentQuery{
			SQLHash:        r.SQLHash,
			SQLContent:     r.SQLContent,
			ExecutionCount: r.ExecutionCount,
			LastExecutedAt: r.LastExecutedAt,
			// Cut on a rune boundary. Slicing bytes splits a multi-byte
			// character and produces replacement characters in the UI — the
			// same defect the audit summary carried.
			Snippet: truncateRunes(r.SQLContent, frequentQuerySnippetRunes),
		})
	}

	// Update cache
	fqMu.Lock()
	fqCache[userID] = results
	fqCacheAt[userID] = time.Now()
	fqMu.Unlock()

	return results, nil
}

// InvalidateFrequentQueryCache clears the frequent query cache (for testing).
func InvalidateFrequentQueryCache() {
	fqMu.Lock()
	fqCache = make(map[int64][]FrequentQuery)
	fqCacheAt = make(map[int64]time.Time)
	fqMu.Unlock()
}

// frequentQuerySnippetRunes bounds the preview shown for a frequent query.
const frequentQuerySnippetRunes = 80

// truncateRunes cuts s to at most n runes.
//
// Runes rather than bytes: SQL carries Chinese identifiers and comments, and
// cutting mid-character renders as a replacement glyph.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// cleanupOldRecords removes records exceeding the per-user limit.
//
// Two statements rather than a NOT IN subquery: the ids to keep are read first,
// then everything else for that user is deleted. The extra round trip buys a
// query whose column names are checked at compile time, and this runs after a
// write rather than on a read path.
func (s *HistoryService) cleanupOldRecords(userID int64) {
	ctx := context.Background()
	keep, err := s.client.QueryHistory.Query().
		Where(entqueryhistory.UserIDEQ(userID)).
		Order(ent.Desc(entqueryhistory.FieldID)).
		Limit(maxHistoryPerUser).
		IDs(ctx)
	if err != nil {
		return
	}
	if len(keep) < maxHistoryPerUser {
		return // nothing to trim
	}
	_, _ = s.client.QueryHistory.Delete().
		Where(
			entqueryhistory.UserIDEQ(userID),
			entqueryhistory.IDNotIn(keep...),
		).
		Exec(ctx)
}
