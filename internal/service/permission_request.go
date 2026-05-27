package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/whg517/sqlflow/internal/model"
)

var (
	ErrPermReqNotFound    = errors.New("权限申请不存在")
	ErrPermReqAlreadyDone = errors.New("该申请已处理")
	ErrInvalidAction      = errors.New("无效的操作类型")
	ErrInvalidDuration    = errors.New("有效期必须在 1 分钟到 72 小时之间")
)

// ValidActions lists allowed actions for permission requests.
var ValidActions = []string{"select", "update", "delete", "ddl", "export"}

// PermissionRequestService manages temporary access permission requests for sensitive tables.
type PermissionRequestService struct {
	db       *sql.DB
	permSvc  *PermissionService
	auditSvc *AuditService
}

// NewPermissionRequestService creates a new PermissionRequestService.
func NewPermissionRequestService(db *sql.DB, permSvc *PermissionService, auditSvc *AuditService) *PermissionRequestService {
	return &PermissionRequestService{db: db, permSvc: permSvc, auditSvc: auditSvc}
}

// CreateRequest creates a new permission request.
func (s *PermissionRequestService) CreateRequest(ctx context.Context, applicantID, datasourceID int64, database, tableName, actions, reason string, duration time.Duration) (*model.PermissionRequest, error) {
	if err := validatePermActions(actions); err != nil {
		return nil, err
	}
	if duration < 1*time.Minute || duration > 72*time.Hour {
		return nil, ErrInvalidDuration
	}

	expiresAt := time.Now().UTC().Add(duration)

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO permission_requests (applicant_id, datasource_id, database, table_name, actions, reason, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		applicantID, datasourceID, database, tableName, actions, reason, expiresAt.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return nil, fmt.Errorf("insert permission request: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetRequestByID(ctx, id)
}

// GetRequestByID retrieves a permission request with user and datasource names.
func (s *PermissionRequestService) GetRequestByID(ctx context.Context, id int64) (*model.PermissionRequest, error) {
	r := &model.PermissionRequest{}
	var approvedAt, revokedAt sql.NullTime
	var approverName, datasourceName, applicantName, approveComment, revokeReason sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT r.id, r.applicant_id, COALESCE(u1.username, ''),
		        COALESCE(r.approver_id, 0), COALESCE(u2.username, ''),
		        r.datasource_id, COALESCE(ds.name, ''),
		        r.database, COALESCE(r.table_name, ''), r.actions, COALESCE(r.reason, ''),
		        r.status, COALESCE(r.approve_comment, ''),
		        r.approved_at, r.expires_at, r.revoked_at,
		        COALESCE(r.revoked_by, 0), COALESCE(r.revoke_reason, ''),
		        r.created_at, r.updated_at
		 FROM permission_requests r
		 LEFT JOIN users u1 ON u1.id = r.applicant_id
		 LEFT JOIN users u2 ON u2.id = r.approver_id
		 LEFT JOIN datasources ds ON ds.id = r.datasource_id
		 WHERE r.id = ?`,
		id,
	).Scan(&r.ID, &r.ApplicantID, &applicantName,
		&r.ApproverID, &approverName,
		&r.DatasourceID, &datasourceName,
		&r.Database, &r.TableName, &r.Actions, &r.Reason,
		&r.Status, &approveComment,
		&approvedAt, &r.ExpiresAt, &revokedAt,
		&r.RevokedBy, &revokeReason,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPermReqNotFound
		}
		return nil, fmt.Errorf("query permission request: %w", err)
	}

	r.ApplicantName = applicantName.String
	r.ApproverName = approverName.String
	r.DatasourceName = datasourceName.String
	r.ApproveComment = approveComment.String
	r.RevokeReason = revokeReason.String
	if approvedAt.Valid {
		r.ApprovedAt = &approvedAt.Time
	}
	if revokedAt.Valid {
		r.RevokedAt = &revokedAt.Time
	}
	return r, nil
}

// ApproveRequest approves a permission request and grants temporary casbin policies.
func (s *PermissionRequestService) ApproveRequest(ctx context.Context, requestID, approverID int64, comment string) (*model.PermissionRequest, error) {
	r, err := s.GetRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if r.Status != model.PermReqStatusPending {
		return nil, ErrPermReqAlreadyDone
	}

	if time.Now().After(r.ExpiresAt) {
		_ = s.markExpired(ctx, requestID)
		return nil, errors.New("该申请已过期")
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	_, err = s.db.ExecContext(ctx,
		`UPDATE permission_requests SET status = 'APPROVED', approver_id = ?, approve_comment = ?, approved_at = ?, updated_at = ?
		 WHERE id = ?`,
		approverID, comment, now, now, requestID,
	)
	if err != nil {
		return nil, fmt.Errorf("approve permission request: %w", err)
	}

	// Grant temporary casbin policies
	dsStr := fmt.Sprintf("%d", r.DatasourceID)
	userSub := fmt.Sprintf("user:%d", r.ApplicantID)
	actions := strings.Split(r.Actions, ",")
	obj := r.TableName
	if obj == "" {
		obj = r.Database
	}

	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		if err := s.permSvc.AddTemporaryPolicy(ctx, userSub, dsStr, obj, action, r.ExpiresAt); err != nil {
			fmt.Printf("warn: failed to add temporary policy for %s on %s/%s: %v\n", userSub, dsStr, obj, err)
		}
	}

	s.logAudit(ctx, approverID, r, "approve")
	return s.GetRequestByID(ctx, requestID)
}

// RejectRequest rejects a permission request.
func (s *PermissionRequestService) RejectRequest(ctx context.Context, requestID, approverID int64, comment string) (*model.PermissionRequest, error) {
	r, err := s.GetRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if r.Status != model.PermReqStatusPending {
		return nil, ErrPermReqAlreadyDone
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = s.db.ExecContext(ctx,
		`UPDATE permission_requests SET status = 'REJECTED', approver_id = ?, approve_comment = ?, updated_at = ? WHERE id = ?`,
		approverID, comment, now, requestID,
	)
	if err != nil {
		return nil, fmt.Errorf("reject permission request: %w", err)
	}

	s.logAudit(ctx, approverID, r, "reject")
	return s.GetRequestByID(ctx, requestID)
}

// RevokeRequest revokes an approved permission request and removes casbin policies.
func (s *PermissionRequestService) RevokeRequest(ctx context.Context, requestID, revokerID int64, reason string) (*model.PermissionRequest, error) {
	r, err := s.GetRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if r.Status != model.PermReqStatusApproved {
		return nil, ErrPermReqAlreadyDone
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = s.db.ExecContext(ctx,
		`UPDATE permission_requests SET status = 'REVOKED', revoked_at = ?, revoked_by = ?, revoke_reason = ?, updated_at = ?
		 WHERE id = ?`,
		now, revokerID, reason, now, requestID,
	)
	if err != nil {
		return nil, fmt.Errorf("revoke permission request: %w", err)
	}

	// Remove temporary policies
	dsStr := fmt.Sprintf("%d", r.DatasourceID)
	userSub := fmt.Sprintf("user:%d", r.ApplicantID)
	actions := strings.Split(r.Actions, ",")
	obj := r.TableName
	if obj == "" {
		obj = r.Database
	}
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action != "" {
			_ = s.permSvc.RemoveTemporaryPolicy(ctx, userSub, dsStr, obj, action)
		}
	}

	s.logAudit(ctx, revokerID, r, "revoke")
	return s.GetRequestByID(ctx, requestID)
}

// ListRequests returns paginated permission requests with optional filters.
func (s *PermissionRequestService) ListRequests(ctx context.Context, page, pageSize int, status, applicantIDStr string) ([]*model.PermissionRequest, int64, error) {
	p := ParsePagination(page, pageSize)

	var whereParts []string
	var args []interface{}

	if status != "" {
		whereParts = append(whereParts, "r.status = ?")
		args = append(args, status)
	}
	if applicantIDStr != "" {
		whereParts = append(whereParts, "r.applicant_id = ?")
		args = append(args, applicantIDStr)
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = "WHERE " + strings.Join(whereParts, " AND ")
	}

	var total int64
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM permission_requests r %s`, whereClause)
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count permission requests: %w", err)
	}

	querySQL := fmt.Sprintf(
		`SELECT r.id, r.applicant_id, COALESCE(u1.username, ''),
		        COALESCE(r.approver_id, 0), COALESCE(u2.username, ''),
		        r.datasource_id, COALESCE(ds.name, ''),
		        r.database, COALESCE(r.table_name, ''), r.actions, COALESCE(r.reason, ''),
		        r.status, COALESCE(r.approve_comment, ''),
		        r.approved_at, r.expires_at, r.revoked_at,
		        COALESCE(r.revoked_by, 0), COALESCE(r.revoke_reason, ''),
		        r.created_at, r.updated_at
		 FROM permission_requests r
		 LEFT JOIN users u1 ON u1.id = r.applicant_id
		 LEFT JOIN users u2 ON u2.id = r.approver_id
		 LEFT JOIN datasources ds ON ds.id = r.datasource_id
		 %s ORDER BY r.created_at DESC`, whereClause)

	querySQL += fmt.Sprintf(" LIMIT %d OFFSET %d", p.PageSize, (p.Page-1)*p.PageSize)
	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query permission requests: %w", err)
	}
	defer rows.Close()

	requests, err := scanPermRequests(rows)
	if err != nil {
		return nil, 0, err
	}
	return requests, total, nil
}

