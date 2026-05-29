package model

import "encoding/json"

// QuerySnapshot represents a saved query result snapshot.
type QuerySnapshot struct {
	ID              int64           `json:"id"`
	UserID          int64           `json:"user_id"`
	QueryHistoryID  int64           `json:"query_history_id"`
	Label           string          `json:"label,omitempty"`
	Columns         json.RawMessage `json:"columns_json"`
	Rows            json.RawMessage `json:"rows_json"`
	RowCount        int             `json:"total_rows"`
	CreatedAt       string          `json:"created_at"`
	// Joined from query_history for display
	SQLContent  string `json:"sql_content,omitempty"`
	SQLSummary  string `json:"sql_summary,omitempty"`
	Database    string `json:"database,omitempty"`
}
