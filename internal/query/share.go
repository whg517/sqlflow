package query

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	"github.com/whg517/sqlflow/internal/db/ent/sharedresult"
	"github.com/whg517/sqlflow/internal/model"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrShareNotFound      = errors.New("共享链接不存在")
	ErrShareExpired       = errors.New("共享链接已过期")
	ErrShareRevoked       = errors.New("共享链接已撤销")
	ErrSharePassword      = errors.New("密码错误")
	ErrShareRowLimit      = errors.New("共享数据超过 10000 行上限")
	ErrShareExpiryTooLong = errors.New("过期时间不能超过 7 天")
)

const (
	shareMaxRows         = 10000
	shareMaxExpiry       = 7 * 24 * time.Hour
	shareAccessTokenTTL  = 15 * time.Minute
	shareAccessTokenType = "v1"
)

// ShareService handles shared query results.
type ShareService struct {
	database     *db.DB
	client       *ent.Client
	accessSecret []byte
}

// NewShareService creates a new ShareService.
func NewShareService(database *db.DB, accessSecret string) *ShareService {
	return &ShareService{
		database:     database,
		client:       database.Client(),
		accessSecret: []byte(accessSecret),
	}
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

	saved, err := s.client.SharedResult.Create().
		SetUserID(req.UserID).
		SetUsername(req.Username).
		SetToken(token).
		SetColumnsJSON(string(columnsJSON)).
		SetRowsJSON(string(rowsJSON)).
		SetRowCount(int64(len(req.Rows))).
		SetExpiresAt(req.ExpiresAt).
		SetPasswordHash(passwordHash).
		SetSQLSummary(req.SQLSummary).
		SetDatasourceName(req.DatasourceName).
		SetRevoked(false).
		Save(ctx)
	if err != nil {
		log.Printf("CreateShare failed: %v", err)
		return nil, fmt.Errorf("创建共享链接失败")
	}

	result := &model.SharedResult{
		ID:             int64(saved.ID),
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
		CreatedAt:      saved.CreatedAt,
	}

	return result, nil
}

// GetShare retrieves a shared result by token. Password-protected shares only
// include result data when accessProof is a valid short-lived proof issued by
// VerifyPassword.
func (s *ShareService) GetShare(ctx context.Context, token, accessProof string) (*model.SharedResultPublic, error) {
	sr, err := s.client.SharedResult.Query().
		Where(sharedresult.Token(token)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrShareNotFound
		}
		log.Printf("GetShare failed: %v", err)
		return nil, fmt.Errorf("获取共享链接失败")
	}

	// Check expiry
	if time.Now().After(sr.ExpiresAt) {
		return nil, ErrShareExpired
	}

	// Check if revoked
	if sr.Revoked {
		return nil, ErrShareRevoked
	}

	hasPassword := sr.PasswordHash != ""
	accessGranted := !hasPassword || s.verifyAccessProof(accessProof, int64(sr.ID), token)
	if !accessGranted {
		return &model.SharedResultPublic{
			Columns:       make([]string, 0),
			Rows:          make([]map[string]interface{}, 0),
			ExpiresAt:     sr.ExpiresAt.Format(time.RFC3339),
			HasPassword:   true,
			AccessGranted: false,
			CreatedAt:     sr.CreatedAt.Format(time.RFC3339),
		}, nil
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
		ID:             int64(sr.ID),
		Columns:        columns,
		Rows:           rows,
		RowCount:       sr.RowCount,
		SQLSummary:     sr.SQLSummary,
		DatasourceName: sr.DatasourceName,
		ExpiresAt:      sr.ExpiresAt.Format(time.RFC3339),
		HasPassword:    hasPassword,
		AccessGranted:  true,
		CreatedAt:      sr.CreatedAt.Format(time.RFC3339),
	}, nil
}

