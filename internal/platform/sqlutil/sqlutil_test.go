package sqlutil

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
