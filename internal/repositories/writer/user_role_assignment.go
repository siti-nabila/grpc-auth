package writer

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"time"
)

type userRoleAssignmentWriter struct{ tx domain.Transaction }

type assignmentInsert struct {
	UserID    uint64    `sql:"column:user_id"`
	PairID    uint64    `sql:"column:role_id"`
	Status    string    `sql:"column:status"`
	GrantedBy uint64    `sql:"column:granted_by"`
	GrantedAt time.Time `sql:"column:granted_at"`
}

func (assignmentInsert) TableName() string { return "user_role" }

type assignmentActivation struct {
	Status       string     `sql:"column:status"`
	GrantedBy    uint64     `sql:"column:granted_by"`
	GrantedAt    time.Time  `sql:"column:granted_at"`
	RevokedBy    *uint64    `sql:"column:revoked_by"`
	RevokedAt    *time.Time `sql:"column:revoked_at"`
	RevokeReason *string    `sql:"column:revoke_reason"`
}

func (assignmentActivation) TableName() string { return "user_role" }

type assignmentRevocation struct {
	Status       string    `sql:"column:status"`
	RevokedBy    uint64    `sql:"column:revoked_by"`
	RevokedAt    time.Time `sql:"column:revoked_at"`
	RevokeReason string    `sql:"column:revoke_reason"`
}

func (assignmentRevocation) TableName() string { return "user_role" }

func NewUserRoleAssignmentWriter(tx domain.Transaction) domain.UserRoleAssignmentWriter {
	return &userRoleAssignmentWriter{tx}
}

func (w *userRoleAssignmentWriter) Activate(ctx context.Context, user uint64, row domain.UserRoleAssignmentItem, actor uint64, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if domain.ValidateManagementTarget(user) != nil || domain.ValidateManagementTarget(row.RoleID) != nil || domain.ValidateManagementTarget(actor) != nil || at.IsZero() {
		return false, common.ErrBadRequest
	}
	exec, ok := w.tx.(domain.InsertWriter)
	if !ok || exec == nil {
		return false, common.ErrDependencyUnavailable
	}
	if row.ID == 0 {
		result, err := exec.CreateWith(&assignmentInsert{user, row.RoleID, "ACTIVE", actor, at.UTC()}).ExecResult()
		return managementAffected(result, err)
	}
	if domain.ValidateManagementTarget(row.ID) != nil || row.Status != "REVOKED" {
		return false, common.ErrBadRequest
	}
	patch := assignmentActivation{Status: "ACTIVE", GrantedBy: actor, GrantedAt: at.UTC()}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).Where("id = ? AND user_id = ? AND role_id = ? AND status = ?", row.ID, user, row.RoleID, "REVOKED").Updates()
	return managementAffected(result, err)
}

func (w *userRoleAssignmentWriter) Revoke(ctx context.Context, user uint64, row domain.UserRoleAssignmentItem, actor uint64, reason string, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if domain.ValidateManagementTarget(user) != nil || domain.ValidateManagementTarget(row.RoleID) != nil || domain.ValidateAssignmentReason(reason) != nil || domain.ValidateManagementTarget(row.ID) != nil || row.Status != "ACTIVE" || domain.ValidateManagementTarget(actor) != nil || at.IsZero() {
		return false, common.ErrBadRequest
	}
	if w.tx == nil {
		return false, common.ErrDependencyUnavailable
	}
	patch := assignmentRevocation{"REVOKED", actor, at.UTC(), reason}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).Where("id = ? AND user_id = ? AND role_id = ? AND status = ?", row.ID, user, row.RoleID, "ACTIVE").Updates()
	return managementAffected(result, err)
}

func (w *userRoleAssignmentWriter) AdvanceVersion(ctx context.Context, user uint64, expected int64, at time.Time) (bool, error) {
	return (&managementProfileWriter{w.tx}).AdvanceProfileVersion(ctx, user, expected, at)
}