// ExpireOverdue marks expired approved requests as EXPIRED and removes their policies.
func (s *PermissionRequestService) ExpireOverdue(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, applicant_id, datasource_id, database, table_name, actions FROM permission_requests
		 WHERE status = 'APPROVED' AND expires_at <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("query expired requests: %w", err)
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var id, applicantID, datasourceID int64
		var database, tableName, actions string
		if err := rows.Scan(&id, &applicantID, &datasourceID, &database, &tableName, &actions); err != nil {
			continue
		}

		_, _ = s.db.ExecContext(ctx,
			`UPDATE permission_requests SET status = 'EXPIRED', updated_at = ? WHERE id = ?`, now, id)

		dsStr := fmt.Sprintf("%d", datasourceID)
		userSub := fmt.Sprintf("user:%d", applicantID)
		obj := tableName
		if obj == "" {
			obj = database
		}
		for _, action := range strings.Split(actions, ",") {
			action = strings.TrimSpace(action)
			if action != "" {
				_ = s.permSvc.RemoveTemporaryPolicy(ctx, userSub, dsStr, obj, action)
			}
		}
		count++
	}
	return count, rows.Err()
}

// MyActiveRequests returns the current user's active (approved, not expired) permission requests.
func (s *PermissionRequestService) MyActiveRequests(ctx context.Context, userID int64) ([]*model.PermissionRequest, int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.applicant_id, COALESCE(u1.username, ''),
		        COALESCE(r.approver_id, 0), COALESCE(u2.username, ''),
		        r.datasource_id, COALESCE(ds.name, ''),
		        r.database, COALESCE(r.table_name, ''), r.actions, COALESCE(r.reason, ''),
		        r.status, COALESCE(r.approve_comment, ''),
		        r.approved_at, r.expires_at, r.revoked_at,
		        COALESCE(r.revoked_by, 0), COALESCE(r.revoke_reason, ''),
		        r.created_at, r.updated_at
		 FROM permission_requests r
		 LEFT JOIN users u1 ON u1.id = r.applicant_id
		 LEFT JOIN users u2 ON u2.id = r.approver_id
		 LEFT JOIN datasources ds ON ds.id = r.datasource_id
		 WHERE r.applicant_id = ? AND r.status = 'APPROVED' AND r.expires_at > datetime('now')
		 ORDER BY r.expires_at ASC`,
		userID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query active requests: %w", err)
	}
	defer rows.Close()

	requests, err := scanPermRequests(rows)
	if err != nil {
		return nil, 0, err
	}
	return requests, int64(len(requests)), nil
}

func (s *PermissionRequestService) markExpired(ctx context.Context, id int64) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.ExecContext(ctx,
		`UPDATE permission_requests SET status = 'EXPIRED', updated_at = ? WHERE id = ?`, now, id)
	return err
}

func validatePermActions(actions string) error {
	for _, a := range strings.Split(actions, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		valid := false
		for _, v := range ValidActions {
			if a == v {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("%w: %s", ErrInvalidAction, a)
		}
	}
	return nil
}

func (s *PermissionRequestService) logAudit(ctx context.Context, actorID int64, r *model.PermissionRequest, action string) {
	if s.auditSvc == nil {
		return
	}
	s.auditSvc.Write(ctx, AuditRecord{
		UserID:     actorID,
		Action:     "perm_request_" + action,
		Database:   r.Database,
		SQLContent: fmt.Sprintf("permission_request:%d:%s:%s", r.ID, r.Database, r.TableName),
		SQLSummary: fmt.Sprintf("%s permission request #%d for %s/%s", action, r.ID, r.Database, r.TableName),
	})
}

func scanPermRequests(rows *sql.Rows) ([]*model.PermissionRequest, error) {
	var requests []*model.PermissionRequest
	for rows.Next() {
		r := &model.PermissionRequest{}
		var approvedAt, revokedAt sql.NullTime
		var approverName, datasourceName, applicantName, approveComment, revokeReason sql.NullString

		err := rows.Scan(&r.ID, &r.ApplicantID, &applicantName,
			&r.ApproverID, &approverName,
			&r.DatasourceID, &datasourceName,
			&r.Database, &r.TableName, &r.Actions, &r.Reason,
			&r.Status, &approveComment,
			&approvedAt, &r.ExpiresAt, &revokedAt,
			&r.RevokedBy, &revokeReason,
			&r.CreatedAt, &r.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan permission request: %w", err)
		}

		r.ApplicantName = applicantName.String
		r.ApproverName = approverName.String
		r.DatasourceName = datasourceName.String
		r.ApproveComment = approveComment.String
		r.RevokeReason = revokeReason.String
		if approvedAt.Valid {
			r.ApprovedAt = &approvedAt.Time
		}
		if revokedAt.Valid {
			r.RevokedAt = &revokedAt.Time
		}
		requests = append(requests, r)
	}
	return requests, rows.Err()
}
