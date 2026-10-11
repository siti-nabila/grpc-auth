package domain

import (
	"context"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
)

// RefreshReader accepts only the stored SHA-256 representation, never a raw token.
// The feature supplies either its primary candidate handle or its locked transaction.
type RefreshReader interface {
	GetByDigest(context.Context, []byte) (InitialRefresh, error)
}

type RefreshRotationWriter interface {
	InitialRefreshWriter
	// Consume returns false when the predecessor is no longer eligible. The
	// feature must roll back its already inserted successor in that case.
	Consume(context.Context, RefreshConsumption) (bool, error)
}

type RefreshConsumption struct {
	ID, SuccessorID string
	At              time.Time
}

func (r RefreshConsumption) Validate() error {
	if r.ID == r.SuccessorID || !canonicalUUID.MatchString(r.ID) || !canonicalUUID.MatchString(r.SuccessorID) || r.At.IsZero() {
		return common.ErrBadRequest
	}
	return nil
}
