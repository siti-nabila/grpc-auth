package common

import (
	"errors"
	"slices"
)

// LockDiscoveredAuthority locks a stable scope on the caller's transaction.
// A nonempty restart requires the feature to rollback before beginning again.
// The callback must read only through repositories bound to that transaction.
func LockDiscoveredAuthority(owner *TransactionOwner, held []string, discover func() ([]string, error), busy error) ([]string, error) {
	held = CanonicalLockKeys(held...)
	for check := 0; check < 3; check++ {
		discovered, err := discover()
		if err != nil {
			return nil, err
		}
		discovered = CanonicalLockKeys(discovered...)
		missing := []string{}
		for _, key := range discovered {
			if !slices.Contains(held, key) {
				missing = append(missing, key)
			}
		}
		if RequiresLockRestart(held, missing) {
			return CanonicalLockKeys(append(held, missing...)...), nil
		}
		if err = AcquireLocks(owner, busy, missing...); err != nil {
			if errors.Is(err, busy) {
				return nil, err
			}
			return nil, DependencyError(err)
		}
		held = CanonicalLockKeys(append(held, missing...)...)
		next, err := discover()
		if err != nil {
			return nil, err
		}
		next = CanonicalLockKeys(next...)
		if slices.Equal(discovered, next) {
			return nil, nil
		}
		if check == 2 {
			return CanonicalLockKeys(append(held, next...)...), nil
		}
	}
	return nil, busy
}
