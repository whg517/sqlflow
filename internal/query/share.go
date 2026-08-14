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
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrShareNotFound      = errors.New("共享链接不存在")
	ErrShareExpired       = errors.New("共享链接已过期")
	ErrShareRevoked       = errors.New("共享链接已撤销")
	ErrSharePassword      = errors.New("密码错误")
	ErrShareRowLimit      = errors.New("共享数据超过 10000 行上限")
	ErrShareExpiryTooLong = errors.New("过期时间不能超过 7 天")
	// ErrShareUnmaskable means a stored share has no scope to look rules up by,
	// so nothing can be said about what its rows expose.
	ErrShareUnmaskable = errors.New("共享结果缺少脱敏所需的来源信息，已拒绝返回")
)

const (
	shareMaxExpiry       = 7 * 24 * time.Hour
	shareAccessTokenTTL  = 15 * time.Minute
	shareAccessTokenType = "v1"
)

// ShareScope is where a published result came from.
type ShareScope struct {
	Database string
	Targets  []string
}

// ShareScopeResolver derives the scope a result belongs to, and refuses to
// derive one the actor may not read.
//
// Declared here at the point of use rather than by taking the whole query
// Service: publishing needs one answer — which database and which tables — and
// naming just that keeps the two from tangling. It is also why the scope cannot
// come from the request: a client that named no targets would publish rows no
// rule could ever match.
type ShareScopeResolver interface {
	ResolveShareScope(ctx context.Context, userID int64, role string, datasourceID int64, sqlContent string) (ShareScope, error)
}

// ShareService handles shared query results.
type ShareService struct {
	limits       Limits
	database     *db.DB
	client       *ent.Client
	accessSecret []byte
	scope        ShareScopeResolver
	auditSvc     auditlog.Writer
}

// NewShareService creates a new ShareService.
func NewShareService(database *db.DB, accessSecret string, scope ShareScopeResolver, auditSvc auditlog.Writer, limits Limits) *ShareService {
	return &ShareService{
		limits:       limits.withDefaults(),
		database:     database,
		client:       database.Client(),
		accessSecret: []byte(accessSecret),
		scope:        scope,
		auditSvc:     auditlog.OrDiscard(auditSvc),
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
	if len(req.Rows) > s.limits.ShareMaxRows {
		return nil, ErrShareRowLimit
	}

	maxExpiry := time.Now().Add(shareMaxExpiry)
	if req.ExpiresAt.After(maxExpiry) {
		return nil, ErrShareExpiryTooLong
	}

	// Record where the rows came from, so GetShare can protect them. The actor
	// is re-authorized here rather than trusted: publishing is a second entrance
	// to the same data and invariant 1 gives it no exemption.
	scope, err := s.scope.ResolveShareScope(ctx, req.UserID, req.Role, req.DatasourceID, req.SQLContent)
	if err != nil {
		s.auditSvc.Write(ctx, auditlog.Record{
			UserID:       req.UserID,
			Action:       "query_share_rejected",
			DatasourceID: req.DatasourceID,
			SQLContent:   req.SQLContent,
			SQLSummary:   auditlog.Summarize(req.SQLContent),
			ErrorMessage: err.Error(),
		})
		return nil, err
	}
	targetsJSON, err := json.Marshal(scope.Targets)
	if err != nil {
		return nil, fmt.Errorf("marshal targets: %w", err)
	}

	// The summary is derived, never accepted. The client used to send
	// `currentSql.trim().substring(0, 200)` — the raw statement, verbatim — and
	// it was handed to anyone holding the link, while the server had Summarize
	// all along and this path simply never called it.
	summary := auditlog.Summarize(req.SQLContent)

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
		SetSQLSummary(summary).
		SetDatasourceID(req.DatasourceID).
		SetDatabase(scope.Database).
		SetTargetsJSON(string(targetsJSON)).
		SetRevoked(false).
		Save(ctx)
	if err != nil {
		log.Printf("CreateShare failed: %v", err)
		return nil, fmt.Errorf("创建共享链接失败")
	}

	// Publishing rows at an unauthenticated URL is the most consequential thing
	// this package does and it left no trace at all.
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:       req.UserID,
		Action:       "query_share",
		DatasourceID: req.DatasourceID,
		Database:     scope.Database,
		SQLContent:   req.SQLContent,
		SQLSummary:   summary,
		ResultRows:   int64(len(req.Rows)),
	})

	result := &model.SharedResult{
		ID:           int64(saved.ID),
		UserID:       req.UserID,
		Username:     req.Username,
		Token:        token,
		ColumnsJSON:  string(columnsJSON),
		RowsJSON:     string(rowsJSON),
		RowCount:     int64(len(req.Rows)),
		ExpiresAt:    req.ExpiresAt,
		PasswordHash: passwordHash,
		SQLSummary:   summary,
		DatasourceID: req.DatasourceID,
		Database:     scope.Database,
		Revoked:      false,
		CreatedAt:    saved.CreatedAt,
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

	if err := s.maskForAnonymousReader(ctx, sr, rows); err != nil {
		return nil, err
	}

	return &model.SharedResultPublic{
		ID:            int64(sr.ID),
		Columns:       columns,
		Rows:          rows,
		RowCount:      sr.RowCount,
		SQLSummary:    sr.SQLSummary,
		ExpiresAt:     sr.ExpiresAt.Format(time.RFC3339),
		HasPassword:   hasPassword,
		AccessGranted: true,
		CreatedAt:     sr.CreatedAt.Format(time.RFC3339),
	}, nil
}

