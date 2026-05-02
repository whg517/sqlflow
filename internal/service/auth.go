package service

import (
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
	db        *sql.DB
	jwtSecret []byte
	jwtExpiry time.Duration
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *sql.DB, jwtSecret string, jwtExpiry time.Duration) *AuthService {
	return &AuthService{
		db:        db,
		jwtSecret: []byte(jwtSecret),
		jwtExpiry: jwtExpiry,
	}
}

// CreateUser inserts a new user with a bcrypt-hashed password.
func (s *AuthService) CreateUser(username, password, role string) (*model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	result, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		username, string(hash), role,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetUserByID(id)
}

// GetUserByUsername finds a user by username.
func (s *AuthService) GetUserByUsername(username string) (*model.User, error) {
	u := &model.User{}
	err := s.db.QueryRow(
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
func (s *AuthService) GetUserByID(id int64) (*model.User, error) {
	u := &model.User{}
	err := s.db.QueryRow(
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

// Authenticate validates credentials and returns a signed JWT token.
func (s *AuthService) Authenticate(username, password string) (string, *model.User, error) {
	u, err := s.GetUserByUsername(username)
	if err != nil {
		return "", nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}

	token, err := s.generateToken(u)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, u, nil
}

// ChangePassword changes a user's password after verifying the old one.
func (s *AuthService) ChangePassword(userID int64, oldPassword, newPassword string) error {
	u, err := s.GetUserByID(userID)
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

	_, err = s.db.Exec(
		`UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE id = ?`,
		string(hash), userID,
	)
	return err
}

// UserCount returns the number of users in the database.
func (s *AuthService) UserCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// ListUsers returns a paginated list of users.
func (s *AuthService) ListUsers(page, pageSize int64) ([]*model.User, int64, error) {
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.Query(
		`SELECT id, username, password_hash, role, created_at, updated_at FROM users ORDER BY id LIMIT ? OFFSET ?`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, total, nil
}

// UpdateUserRole updates a user's role. Returns an error if the user does not exist.
func (s *AuthService) UpdateUserRole(userID int64, role string) (*model.User, error) {
	result, err := s.db.Exec(
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
	return s.GetUserByID(userID)
}

// DeleteUser deletes a user by ID.
func (s *AuthService) DeleteUser(userID int64) error {
	result, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, userID)
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
func (s *AuthService) AdminCount() (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count, err
}

// ResetPassword resets a user's password (admin operation, no old password check).
func (s *AuthService) ResetPassword(userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	result, err := s.db.Exec(
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
