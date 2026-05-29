package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/whg517/sqlflow/internal/model"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrShareNotFound = errors.New("共享链接不存在")
	ErrShareExpired = errors.New("共享链接已过期")
	ErrShareRevoked = errors.New("共享链接已撤销")
	ErrSharePassword = errors.New("密码错误")
	ErrShareRowLimit = errors.New("共享数据超过 10000 行上限")
	ErrShareExpiryTooLong = errors.New("过期时间不能超过 7 天")
)

const (
	shareMaxRows    = 10000
	shareMaxExpiry  = 7 * 24 * time.Hour
)

// ShareService handles shared query results.
type ShareService struct {
	db *sql.DB
}

// NewShareService creates a new ShareService.
func NewShareService(db *sql.DB) *ShareService {
	return &ShareService{db: db}
}

// generateToken generates a cryptographically random 16-byte hex token.
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateShare creates a new shared result link.
func (s *ShareService) CreateShare(ctx context.Context, req *CreateShareRequest) (*model.SharedResult, error) {
	if len(req.Rows) > shareMaxRows {
		return nil, ErrShareRowLimit
	}

	maxExpiry := time.Now().Add(shareMaxExpiry)
	if req.ExpiresAt.After(maxExpiry) {
		return nil, ErrShareExpiryTooLong
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	columnsJSON, err := json.Marshal(req.Columns)
	if err != nil {
		return nil, fmt.Errorf("marshal columns: %w", err)
	}

	rowsJSON, err := json.Marshal(req.Rows)
	if err != nil {
		return nil, fmt.Errorf("marshal rows: %w", err)
	}

	var passwordHash string
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		passwordHash = string(hash)
	}

	result := &model.SharedResult{
		UserID:         req.UserID,
		Username:       req.Username,
		Token:          token,
		ColumnsJSON:    string(columnsJSON),
		RowsJSON:       string(rowsJSON),
		RowCount:       int64(len(req.Rows)),
		ExpiresAt:      req.ExpiresAt,
		PasswordHash:   passwordHash,
		SQLSummary:     req.SQLSummary,
		DatasourceName: req.DatasourceName,
		Revoked:        false,
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO shared_results (user_id, username, token, columns_json, rows_json, row_count, expires_at, password_hash, sql_summary, datasource_name, revoked)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		result.UserID, result.Username, result.Token, result.ColumnsJSON, result.RowsJSON,
		result.RowCount, result.ExpiresAt, result.PasswordHash, result.SQLSummary, result.DatasourceName,
	)
	if err != nil {
		log.Printf("CreateShare failed: %v", err)
		return nil, fmt.Errorf("创建共享链接失败")
	}

	id, _ := res.LastInsertId()
	result.ID = id

	return result, nil
}

// GetShare retrieves a shared result by token (public, no auth).
func (s *ShareService) GetShare(ctx context.Context, token string) (*model.SharedResultPublic, error) {
	var sr model.SharedResult
	var revoked int

	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, username, token, columns_json, rows_json, row_count, expires_at, password_hash, sql_summary, datasource_name, revoked, revoked_at, created_at
		 FROM shared_results WHERE token = ?`, token,
	).Scan(&sr.ID, &sr.UserID, &sr.Username, &sr.Token, &sr.ColumnsJSON, &sr.RowsJSON,
		&sr.RowCount, &sr.ExpiresAt, &sr.PasswordHash, &sr.SQLSummary, &sr.DatasourceName,
		&revoked, &sr.RevokedAt, &sr.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrShareNotFound
	}
	if err != nil {
		log.Printf("GetShare failed: %v", err)
		return nil, fmt.Errorf("获取共享链接失败")
	}

	sr.Revoked = revoked == 1

	// Check expiry
	if time.Now().After(sr.ExpiresAt) {
		return nil, ErrShareExpired
	}

	// Check if revoked
	if sr.Revoked {
		return nil, ErrShareRevoked
	}

	// Parse JSON fields
	var columns []string
	if err := json.Unmarshal([]byte(sr.ColumnsJSON), &columns); err != nil {
		return nil, fmt.Errorf("解析列数据失败")
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(sr.RowsJSON), &rows); err != nil {
		return nil, fmt.Errorf("解析行数据失败")
	}

	return &model.SharedResultPublic{
		ID:             sr.ID,
		Columns:        columns,
		Rows:           rows,
		RowCount:       sr.RowCount,
		SQLSummary:     sr.SQLSummary,
		DatasourceName: sr.DatasourceName,
		ExpiresAt:      sr.ExpiresAt.Format(time.RFC3339),
		HasPassword:    sr.PasswordHash != "",
		CreatedAt:      sr.CreatedAt.Format(time.RFC3339),
	}, nil
}

// VerifyPassword checks if the provided password matches the shared result.
func (s *ShareService) VerifyPassword(ctx context.Context, token, password string) error {
	var passwordHash string
	err := s.db.QueryRowContext(ctx,
		`SELECT password_hash FROM shared_results WHERE token = ?`, token,
	).Scan(&passwordHash)
	if err == sql.ErrNoRows {
		return ErrShareNotFound
	}
	if err != nil {
		return fmt.Errorf("验证密码失败")
	}

	if passwordHash == "" {
		return nil // no password set
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return ErrSharePassword
	}

	return nil
}

// ListMyShares lists all shared results for a user.
func (s *ShareService) ListMyShares(ctx context.Context, userID int64) ([]model.SharedResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, username, token, row_count, expires_at, sql_summary, datasource_name, revoked, revoked_at, created_at
		 FROM shared_results WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		log.Printf("ListMyShares failed: %v", err)
		return nil, fmt.Errorf("获取共享列表失败")
	}
	defer rows.Close()

	var results []model.SharedResult
	for rows.Next() {
		var sr model.SharedResult
		var revoked int
		if err := rows.Scan(&sr.ID, &sr.UserID, &sr.Username, &sr.Token, &sr.RowCount,
			&sr.ExpiresAt, &sr.SQLSummary, &sr.DatasourceName, &revoked, &sr.RevokedAt, &sr.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan shared result: %w", err)
		}
		sr.Revoked = revoked == 1
		results = append(results, sr)
	}

	return results, nil
}

// RevokeShare revokes a shared result by ID (only the owner can revoke).
func (s *ShareService) RevokeShare(ctx context.Context, id, userID int64) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE shared_results SET revoked = 1, revoked_at = datetime('now') WHERE id = ? AND user_id = ? AND revoked = 0`,
		id, userID,
	)
	if err != nil {
		log.Printf("RevokeShare failed: %v", err)
		return fmt.Errorf("撤销共享链接失败")
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrShareNotFound
	}

	return nil
}

// CleanupExpired removes expired shared results (for periodic cleanup).
func (s *ShareService) CleanupExpired(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM shared_results WHERE expires_at < datetime('now') AND revoked = 0`,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired shares: %w", err)
	}
	return result.RowsAffected()
}

// CreateShareRequest is the input for creating a shared result.
type CreateShareRequest struct {
	UserID         int64
	Username       string
	Columns        []string
	Rows           []map[string]interface{}
	ExpiresAt      time.Time
	Password       string
	SQLSummary     string
	DatasourceName string
}
