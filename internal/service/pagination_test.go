package service

import (
	"testing"
)

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name         string
		page         int
		pageSize     int
		wantPage     int
		wantPageSize int
		wantOffset   int
	}{
		{"defaults_zero", 0, 0, 1, 50, 0},
		{"defaults_negative", -1, -5, 1, 50, 0},
		{"normal_page1", 1, 10, 1, 10, 0},
		{"normal_page3_size20", 3, 20, 3, 20, 40},
		{"page2_size50", 2, 50, 2, 50, 50},
		{"page_size_capped_at_100", 1, 200, 1, 100, 0},
		{"page_size_exactly_100", 1, 100, 1, 100, 0},
		{"page_zero_uses_default", 0, 10, 1, 10, 0},
		{"page_size_one", 5, 1, 5, 1, 4},
		{"large_page", 999, 10, 999, 10, 9980},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePagination(tt.page, tt.pageSize)
			if got.Page != tt.wantPage {
				t.Errorf("Page = %d, want %d", got.Page, tt.wantPage)
			}
			if got.PageSize != tt.wantPageSize {
				t.Errorf("PageSize = %d, want %d", got.PageSize, tt.wantPageSize)
			}
			if got.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", got.Offset, tt.wantOffset)
			}
		})
	}
}

func TestBuildWhereClause(t *testing.T) {
	t.Run("empty_filters", func(t *testing.T) {
		clause, args := BuildWhereClause(nil)
		if clause != "" {
			t.Errorf("clause = %q, want empty", clause)
		}
		if args != nil {
			t.Errorf("args = %v, want nil", args)
		}
	})

	t.Run("empty_slice", func(t *testing.T) {
		clause, args := BuildWhereClause([]FilterClause{})
		if clause != "" {
			t.Errorf("clause = %q, want empty", clause)
		}
		if args != nil {
			t.Errorf("args = %v, want nil", args)
		}
	})

	t.Run("single_filter", func(t *testing.T) {
		clause, args := BuildWhereClause([]FilterClause{
			{Condition: "user_id = ?", Args: []interface{}{int64(1)}},
		})
		wantClause := "WHERE user_id = ?"
		if clause != wantClause {
			t.Errorf("clause = %q, want %q", clause, wantClause)
		}
		if len(args) != 1 || args[0].(int64) != 1 {
			t.Errorf("args = %v, want [1]", args)
		}
	})

	t.Run("multiple_filters", func(t *testing.T) {
		clause, args := BuildWhereClause([]FilterClause{
			{Condition: "user_id = ?", Args: []interface{}{int64(1)}},
			{Condition: "status = ?", Args: []interface{}{"active"}},
		})
		wantClause := "WHERE user_id = ? AND status = ?"
		if clause != wantClause {
			t.Errorf("clause = %q, want %q", clause, wantClause)
		}
		if len(args) != 2 {
			t.Fatalf("len(args) = %d, want 2", len(args))
		}
		if args[0].(int64) != 1 {
			t.Errorf("args[0] = %v, want 1", args[0])
		}
		if args[1].(string) != "active" {
			t.Errorf("args[1] = %v, want active", args[1])
		}
	})

	t.Run("filter_with_no_args", func(t *testing.T) {
		clause, args := BuildWhereClause([]FilterClause{
			{Condition: "deleted_at IS NULL", Args: nil},
		})
		wantClause := "WHERE deleted_at IS NULL"
		if clause != wantClause {
			t.Errorf("clause = %q, want %q", clause, wantClause)
		}
		if len(args) != 0 {
			t.Errorf("len(args) = %d, want 0", len(args))
		}
	})
}

func TestPaginatedCountSQL(t *testing.T) {
	tests := []struct {
		name  string
		table string
		where string
		want  string
	}{
		{"no_where", "query_history", "", "SELECT COUNT(*) FROM query_history "},
		{"with_where", "query_history", "WHERE user_id = ?", "SELECT COUNT(*) FROM query_history WHERE user_id = ?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PaginatedCountSQL(tt.table, tt.where)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPaginatedQuerySQL(t *testing.T) {
	p := ParsePagination(2, 20)

	got := PaginatedQuerySQL(
		"SELECT id, name", "users", "WHERE active = 1", "id DESC", p,
	)
	want := "SELECT id, name FROM users WHERE active = 1 ORDER BY id DESC LIMIT ? OFFSET ?"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendLimitArgs(t *testing.T) {
	p := ParsePagination(3, 25)

	t.Run("append_to_empty", func(t *testing.T) {
		args := make([]interface{}, 0)
		result := AppendLimitArgs(args, p)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].(int) != 25 {
			t.Errorf("result[0] = %v, want 25", result[0])
		}
		if result[1].(int) != 50 {
			t.Errorf("result[1] = %v, want 50", result[1])
		}
	})

	t.Run("append_to_existing", func(t *testing.T) {
		args := []interface{}{int64(1), "active"}
		result := AppendLimitArgs(args, p)
		if len(result) != 4 {
			t.Fatalf("len = %d, want 4", len(result))
		}
		if result[0].(int64) != 1 {
			t.Errorf("result[0] = %v, want 1", result[0])
		}
		if result[1].(string) != "active" {
			t.Errorf("result[1] = %v, want active", result[1])
		}
		if result[2].(int) != 25 {
			t.Errorf("result[2] = %v, want 25", result[2])
		}
		if result[3].(int) != 50 {
			t.Errorf("result[3] = %v, want 50", result[3])
		}
	})
}
