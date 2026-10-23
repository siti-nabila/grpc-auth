package writer

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type managementAccountWriter struct{ tx domain.Transaction }
type managementProfileWriter struct{ tx domain.Transaction }
type managementStatusWriter struct{ tx domain.Transaction }

func NewManagementAccountWriter(tx domain.Transaction) domain.RegistrationWriter {
	return &managementAccountWriter{tx}
}
func NewManagementProfileWriter(tx domain.Transaction) domain.ManagementProfileWriter {
	return &managementProfileWriter{tx}
}
func NewManagementStatusWriter(tx domain.Transaction) domain.ManagementStatusWriter {
	return &managementStatusWriter{tx}
}

func (w *managementAccountWriter) Create(r *domain.PendingRegistration) error {
	if w.tx == nil {
		return common.ErrDependencyUnavailable
	}
	if r == nil || r.ID != 0 || r.Email == "" || r.Password == "" || (r.Status != types.UserPending && r.Status != types.UserActive) || r.Version != 1 || r.AuthVersion != 1 || !r.MustChangePassword {
		return common.ErrBadRequest
	}
	return helpers.HandleDBError(w.tx.Create(r))
}

type managementProfilePatch struct {
	Name        string         `sql:"column:name"`
	Address     string         `sql:"column:address"`
	Phone       string         `sql:"column:phone"`
	CountryCode sql.NullString `sql:"column:country_code"`
}

func (managementProfilePatch) TableName() string { return "profile" }

func (w *managementProfileWriter) UpdateProfile(ctx context.Context, id uint64, r domain.UserDetail) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if w.tx == nil {
		return false, common.ErrDependencyUnavailable
	}
	patch := managementProfilePatch{r.Name, r.Address, r.Phone, domain.NullableCountryCode(&r.CountryCode)}
	res, err := w.tx.UseModel(&patch).WithContext(ctx).Where("user_id = ?", id).Updates()
	return managementAffected(res, err)
}

type managementVersionPatch struct {
	Version   int64     `sql:"column:version"`
	UpdatedAt time.Time `sql:"column:updated_at"`
}

func (managementVersionPatch) TableName() string { return "auth" }

func (w *managementProfileWriter) AdvanceProfileVersion(ctx context.Context, id uint64, expected int64, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if w.tx == nil {
		return false, common.ErrDependencyUnavailable
	}
	if domain.ValidateManagementTarget(id) != nil || expected < 1 || expected == math.MaxInt64 || at.IsZero() {
		return false, common.ErrBadRequest
	}
	patch := managementVersionPatch{expected + 1, at.UTC()}
	res, err := w.tx.UseModel(&patch).WithContext(ctx).Where("id = ? AND version = ?", id, expected).Updates()
	return managementAffected(res, err)
}

type managementStatusPatch struct {
	Status      types.UserStatus `sql:"column:status"`
	Version     int64            `sql:"column:version"`
	AuthVersion int64            `sql:"column:auth_version"`
	UpdatedAt   time.Time        `sql:"column:updated_at"`
}

func (managementStatusPatch) TableName() string { return "auth" }

func (w *managementStatusWriter) ChangeStatus(ctx context.Context, before domain.SessionUserState, status types.UserStatus, invalidate bool, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if w.tx == nil {
		return false, common.ErrDependencyUnavailable
	}
	if domain.ValidateManagementTarget(before.ID) != nil || before.Version < 1 || before.Version == math.MaxInt64 || before.AuthVersion < 1 || invalidate && before.AuthVersion == math.MaxInt64 || !domain.UserStatusTransitionAllowed(before.Status, status) || at.IsZero() {
		return false, common.ErrBadRequest
	}
	patch := managementStatusPatch{status, before.Version + 1, before.AuthVersion, at.UTC()}
	if invalidate {
		patch.AuthVersion++
	}
	res, err := w.tx.UseModel(&patch).WithContext(ctx).Where("id = ? AND version = ? AND auth_version = ? AND status = ? AND deleted_at IS NULL", before.ID, before.Version, before.AuthVersion, before.Status).Updates()
	return managementAffected(res, err)
}

func managementAffected(result sql.Result, err error) (bool, error) {
	if err != nil {
		return false, helpers.HandleDBError(err)
	}
	if result == nil {
		return false, common.ErrDependencyUnavailable
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n < 0 || n > 1 {
		return false, common.ErrDependencyUnavailable
	}
	return n == 1, nil
}