// VerifyPassword checks if the provided password matches the shared result and
// returns a short-lived proof bound to that exact share.
func (s *ShareService) VerifyPassword(ctx context.Context, token, password string) (string, error) {
	sr, err := s.client.SharedResult.Query().
		Where(sharedresult.Token(token)).
		Select(
			sharedresult.FieldID,
			sharedresult.FieldPasswordHash,
			sharedresult.FieldExpiresAt,
			sharedresult.FieldRevoked,
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", ErrShareNotFound
		}
		return "", fmt.Errorf("验证密码失败")
	}

	if time.Now().After(sr.ExpiresAt) {
		return "", ErrShareExpired
	}
	if sr.Revoked {
		return "", ErrShareRevoked
	}

	if sr.PasswordHash == "" {
		return "", nil // no password set
	}

	if err := bcrypt.CompareHashAndPassword([]byte(sr.PasswordHash), []byte(password)); err != nil {
		return "", ErrSharePassword
	}

	expiresAt := time.Now().Add(shareAccessTokenTTL)
	if sr.ExpiresAt.Before(expiresAt) {
		expiresAt = sr.ExpiresAt
	}
	return s.generateAccessProof(int64(sr.ID), token, expiresAt), nil
}

func (s *ShareService) generateAccessProof(shareID int64, token string, expiresAt time.Time) string {
	expiresUnix := strconv.FormatInt(expiresAt.Unix(), 10)
	payload := strings.Join([]string{shareAccessTokenType, strconv.FormatInt(shareID, 10), token, expiresUnix}, "|")
	mac := hmac.New(sha256.New, s.accessSecret)
	_, _ = mac.Write([]byte(payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return shareAccessTokenType + "." + expiresUnix + "." + signature
}

func (s *ShareService) verifyAccessProof(proof string, shareID int64, token string) bool {
	parts := strings.Split(proof, ".")
	if len(parts) != 3 || parts[0] != shareAccessTokenType {
		return false
	}

	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() >= expiresUnix {
		return false
	}

	payload := strings.Join([]string{shareAccessTokenType, strconv.FormatInt(shareID, 10), token, parts[1]}, "|")
	mac := hmac.New(sha256.New, s.accessSecret)
	_, _ = mac.Write([]byte(payload))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	return hmac.Equal(actual, expected)
}

// ListMyShares lists all shared results for a user.
func (s *ShareService) ListMyShares(ctx context.Context, userID int64) ([]model.SharedResult, error) {
	results, err := s.client.SharedResult.Query().
		Where(sharedresult.UserID(userID)).
		Order(sharedresult.ByCreatedAt(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		log.Printf("ListMyShares failed: %v", err)
		return nil, fmt.Errorf("获取共享列表失败")
	}

	var out []model.SharedResult
	for _, sr := range results {
		out = append(out, model.SharedResult{
			ID:             int64(sr.ID),
			UserID:         sr.UserID,
			Username:       sr.Username,
			Token:          sr.Token,
			RowCount:       sr.RowCount,
			ExpiresAt:      sr.ExpiresAt,
			SQLSummary:     sr.SQLSummary,
			DatasourceName: sr.DatasourceName,
			Revoked:        sr.Revoked,
			RevokedAt:      sr.RevokedAt,
			CreatedAt:      sr.CreatedAt,
		})
	}

	return out, nil
}

// RevokeShare revokes a shared result by ID (only the owner can revoke).
func (s *ShareService) RevokeShare(ctx context.Context, id, userID int64) error {
	n, err := s.client.SharedResult.Update().
		Where(
			sharedresult.ID(int(id)),
			sharedresult.UserID(userID),
			sharedresult.Revoked(false),
		).
		SetRevoked(true).
		SetRevokedAt(time.Now()).
		Save(ctx)
	if err != nil {
		log.Printf("RevokeShare failed: %v", err)
		return fmt.Errorf("撤销共享链接失败")
	}

	if n == 0 {
		return ErrShareNotFound
	}

	return nil
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
