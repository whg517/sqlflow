package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"strconv"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/db/ent"
	entdatasource "github.com/whg517/sqlflow/internal/db/ent/datasource"
	entPermReq "github.com/whg517/sqlflow/internal/db/ent/permissionrequest"
	entuser "github.com/whg517/sqlflow/internal/db/ent/user"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/sqlutil"
)

// Raw SQL comments explain why certain queries remain as database/sql
// instead of being migrated to ent (typically due to LEFT JOINs across
// tables that have no ent edge relationship defined).

var (
	ErrPermReqNotFound    = errors.New("权限申请不存在")
	ErrPermReqAlreadyDone = errors.New("该申请已处理")
	ErrInvalidAction      = errors.New("无效的操作类型")
	ErrInvalidDuration    = errors.New("有效期必须在 1 分钟到 72 小时之间")
)

// ValidActions lists allowed actions for permission requests.
var ValidActions = []string{"select", "update", "delete", "ddl", "export"}

// RequestService manages temporary access permission requests for sensitive tables.
type RequestService struct {
	database *db.DB
	client   *ent.Client
	permSvc  *Service
	auditSvc auditlog.Writer
}

// NewRequestService creates a new RequestService.
func NewRequestService(database *db.DB, permSvc *Service, auditSvc auditlog.Writer) *RequestService {
	return &RequestService{
		database: database,
		client:   database.Client(),
		permSvc:  permSvc,
		auditSvc: auditlog.OrDiscard(auditSvc),
	}
}

// CreateRequest creates a new permission request.
func (s *RequestService) CreateRequest(ctx context.Context, applicantID, datasourceID int64, database, tableName, actions, reason string, duration time.Duration) (*model.PermissionRequest, error) {
	if err := validatePermActions(actions); err != nil {
		return nil, err
	}
	if duration < 1*time.Minute || duration > 72*time.Hour {
		return nil, ErrInvalidDuration
	}

	expiresAt := time.Now().UTC().Add(duration)

	saved, err := s.client.PermissionRequest.Create().
		SetApplicantID(applicantID).
		SetDatasourceID(datasourceID).
		SetDatabase(database).
		SetTableName(tableName).
		SetActions(actions).
		SetReason(reason).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("insert permission request: %w", err)
	}

	return s.GetRequestByID(ctx, int64(saved.ID))
}

