package domain

import (
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type MyEffectivePermissionsInput struct {
	ApplicationCode string
}

func (r MyEffectivePermissionsInput) Normalized() MyEffectivePermissionsInput {
	r.ApplicationCode = strings.TrimSpace(r.ApplicationCode)
	return r
}

func (r MyEffectivePermissionsInput) Validate() error {
	r = r.Normalized()
	if r.ApplicationCode != "" && !applicationCodeValid(r.ApplicationCode) {
		return common.ErrBadRequest
	}
	return nil
}

type UserEffectivePermissionsInput struct {
	UserID          uint64
	ApplicationCode string
}

func (r UserEffectivePermissionsInput) Normalized() UserEffectivePermissionsInput {
	r.ApplicationCode = strings.TrimSpace(r.ApplicationCode)
	return r
}

func (r UserEffectivePermissionsInput) Validate() error {
	if err := ValidateManagementTarget(r.UserID); err != nil {
		return err
	}
	return (MyEffectivePermissionsInput{ApplicationCode: r.ApplicationCode}).Validate()
}

// EffectivePermissionsResult describes retained active-role contributions.
// UserStatus and Version do not turn this view into an authorization grant.
type EffectivePermissionsResult struct {
	UserID      uint64
	UserStatus  types.UserStatus
	Version     int64
	Permissions []EffectivePermission
}
