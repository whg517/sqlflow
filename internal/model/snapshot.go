package model

import "encoding/json"

// QuerySnapshot represents a saved query result snapshot.
type QuerySnapshot struct {
	ID        int64           `json:"id"`
	UserID    int64           `json:"user_id"`
	Label     string          `json:"label"`
	Columns   json.RawMessage `json:"columns"`
	Rows      json.RawMessage `json:"rows"`
	RowCount  int             `json:"row_count"`
	CreatedAt string          `json:"created_at"`
}