// permRequestRow is a permission request with the three names it is displayed
// with.
//
// Applicant, approver and datasource all live on other tables with no ent edge,
// so they arrive through joins rather than on the entity.
type permRequestRow struct {
	ID             int64      `json:"id"`
	ApplicantID    int64      `json:"applicant_id"`
	ApplicantName  string     `json:"applicant_name"`
	ApproverID     int64      `json:"approver_id"`
	ApproverName   string     `json:"approver_name"`
	DatasourceID   int64      `json:"datasource_id"`
	DatasourceName string     `json:"datasource_name"`
	Database       string     `json:"database"`
	TableName      string     `json:"table_name"`
	Actions        string     `json:"actions"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	ApproveComment string     `json:"approve_comment"`
	ApprovedAt     *time.Time `json:"approved_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
	RevokedBy      int64      `json:"revoked_by"`
	RevokeReason   string     `json:"revoke_reason"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (r permRequestRow) toModel() *model.PermissionRequest {
	return &model.PermissionRequest{
		ID: r.ID, ApplicantID: r.ApplicantID, ApplicantName: r.ApplicantName,
		ApproverID: r.ApproverID, ApproverName: r.ApproverName,
		DatasourceID: r.DatasourceID, DatasourceName: r.DatasourceName,
		Database: r.Database, TableName: r.TableName,
		Actions: r.Actions, Reason: r.Reason,
		Status:         model.PermissionRequestStatus(r.Status),
		ApproveComment: r.ApproveComment, ApprovedAt: r.ApprovedAt,
		ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt,
		RevokedBy: r.RevokedBy, RevokeReason: r.RevokeReason,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// selectPermRequestsWithNames joins the three tables the display names come
// from.
//
// One helper, because the same select was written out at three call sites and
// each carried its own copy of twenty columns.
func selectPermRequestsWithNames(sel *entsql.Selector) {
	u1 := entsql.Table(entuser.Table).As("u1")
	u2 := entsql.Table(entuser.Table).As("u2")
	ds := entsql.Table(entdatasource.Table).As("ds")
	sel.LeftJoin(u1).On(sel.C(entPermReq.FieldApplicantID), u1.C(entuser.FieldID))
	sel.LeftJoin(u2).On(sel.C(entPermReq.FieldApproverID), u2.C(entuser.FieldID))
	sel.LeftJoin(ds).On(sel.C(entPermReq.FieldDatasourceID), ds.C(entdatasource.FieldID))

	cols := make([]string, 0, len(entPermReq.Columns))
	for _, c := range entPermReq.Columns {
		cols = append(cols, sel.C(c))
	}
	sel.Select(cols...).AppendSelect(
		entsql.As("COALESCE("+u1.C(entuser.FieldUsername)+", '')", "applicant_name"),
		entsql.As("COALESCE("+u2.C(entuser.FieldUsername)+", '')", "approver_name"),
		entsql.As("COALESCE("+ds.C(entdatasource.FieldName)+", '')", "datasource_name"),
	)
}

// fetchPermRequests runs a joined query and converts the rows.
func (s *RequestService) fetchPermRequests(ctx context.Context, q *ent.PermissionRequestQuery, order func(*entsql.Selector)) ([]*model.PermissionRequest, error) {
	var rows []permRequestRow
	err := q.Modify(func(sel *entsql.Selector) {
		selectPermRequestsWithNames(sel)
		order(sel)
	}).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	requests := make([]*model.PermissionRequest, 0, len(rows))
	for _, r := range rows {
		requests = append(requests, r.toModel())
	}
	return requests, nil
}

// GetRequestByID retrieves a permission request with user and datasource names.
func (s *RequestService) GetRequestByID(ctx context.Context, id int64) (*model.PermissionRequest, error) {
	requests, err := s.fetchPermRequests(ctx,
		s.client.PermissionRequest.Query().Where(entPermReq.IDEQ(int(id))),
		func(*entsql.Selector) {})
	if err != nil {
		return nil, fmt.Errorf("get permission request: %w", err)
	}
	if len(requests) == 0 {
		return nil, ErrPermReqNotFound
	}
	return requests[0], nil
}

// ApproveRequest approves a permission request and grants temporary casbin policies.
func (s *RequestService) ApproveRequest(ctx context.Context, requestID, approverID int64, comment string) (*model.PermissionRequest, error) {
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

	now := time.Now().UTC()
	_, err = s.client.PermissionRequest.UpdateOneID(int(requestID)).
		SetStatus("APPROVED").
		SetApproverID(approverID).
		SetApproveComment(comment).
		SetApprovedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("approve permission request: %w", err)
	}

	// Grant temporary casbin policies
	dsStr := authz.DatasourceDomain(r.DatasourceID)
	userSub := authz.UserSubject(r.ApplicantID)
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
func (s *RequestService) RejectRequest(ctx context.Context, requestID, approverID int64, comment string) (*model.PermissionRequest, error) {
	r, err := s.GetRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if r.Status != model.PermReqStatusPending {
		return nil, ErrPermReqAlreadyDone
	}

	now := time.Now().UTC()
	_, err = s.client.PermissionRequest.UpdateOneID(int(requestID)).
		SetStatus("REJECTED").
		SetApproverID(approverID).
		SetApproveComment(comment).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("reject permission request: %w", err)
	}

	s.logAudit(ctx, approverID, r, "reject")
	return s.GetRequestByID(ctx, requestID)
}

// RevokeRequest revokes an approved permission request and removes casbin policies.
func (s *RequestService) RevokeRequest(ctx context.Context, requestID, revokerID int64, reason string) (*model.PermissionRequest, error) {
	r, err := s.GetRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if r.Status != model.PermReqStatusApproved {
		return nil, ErrPermReqAlreadyDone
	}

	now := time.Now().UTC()
	_, err = s.client.PermissionRequest.UpdateOneID(int(requestID)).
		SetStatus("REVOKED").
		SetRevokedAt(now).
		SetRevokedBy(revokerID).
		SetRevokeReason(reason).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("revoke permission request: %w", err)
	}

	// Remove temporary policies
	dsStr := authz.DatasourceDomain(r.DatasourceID)
	userSub := authz.UserSubject(r.ApplicantID)
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
func (s *RequestService) ListRequests(ctx context.Context, page, pageSize int, status, applicantIDStr string) ([]*model.PermissionRequest, int64, error) {
	p := sqlutil.ParsePagination(page, pageSize)

	q := s.client.PermissionRequest.Query()
	if status != "" {
		q = q.Where(entPermReq.StatusEQ(status))
	}
	// applicant_id is a bigint. Passing the raw query-string value made
	// PostgreSQL reject the comparison, and only worked under SQLite's dynamic
	// typing. An unparseable value filters nothing rather than erroring, which
	// matches how the other filters treat an empty parameter.
	if id, err := strconv.ParseInt(applicantIDStr, 10, 64); err == nil && applicantIDStr != "" {
		q = q.Where(entPermReq.ApplicantIDEQ(id))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count permission requests: %w", err)
	}

	requests, err := s.fetchPermRequests(ctx, q, func(sel *entsql.Selector) {
		sel.OrderBy(entsql.Desc(sel.C(entPermReq.FieldCreatedAt))).
			Limit(p.PageSize).
			Offset(p.Offset)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("query permission requests: %w", err)
	}
	return requests, int64(total), nil
}

// ExpireOverdue marks expired approved requests as EXPIRED and removes their policies.
func (s *RequestService) ExpireOverdue(ctx context.Context) (int64, error) {
	now := time.Now().UTC()

	// Find APPROVED requests past expiry using ent
	expired, err := s.client.PermissionRequest.Query().
		Where(
			entPermReq.StatusEQ("APPROVED"),
			entPermReq.ExpiresAtLTE(now),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query expired requests: %w", err)
	}

	var count int64
	for _, r := range expired {
		// Update to EXPIRED
		_, _ = s.client.PermissionRequest.UpdateOneID(r.ID).
			SetStatus("EXPIRED").
			SetUpdatedAt(now).
			Save(ctx)

		// Remove temporary policies
		dsStr := authz.DatasourceDomain(int64(r.DatasourceID))
		userSub := authz.UserSubject(int64(r.ApplicantID))
		obj := r.TableName
		if obj == "" {
			obj = r.Database
		}
		for _, action := range strings.Split(r.Actions, ",") {
			action = strings.TrimSpace(action)
			if action != "" {
				_ = s.permSvc.RemoveTemporaryPolicy(ctx, userSub, dsStr, obj, action)
			}
		}
		count++
	}
	return count, nil
}

// MyActiveRequests returns the current user's active (approved, not expired) permission requests.
func (s *RequestService) MyActiveRequests(ctx context.Context, userID int64) ([]*model.PermissionRequest, int64, error) {
	q := s.client.PermissionRequest.Query().Where(
		entPermReq.ApplicantIDEQ(userID),
		entPermReq.StatusEQ("APPROVED"),
		entPermReq.ExpiresAtGT(time.Now()),
	)
	requests, err := s.fetchPermRequests(ctx, q, func(sel *entsql.Selector) {
		// Soonest to expire first: the list exists so the user can renew before
		// losing access.
		sel.OrderBy(entsql.Asc(sel.C(entPermReq.FieldExpiresAt)))
	})
	if err != nil {
		return nil, 0, fmt.Errorf("query active requests: %w", err)
	}
	return requests, int64(len(requests)), nil
}

func (s *RequestService) markExpired(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	_, err := s.client.PermissionRequest.UpdateOneID(int(id)).
		SetStatus("EXPIRED").
		SetUpdatedAt(now).
		Save(ctx)
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

func (s *RequestService) logAudit(ctx context.Context, actorID int64, r *model.PermissionRequest, action string) {
	if s.auditSvc == nil {
		return
	}
	s.auditSvc.Write(ctx, auditlog.Record{
		UserID:     actorID,
		Action:     "perm_request_" + action,
		Database:   r.Database,
		SQLContent: fmt.Sprintf("permission_request:%d:%s:%s", r.ID, r.Database, r.TableName),
		SQLSummary: fmt.Sprintf("%s permission request #%d for %s/%s", action, r.ID, r.Database, r.TableName),
	})
}