// maskForAnonymousReader applies the current rules to the stored rows, in place.
//
// The rules are consulted on every read rather than baked in at creation, and
// that difference is the point. The reader of /s/:token is anonymous and holds
// no grant, so every rule matching the recorded targets applies — including one
// created after the link was published, and including the case the old design
// could not address at all: a publisher who held an unmask grant stored raw
// values, and those values went to the public URL exactly as they were.
//
// CLAUDE.md says query, export and share share one rule set. Until the record
// carried a datasource, a database and a target list, share consulted none.
func (s *ShareService) maskForAnonymousReader(ctx context.Context, sr *ent.SharedResult, rows []map[string]interface{}) error {
	var targets []string
	if err := json.Unmarshal([]byte(sr.TargetsJSON), &targets); err != nil {
		return fmt.Errorf("解析共享目标失败")
	}
	if sr.DatasourceID == 0 {
		// No datasource at all means we cannot reason about where the rows came
		// from. A share is only created through CreateShare, which always records
		// the datasource, so this is unreachable from the live path — but serving
		// rows we cannot reason about is the failure this whole change exists to
		// remove.
		return ErrShareUnmaskable
	}
	if len(targets) == 0 {
		// The query named no table targets (e.g. `SELECT 1`, `SELECT now()`).
		// Such a result cannot carry field-level data from any table, so no mask
		// rule can apply to it — it is safe to publish as-is. Refusing here used
		// to turn every target-less share into a 500 after the password check.
		return nil
	}

	// The same decision every other reader crosses, entered with an actor that
	// holds nothing — which is the honest description of an anonymous holder of
	// a link, and is why the share path no longer needs its own copy of the
	// application loop. A refusal is a refusal here too: a shape that cannot be
	// masked must not be published.
	decision, err := releaseRows(ctx, s.client, nil, releaseActor{},
		sr.DatasourceID, sr.Database, targets, driver.ShapeTable, rows)
	if err != nil {
		return err
	}
	if decision.Verdict == releaseRefuse {
		return decision.Reason
	}
	return nil
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
			ID:           int64(sr.ID),
			UserID:       sr.UserID,
			Username:     sr.Username,
			Token:        sr.Token,
			RowCount:     sr.RowCount,
			ExpiresAt:    sr.ExpiresAt,
			SQLSummary:   sr.SQLSummary,
			DatasourceID: sr.DatasourceID,
			Database:     sr.Database,
			Revoked:      sr.Revoked,
			RevokedAt:    sr.RevokedAt,
			CreatedAt:    sr.CreatedAt,
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
	UserID   int64
	Username string
	// Role is what the scope resolver re-authorizes against.
	Role      string
	Columns   []string
	Rows      []map[string]interface{}
	ExpiresAt time.Time
	Password  string

	// DatasourceID and SQLContent are what the scope is derived from. Neither a
	// summary nor a datasource name is accepted from the caller any more: the
	// summary is produced by auditlog.Summarize here, and the datasource is
	// identified by id so mask rules can actually be found.
	DatasourceID int64
	SQLContent   string
}
