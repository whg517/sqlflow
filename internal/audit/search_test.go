package audit

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// newSearchService returns a service over a migrated schema with two users.
//
// The users exist because Search joins them for the username column; without a
// row the join still succeeds, so a test with no users could not tell a working
// join from a broken one.
func newSearchService(t *testing.T) *Service {
	t.Helper()
	database := testutil.NewDB(t)
	testutil.SeedUser(t, database.DB, "alice", "developer")
	testutil.SeedUser(t, database.DB, "bob", "admin")
	return NewService(database, 0, 0)
}

// seedSearchData writes the records the keyword assertions are counted against.
func seedSearchData(t *testing.T, svc *Service) {
	t.Helper()
	records := []auditlog.Record{
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM orders WHERE status = 'active'", SQLSummary: "Query active orders"},
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM users WHERE role = 'admin'", SQLSummary: "Query admin users"},
		{UserID: 2, Action: "export", SQLContent: "SELECT * FROM orders WHERE created_at > '2024-01-01'", SQLSummary: "Export recent orders"},
		{UserID: 2, Action: "ticket_create", SQLContent: "UPDATE products SET price = 99.99 WHERE id = 1", SQLSummary: "Update product price"},
		{UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM payments WHERE amount > 1000", SQLSummary: "Query large payments"},
	}
	for _, r := range records {
		svc.Write(context.Background(), r)
	}
}

func TestAuditService_Search_BasicKeyword(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{Keyword: "orders", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total for 'orders' = %d, want 2", result.Total)
	}
	if len(result.Logs) != 2 {
		t.Errorf("logs = %d, want 2", len(result.Logs))
	}
}

// TestAuditService_Search_SubstringWithinToken is the case the trigram index
// exists for in a Latin corpus.
//
// "payment" is not a word in any record — "payments" is — so word matching
// alone reports nothing, and a search box that returns nothing for an obvious
// prefix reads as broken.
func TestAuditService_Search_SubstringWithinToken(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{Keyword: "payment", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total for 'payment' = %d, want 1", result.Total)
	}
}

func TestAuditService_Search_EmptyKeyword(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{Keyword: "", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 0 || len(result.Logs) != 0 {
		t.Errorf("empty keyword returned total=%d logs=%d, want 0/0", result.Total, len(result.Logs))
	}
}

func TestAuditService_Search_WhitespaceKeyword(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{Keyword: "   ", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// A blank keyword must not degrade into "match everything": that would hand
	// the whole audit table to anyone who submits an empty search box.
	if result.Total != 0 {
		t.Errorf("whitespace keyword total = %d, want 0", result.Total)
	}
}

func TestAuditService_Search_FilterByAction(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{
		Keyword: "orders", Page: 1, PageSize: 10, Action: "export",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("total for action=export = %d, want 1", result.Total)
	}
	if len(result.Logs) != 1 || result.Logs[0].Action != "export" {
		t.Errorf("logs = %+v, want a single export record", result.Logs)
	}
}

func TestAuditService_Search_FilterByUserID(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{
		Keyword: "orders", Page: 1, PageSize: 10, UserID: "2",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total for user_id=2 = %d, want 1", result.Total)
	}
}

func TestAuditService_Search_Pagination(t *testing.T) {
	svc := newSearchService(t)
	for i := range 20 {
		svc.Write(t.Context(), auditlog.Record{
			UserID:     1,
			Action:     "query_execute",
			SQLContent: fmt.Sprintf("SELECT * FROM table_%d", i),
		})
	}

	seen := map[int64]bool{}
	for page := 1; page <= 2; page++ {
		result, err := svc.Search(t.Context(), SearchParams{Keyword: "SELECT", Page: page, PageSize: 5})
		if err != nil {
			t.Fatalf("search page %d: %v", page, err)
		}
		if result.Total != 20 {
			t.Errorf("page %d total = %d, want 20", page, result.Total)
		}
		if len(result.Logs) != 5 {
			t.Errorf("page %d logs = %d, want 5", page, len(result.Logs))
		}
		// Pages must not overlap. ts_rank scores these records identically, so
		// without the id tiebreaker the order is undefined and a row can appear
		// on both pages while another never appears at all.
		for _, l := range result.Logs {
			if seen[l.ID] {
				t.Errorf("record %d returned on more than one page", l.ID)
			}
			seen[l.ID] = true
		}
	}
	if len(seen) != 10 {
		t.Errorf("distinct records across two pages = %d, want 10", len(seen))
	}
}

func TestAuditService_Search_HighlightFields(t *testing.T) {
	svc := newSearchService(t)
	svc.Write(t.Context(), auditlog.Record{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT * FROM orders WHERE status = 'active'",
		SQLSummary: "Query active orders",
	})

	result, err := svc.Search(t.Context(), SearchParams{Keyword: "orders", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Logs) != 1 {
		t.Fatalf("logs = %d, want 1", len(result.Logs))
	}
	got := result.Logs[0]
	if !strings.Contains(got.HighlightSQLContent, "<mark>orders</mark>") {
		t.Errorf("highlight_sql_content = %q, want the keyword marked", got.HighlightSQLContent)
	}
	if got.SQLContent != "SELECT * FROM orders WHERE status = 'active'" {
		t.Errorf("sql_content = %q, want the record unmodified", got.SQLContent)
	}
	// Ranking runs the other way round from SQLite's FTS5, where a smaller rank
	// meant a better match: ts_rank scores relevance upward.
	if got.Rank <= 0 {
		t.Errorf("rank = %f, want a positive relevance score", got.Rank)
	}
}

func TestAuditService_Search_NoResults(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{
		Keyword: "nonexistent_table_xyz", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 0 || len(result.Logs) != 0 {
		t.Errorf("total=%d logs=%d, want 0/0", result.Total, len(result.Logs))
	}
}

func TestAuditService_Search_FilterByTimeRange(t *testing.T) {
	svc := newSearchService(t)
	svc.Write(t.Context(), auditlog.Record{
		UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM orders",
	})

	result, err := svc.Search(t.Context(), SearchParams{
		Keyword: "orders", Page: 1, PageSize: 10, Start: "2000-01-01", End: "2099-12-31",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("within range total = %d, want 1", result.Total)
	}

	outside, err := svc.Search(t.Context(), SearchParams{
		Keyword: "orders", Page: 1, PageSize: 10, Start: "2099-12-31", End: "2100-12-31",
	})
	if err != nil {
		t.Fatalf("search outside range: %v", err)
	}
	if outside.Total != 0 {
		t.Errorf("outside range total = %d, want 0", outside.Total)
	}
}

// TestAuditService_Search_EndDateIncludesItsOwnDay guards the off-by-one a
// date-only bound invites: comparing created_at <= '2026-08-04' resolves the
// bound to midnight and silently drops everything recorded that day — which is
// the day an operator investigating an incident is looking at.
func TestAuditService_Search_EndDateIncludesItsOwnDay(t *testing.T) {
	svc := newSearchService(t)
	svc.Write(t.Context(), auditlog.Record{
		UserID: 1, Action: "query_execute", SQLContent: "SELECT * FROM orders",
	})

	var today string
	if err := svc.database.DB.QueryRow(`SELECT to_char(now(), 'YYYY-MM-DD')`).Scan(&today); err != nil {
		t.Fatalf("read current date: %v", err)
	}

	result, err := svc.Search(t.Context(), SearchParams{
		Keyword: "orders", Page: 1, PageSize: 10, Start: today, End: today,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total for End=today = %d, want 1", result.Total)
	}
}

// TestAuditService_Search_ChineseKeyword covers what the tokenizer cannot do.
//
// to_tsvector reports a run of Chinese as one token, so 订单状态 does not
// contain a token 订单 and word matching finds nothing. The trigram index is
// what makes this query return the record.
func TestAuditService_Search_ChineseKeyword(t *testing.T) {
	svc := newSearchService(t)
	svc.Write(t.Context(), auditlog.Record{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT * FROM orders WHERE 订单状态 = 'active'",
		SQLSummary: "查询订单明细",
	})

	result, err := svc.Search(t.Context(), SearchParams{Keyword: "订单", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search chinese: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("total for 订单 = %d, want 1", result.Total)
	}
	if !strings.Contains(result.Logs[0].HighlightSQLContent, "<mark>订单</mark>") {
		t.Errorf("highlight = %q, want the keyword marked", result.Logs[0].HighlightSQLContent)
	}
}

// TestAuditService_Search_KeywordIsNotAPattern checks that a keyword made of
// LIKE metacharacters is matched literally.
//
// The trigram path compares with ILIKE, so an unescaped % would match every
// record — turning the search box into a way to read the entire audit log
// regardless of what was typed.
func TestAuditService_Search_KeywordIsNotAPattern(t *testing.T) {
	svc := newSearchService(t)
	seedSearchData(t, svc)

	result, err := svc.Search(t.Context(), SearchParams{Keyword: "%", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total for '%%' = %d, want 0 — the wildcard leaked into the pattern", result.Total)
	}
}
