package domain

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"math"
	"time"
)

// CredentialInvalidation is supplied only after the feature's locked recheck.
// Entity version and auth version are distinct, each incremented exactly once.
type CredentialInvalidation struct {
	UserID                               uint64
	ExpectedVersion, ExpectedAuthVersion int64
	At                                   time.Time
}

func (r CredentialInvalidation) Validate() error {
	if r.UserID == 0 || r.UserID > 2147483647 || r.ExpectedVersion < 1 || r.ExpectedVersion == math.MaxInt64 || r.ExpectedAuthVersion < 1 || r.ExpectedAuthVersion == math.MaxInt64 || r.At.IsZero() {
		return common.ErrBadRequest
	}
	return nil
}

type PasswordChange struct {
	CredentialInvalidation
	PasswordHash string `json:"-"`
}

func (r PasswordChange) Validate() error {
	if err := r.CredentialInvalidation.Validate(); err != nil {
		return err
	}
	if !boundedText(r.PasswordHash, 150, true) {
		return common.ErrBadRequest
	}
	return nil
}

type CredentialInvalidationWriter interface {
	Invalidate(context.Context, CredentialInvalidation) (bool, error)
	ChangePassword(context.Context, PasswordChange) (bool, error)
}
