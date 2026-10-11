package writer

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"time"
)

type credentialInvalidationWriter struct{ tx domain.Transaction }

func NewCredentialInvalidationWriter(tx domain.Transaction) domain.CredentialInvalidationWriter {
	return &credentialInvalidationWriter{tx: tx}
}

type credentialGeneration struct {
	Version     int64     `sql:"column:version"`
	AuthVersion int64     `sql:"column:auth_version"`
	UpdatedAt   time.Time `sql:"column:updated_at"`
}

func (credentialGeneration) TableName() string { return "auth" }

type passwordChange struct {
	Password           string    `sql:"column:password" json:"-"`
	PasswordChangedAt  time.Time `sql:"column:password_changed_at"`
	MustChangePassword bool      `sql:"column:must_change_password"`
	Version            int64     `sql:"column:version"`
	AuthVersion        int64     `sql:"column:auth_version"`
	UpdatedAt          time.Time `sql:"column:updated_at"`
}

func (passwordChange) TableName() string { return "auth" }
func (w *credentialInvalidationWriter) Invalidate(ctx context.Context, req domain.CredentialInvalidation) (bool, error) {
	if err := req.Validate(); err != nil {
		return false, err
	}
	patch := credentialGeneration{req.ExpectedVersion + 1, req.ExpectedAuthVersion + 1, req.At.UTC()}
	return w.update(ctx, req, &patch)
}
func (w *credentialInvalidationWriter) ChangePassword(ctx context.Context, req domain.PasswordChange) (bool, error) {
	if err := req.Validate(); err != nil {
		return false, err
	}
	patch := passwordChange{req.PasswordHash, req.At.UTC(), false, req.ExpectedVersion + 1, req.ExpectedAuthVersion + 1, req.At.UTC()}
	return w.update(ctx, req.CredentialInvalidation, &patch)
}
func (w *credentialInvalidationWriter) update(ctx context.Context, req domain.CredentialInvalidation, patch any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if w.tx == nil {
		return false, common.ErrInternalServerError
	}
	result, err := w.tx.UseModel(patch).WithContext(ctx).Where("id = ? AND version = ? AND auth_version = ?", req.UserID, req.ExpectedVersion, req.ExpectedAuthVersion).Updates()
	if err != nil {
		return false, helpers.HandleDBError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count < 0 || count > 1 {
		return false, common.ErrInternalServerError
	}
	return count == 1, nil
}
