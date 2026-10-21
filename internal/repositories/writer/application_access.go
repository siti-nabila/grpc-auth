package writer

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"time"
)

type applicationAccessWriter struct{ tx domain.Transaction }

type applicationInsert struct {
	UserID    uint64    `sql:"column:user_id"`
	PairID    uint64    `sql:"column:application_id"`
	Status    string    `sql:"column:status"`
	GrantedBy uint64    `sql:"column:granted_by"`
	GrantedAt time.Time `sql:"column:granted_at"`
}

func (applicationInsert) TableName() string { return "user_application_access" }

type applicationActivation struct {
	Status       string     `sql:"column:status"`
	GrantedBy    uint64     `sql:"column:granted_by"`
	GrantedAt    time.Time  `sql:"column:granted_at"`
	RevokedBy    *uint64    `sql:"column:revoked_by"`
	RevokedAt    *time.Time `sql:"column:revoked_at"`
	RevokeReason *string    `sql:"column:revoke_reason"`
}

func (applicationActivation) TableName() string { return "user_application_access" }

type applicationRevocation struct {
	Status       string    `sql:"column:status"`
	RevokedBy    uint64    `sql:"column:revoked_by"`
	RevokedAt    time.Time `sql:"column:revoked_at"`
	RevokeReason string    `sql:"column:revoke_reason"`
}

func (applicationRevocation) TableName() string { return "user_application_access" }

func NewApplicationAccessWriter(tx domain.Transaction) domain.ApplicationAccessWriter {
	return &applicationAccessWriter{tx}
}

func (w *applicationAccessWriter) Activate(ctx context.Context, userID, applicationID uint64, before *domain.ApplicationAccessItem, actor uint64, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if domain.ValidateManagementTarget(userID) != nil || domain.ValidateManagementTarget(applicationID) != nil || domain.ValidateManagementTarget(actor) != nil || at.IsZero() {
		return false, common.ErrBadRequest
	}
	if before != nil && (before.UserID != userID || before.ApplicationID != applicationID || before.Status != "REVOKED") {
		return false, common.ErrBadRequest
	}
	exec, ok := w.tx.(domain.InsertWriter)
	if !ok || exec == nil {
		return false, common.ErrDependencyUnavailable
	}
	if before == nil {
		result, err := exec.CreateWith(&applicationInsert{userID, applicationID, "ACTIVE", actor, at.UTC()}).ExecResult()
		return managementAffected(result, err)
	}
	patch := applicationActivation{Status: "ACTIVE", GrantedBy: actor, GrantedAt: at.UTC()}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).Where("user_id = ? AND application_id = ? AND status = ?", userID, applicationID, "REVOKED").Updates()
	return managementAffected(result, err)
}

func (w *applicationAccessWriter) Revoke(ctx context.Context, userID uint64, before domain.ApplicationAccessItem, actor uint64, reason string, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if domain.ValidateManagementTarget(userID) != nil || domain.ValidateManagementTarget(before.ApplicationID) != nil || domain.ValidateManagementTarget(actor) != nil || before.UserID != userID || before.Status != "ACTIVE" || domain.ValidateAssignmentReason(reason) != nil || at.IsZero() {
		return false, common.ErrBadRequest
	}
	if w.tx == nil {
		return false, common.ErrDependencyUnavailable
	}
	patch := applicationRevocation{"REVOKED", actor, at.UTC(), reason}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).Where("user_id = ? AND application_id = ? AND status = ?", userID, before.ApplicationID, "ACTIVE").Updates()
	return managementAffected(result, err)
}
