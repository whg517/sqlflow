package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserNotFound       = errors.New("用户不存在")
	ErrUsernameExists     = errors.New("用户名已存在")
	ErrForbidden          = errors.New("权限不足")
	ErrCannotEditSelf     = errors.New("不能编辑自己的角色")
	ErrCannotDeleteSelf   = errors.New("不能删除自己")
	ErrLastAdmin          = errors.New("不能删除最后一个管理员")
	ErrInvalidRole        = errors.New("角色必须是 admin、dba 或 developer")
)

var ValidRoles = map[string]bool{"admin": true, "dba": true, "developer": true}

// AuthService handles authentication logic.
type AuthService struct {
	db              *sql.DB
	jwtSecret       []byte
	jwtExpiry       time.Duration
	refreshTokenSvc *RefreshTokenService
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *sql.DB, jwtSecret string, jwtExpiry time.Duration) *AuthService {
	return &AuthService{
		db:              db,
		jwtSecret:       []byte(jwtSecret),
		jwtExpiry:       jwtExpiry,
		refreshTokenSvc: NewRefreshTokenService(db),
	}
}

// RefreshTokenService returns the embedded refresh token service.
func (s *AuthService) RefreshTokenService() *RefreshTokenService {
	return s.refreshTokenSvc
}

// JWTExpiry returns the configured access token expiry duration.
func (s *AuthService) JWTExpiry() time.Duration {
	return s.jwtExpiry
}

// CreateUser inserts a new user with a bcrypt-hashed password.
func (s *AuthService) CreateUser(ctx context.Context, username, password, role string) (*model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		username, string(hash), role,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetUserByID(ctx, id)
}

// GetUserByUsername finds a user by username.
func (s *AuthService) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	u := &model.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

// GetUserByID finds a user by ID.
func (s *AuthService) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	u := &model.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

// Authenticate validates credentials and returns a signed JWT access token and a refresh token.
func (s *AuthService) Authenticate(ctx context.Context, username, password string) (accessToken, refreshToken string, user *model.User, err error) {
	u, err := s.GetUserByUsername(ctx, username)
	if err != nil {
		return "", "", nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", "", nil, ErrInvalidCredentials
	}

	accessToken, err = s.generateToken(u)
	if err != nil {
		return "", "", nil, fmt.Errorf("generate token: %w", err)
	}

	refreshToken, err = s.refreshTokenSvc.GenerateToken(ctx, u.ID)
	if err != nil {
		return "", "", nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return accessToken, refreshToken, u, nil
}

// ChangePassword changes a user's password after verifying the old one.
// It also revokes all existing refresh tokens for the user.
func (s *AuthService) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	u, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE id = ?`,
		string(hash), userID,
	)
	if err != nil {
		return err
	}

	// Revoke all refresh tokens for security (best-effort, ignore failure)
	_ = s.refreshTokenSvc.RevokeAllTokens(ctx, userID)

	return nil
}

// UserCount returns the number of users in the database.
func (s *AuthService) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// ListUsers returns a paginated list of users.
func (s *AuthService) ListUsers(ctx context.Context, page, pageSize int64) ([]*model.User, int64, error) {
	p := ParsePagination(int(page), int(pageSize))

	var total int64
	countSQL := PaginatedCountSQL("users", "")
	if err := s.db.QueryRowContext(ctx, countSQL).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	querySQL := PaginatedQuerySQL(
		"SELECT id, username, password_hash, role, created_at, updated_at",
		"users", "", "id", p,
	)
	queryArgs := AppendLimitArgs(nil, p)
	rows, err := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate users: %w", err)
	}
	return users, total, nil
}

// UpdateUserRole updates a user's role. Returns an error if the user does not exist.
func (s *AuthService) UpdateUserRole(ctx context.Context, userID int64, role string) (*model.User, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE users SET role = ?, updated_at = datetime('now') WHERE id = ?`,
		role, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("update user role: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, ErrUserNotFound
	}
	return s.GetUserByID(ctx, userID)
}

// DeleteUser deletes a user by ID.
func (s *AuthService) DeleteUser(ctx context.Context, userID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// AdminCount returns the number of admin users.
func (s *AuthService) AdminCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count, err
}

// ResetPassword resets a user's password (admin operation, no old password check).
// It also revokes all existing refresh tokens for the user.
func (s *AuthService) ResetPassword(ctx context.Context, userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE id = ?`,
		string(hash), userID,
	)
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrUserNotFound
	}

	// Revoke all refresh tokens for security (best-effort, ignore failure)
	_ = s.refreshTokenSvc.RevokeAllTokens(ctx, userID)

	return nil
}

// Claims represents JWT claims.
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// generateToken creates a signed JWT token.
func (s *AuthService) generateToken(u *model.User) (string, error) {
	claims := &Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// GenerateAccessToken creates a signed JWT access token for the given user.
func (s *AuthService) GenerateAccessToken(u *model.User) (string, error) {
	return s.generateToken(u)
}

// ParseToken validates a JWT token and returns the claims.
func (s *AuthService) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
