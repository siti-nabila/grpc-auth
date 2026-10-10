package common

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
)

// DependencyError preserves cancellation and the cause while exposing only the
// catalog's safe unavailable response at the transport boundary.
func DependencyError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.Join(common.ErrDependencyUnavailable, err)
}
