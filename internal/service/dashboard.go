package service

import (
	"context"
	"database/sql"
	"fmt"
)

// DashboardStats holds aggregated statistics for the dashboard overview.
type DashboardStats struct {
	PendingTickets    int `json:"pending_tickets"`
	RecentQueries7d   int `json:"recent_queries_7d"`
	ActiveDatasources int `json:"active_datasources"`
	TotalUsers        int `json:"total_users"`
}

// DashboardService provides dashboard statistics.
type DashboardService struct {
	db *sql.DB
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(db *sql.DB) *DashboardService {
	return &DashboardService{db: db}
}

// GetStats returns aggregated dashboard statistics.
func (s *DashboardService) GetStats(ctx context.Context) (*DashboardStats, error) {
	stats := &DashboardStats{}

	// PendingTickets: tickets with status in (SUBMITTED, AI_REVIEWED, PENDING_APPROVAL)
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE status IN ('SUBMITTED', 'AI_REVIEWED', 'PENDING_APPROVAL')`,
	).Scan(&stats.PendingTickets)
	if err != nil {
		return nil, fmt.Errorf("query pending tickets: %w", err)
	}

	// RecentQueries7d: query_history in the last 7 days
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM query_history WHERE created_at >= datetime('now', '-7 days')`,
	).Scan(&stats.RecentQueries7d)
	if err != nil {
		return nil, fmt.Errorf("query recent queries: %w", err)
	}

	// ActiveDatasources: datasources with status = 'active'
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM datasources WHERE status = 'active'`,
	).Scan(&stats.ActiveDatasources)
	if err != nil {
		return nil, fmt.Errorf("query active datasources: %w", err)
	}

	// TotalUsers: total user count
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users`,
	).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("query total users: %w", err)
	}

	return stats, nil
}
