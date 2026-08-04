package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entRole "github.com/whg517/sqlflow/internal/db/ent/role"
	entUser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"

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
	ErrInvalidRole        = errors.New("角色不存在或已停用")
)

var ValidRoles = map[string]bool{"admin": true, "dba": true, "developer": true}

// Service handles authentication logic.
type Service struct {
	database        *db.DB
	client          *ent.Client
	jwtSecret       []byte
	jwtExpiry       time.Duration
	refreshTokenSvc *RefreshTokenService
}

// NewService creates a new Service.
func NewService(database *db.DB, jwtSecret string, jwtExpiry time.Duration) *Service {
	return &Service{
		database:        database,
		client:          database.Client(),
		jwtSecret:       []byte(jwtSecret),
		jwtExpiry:       jwtExpiry,
		refreshTokenSvc: NewRefreshTokenService(database),
	}
}

// RefreshTokenService returns the embedded refresh token service.
func (s *Service) RefreshTokenService() *RefreshTokenService {
	return s.refreshTokenSvc
}

// JWTExpiry returns the configured access token expiry duration.
func (s *Service) JWTExpiry() time.Duration {
	return s.jwtExpiry
}

// CreateUser inserts a new user with a bcrypt-hashed password.
func (s *Service) CreateUser(ctx context.Context, username, password, role string) (*model.User, error) {
	if err := s.ValidateRole(ctx, role); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	created, err := s.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetRole(role).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	return s.GetUserByID(ctx, int64(created.ID))
}

// GetUserByUsername finds a user by username.
func (s *Service) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	u, err := s.client.User.Query().
		Where(entUser.Username(username)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	return entUserToModel(u), nil
}

// GetUserByID finds a user by ID.
func (s *Service) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	u, err := s.client.User.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	return entUserToModel(u), nil
}

// Authenticate validates credentials and returns a signed JWT access token and a refresh token.
func (s *Service) Authenticate(ctx context.Context, username, password string) (accessToken, refreshToken string, user *model.User, err error) {
	u, err := s.GetUserByUsername(ctx, username)
	if err != nil {
		return "", "", nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", "", nil, ErrInvalidCredentials
	}
	if err := s.ValidateRole(ctx, u.Role); err != nil {
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
func (s *Service) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
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

	_, err = s.client.User.UpdateOneID(int(userID)).
		SetPasswordHash(string(hash)).
		Save(ctx)
	if err != nil {
		return err
	}

	// Revoke all refresh tokens for security (best-effort, ignore failure)
	_ = s.refreshTokenSvc.RevokeAllTokens(ctx, userID)

	return nil
}

// UserCount returns the number of users in the database.
func (s *Service) UserCount(ctx context.Context) (int, error) {
	count, err := s.client.User.Query().Count(ctx)
	return count, err
}

// ListUsers returns a paginated list of users.
func (s *Service) ListUsers(ctx context.Context, page, pageSize int64) ([]*model.User, int64, error) {
	p := sqlutil.ParsePagination(int(page), int(pageSize))

	total, err := s.client.User.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	rows, err := s.client.User.Query().
		Order(ent.Asc(entUser.FieldID)).
		Limit(p.PageSize).
		Offset(p.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}

	users := make([]*model.User, 0, len(rows))
	for _, u := range rows {
		users = append(users, entUserToModel(u))
	}
	return users, int64(total), nil
}

// UpdateUserRole updates a user's role. Returns an error if the user does not exist.
func (s *Service) UpdateUserRole(ctx context.Context, userID int64, role string) (*model.User, error) {
	if err := s.ValidateRole(ctx, role); err != nil {
		return nil, err
	}
	_, err := s.client.User.UpdateOneID(int(userID)).
		SetRole(role).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("update user role: %w", err)
	}
	return s.GetUserByID(ctx, userID)
}

// ValidateRole verifies that a persisted role exists and is active.
//
// A missing role and a disabled one are the same answer to the caller: the role
// cannot be assigned. Only a database failure is worth distinguishing.
func (s *Service) ValidateRole(ctx context.Context, role string) error {
	status, err := s.client.Role.Query().
		Where(entRole.NameEQ(role)).
		Select(entRole.FieldStatus).
		String(ctx)
	if ent.IsNotFound(err) {
		return ErrInvalidRole
	}
	if err != nil {
		return fmt.Errorf("validate role: %w", err)
	}
	if status != "active" {
		return ErrInvalidRole
	}
	return nil
}

// DeleteUser deletes a user by ID.
func (s *Service) DeleteUser(ctx context.Context, userID int64) error {
	err := s.client.User.DeleteOneID(int(userID)).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrUserNotFound
		}
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// AdminCount returns the number of admin users.
func (s *Service) AdminCount(ctx context.Context) (int64, error) {
	count, err := s.client.User.Query().
		Where(entUser.Role("admin")).
		Count(ctx)
	return int64(count), err
}

// ResetPassword resets a user's password (admin operation, no old password check).
// It also revokes all existing refresh tokens for the user.
func (s *Service) ResetPassword(ctx context.Context, userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = s.client.User.UpdateOneID(int(userID)).
		SetPasswordHash(string(hash)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrUserNotFound
		}
		return fmt.Errorf("reset password: %w", err)
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
func (s *Service) generateToken(u *model.User) (string, error) {
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
func (s *Service) GenerateAccessToken(u *model.User) (string, error) {
	return s.generateToken(u)
}

// ParseToken validates a JWT token and returns the claims.
func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
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

// entUserToModel converts an ent User entity to a model.User.
func entUserToModel(u *ent.User) *model.User {
	return &model.User{
		ID:           int64(u.ID),
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		OIDCSubject:  u.OidcSubject,
		OIDCProvider: u.OidcProvider,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}
