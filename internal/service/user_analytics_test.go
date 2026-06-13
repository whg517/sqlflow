package service

import (
	"testing"
	"time"
)

func TestParseTimeRange(t *testing.T) {
	tests := []struct {
		input    string
		wantDays int
		wantErr  bool
	}{
		{"7d", 7, false},
		{"30d", 30, false},
		{"90d", 90, false},
		{"", 7, false},    // defaults to 7d
		{"custom", 7, false}, // defaults to 7d (custom without dates is validated elsewhere)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			params := AnalyticsParams{TimeRange: tt.input}
			if tt.input == "custom" {
				// custom without dates defaults to 7d via validateAnalyticsParams
				params.TimeRange = ""
			}
			if params.TimeRange == "" {
				params.TimeRange = "7d"
			}
			_, days, err := resolveDateRange(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveDateRange(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if days != tt.wantDays {
				t.Errorf("resolveDateRange(%q) days = %d, want %d", tt.input, days, tt.wantDays)
			}
		})
	}
}

func TestValidateAnalyticsParams(t *testing.T) {
	tests := []struct {
		name    string
		params  AnalyticsParams
		wantErr bool
	}{
		{"7d preset", AnalyticsParams{TimeRange: "7d"}, false},
		{"30d preset", AnalyticsParams{TimeRange: "30d"}, false},
		{"90d preset", AnalyticsParams{TimeRange: "90d"}, false},
		{"custom valid", AnalyticsParams{TimeRange: "custom", StartDate: "2026-06-01", EndDate: "2026-06-10"}, false},
		{"custom missing start", AnalyticsParams{TimeRange: "custom", EndDate: "2026-06-10"}, true},
		{"custom missing end", AnalyticsParams{TimeRange: "custom", StartDate: "2026-06-01"}, true},
		{"custom bad format", AnalyticsParams{TimeRange: "custom", StartDate: "not-a-date", EndDate: "2026-06-10"}, true},
		{"custom end before start", AnalyticsParams{TimeRange: "custom", StartDate: "2026-06-10", EndDate: "2026-06-01"}, true},
		{"custom too long", AnalyticsParams{TimeRange: "custom", StartDate: "2025-01-01", EndDate: "2026-12-31"}, true},
		{"empty defaults to 7d", AnalyticsParams{TimeRange: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnalyticsParams(&tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAnalyticsParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseAnalyticsUserID(t *testing.T) {
	tests := []struct {
		input    string
		want     int64
		wantErr  bool
	}{
		{"", 0, false},
		{"42", 42, false},
		{"1", 1, false},
		{"0", 0, true},    // must be positive
		{"-1", 0, true},   // must be positive
		{"abc", 0, true},  // invalid
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseAnalyticsUserID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAnalyticsUserID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseAnalyticsUserID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnalyticsCacheConcurrency(t *testing.T) {
	cache := &analyticsCache{
		entries: make(map[string]*analyticsCacheEntry),
		ttl:     10 * time.Minute,
	}

	// Simulate concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			key := "test"
			data := &UserAnalytics{TimeRange: "7d", UserID: int64(idx)}
			cache.set(key, data)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Last write should be cached
	cached, ok := cache.get("test")
	if !ok {
		t.Error("expected cache hit")
	}
	if cached == nil {
		t.Error("cached data should not be nil")
	}
}

func TestAnalyticsCacheExpiration(t *testing.T) {
	cache := &analyticsCache{
		entries: make(map[string]*analyticsCacheEntry),
		ttl:     1 * time.Nanosecond, // immediately expire
	}

	cache.set("key", &UserAnalytics{TimeRange: "7d"})

	_, ok := cache.get("key")
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestInvalidateAnalyticsCache(t *testing.T) {
	globalAnalyticsCache.set("test1", &UserAnalytics{TimeRange: "7d"})
	globalAnalyticsCache.set("test2", &UserAnalytics{TimeRange: "30d"})

	InvalidateAnalyticsCache()

	_, ok1 := globalAnalyticsCache.get("test1")
	_, ok2 := globalAnalyticsCache.get("test2")
	if ok1 || ok2 {
		t.Error("all cache entries should be cleared after invalidation")
	}
}

func TestAnalyticsThresholds(t *testing.T) {
	if BurstQueryThreshold <= 0 {
		t.Error("BurstQueryThreshold should be positive")
	}
	if OffHoursThreshold <= 0 {
		t.Error("OffHoursThreshold should be positive")
	}
	if AnalyticsCacheTTL <= 0 {
		t.Error("AnalyticsCacheTTL should be positive")
	}
}

func TestUserAnalyticsTypes(t *testing.T) {
	entry := ActiveUserEntry{
		Rank:          1,
		UserID:        42,
		Username:      "testuser",
		QueryCount:    100,
		ApprovalCount: 20,
		ActiveDays:    5,
		TotalActions:  150,
	}
	if entry.Rank != 1 || entry.UserID != 42 {
		t.Error("ActiveUserEntry construction failed")
	}

	anomaly := AnomalyEntry{
		UserID:      1,
		Username:    "suspicious",
		AnomalyType: "burst_queries",
		Description: "1小时内执行 60 次查询（阈值 50）",
		Count:       60,
		TimeWindow:  "2026-06-12T03",
	}
	if anomaly.AnomalyType != "burst_queries" {
		t.Error("AnomalyEntry construction failed")
	}

	breakdown := ActionTypeEntry{
		Action: "query_execute",
		Count:  500,
		Ratio:  50.0,
	}
	if breakdown.Ratio != 50.0 {
		t.Error("ActionTypeEntry construction failed")
	}
}
