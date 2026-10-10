package common

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func RegisterLockKeys(email string) []string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return []string{"auth:guard:policy:global", fmt.Sprintf("auth:account:%x", digest)}
}

func UserLockKey(id uint64) string { return "auth:user:" + strconv.FormatUint(id, 10) }

// AcquireLocks follows the shared category order and canonical numeric IDs.
func AcquireLocks(owner *TransactionOwner, busy error, keys ...string) error {
	for _, key := range CanonicalLockKeys(keys...) {
		acquired, err := owner.TryLock(key)
		if err != nil {
			return err
		}
		if !acquired {
			return busy
		}
	}
	return nil
}

func CanonicalLockKeys(keys ...string) []string {
	ordered := slices.Clone(keys)
	slices.SortFunc(ordered, func(a, b string) int {
		ra, rb := lockRank(a), lockRank(b)
		if ra < rb {
			return -1
		}
		if ra > rb {
			return 1
		}
		if ra >= 4 && ra <= 6 {
			ai, ae := strconv.ParseUint(a[strings.LastIndexByte(a, ':')+1:], 10, 64)
			bi, be := strconv.ParseUint(b[strings.LastIndexByte(b, ':')+1:], 10, 64)
			if ae == nil && be == nil {
				if ai < bi {
					return -1
				}
				if ai > bi {
					return 1
				}
			}
		}
		return strings.Compare(a, b)
	})
	return slices.Compact(ordered)
}

// RequiresLockRestart detects a discovered predecessor; the consumer must
// rollback and begin again before acquiring the enlarged scope set.
func RequiresLockRestart(held, discovered []string) bool {
	ordered := CanonicalLockKeys(held...)
	if len(ordered) == 0 {
		return false
	}
	last := ordered[len(ordered)-1]
	for _, key := range discovered {
		if slices.Contains(ordered, key) {
			continue
		}
		if CanonicalLockKeys(last, key)[0] == key {
			return true
		}
	}
	return false
}

func lockRank(key string) int {
	switch {
	case strings.HasPrefix(key, "auth:idem:"):
		return 0
	case key == "auth:guard:last-admin:global":
		return 1
	case key == "auth:guard:policy:global":
		return 2
	case strings.HasPrefix(key, "auth:account:"), strings.HasPrefix(key, "auth:role-code:"):
		return 3
	case strings.HasPrefix(key, "auth:user:"):
		return 4
	case strings.HasPrefix(key, "auth:role:"):
		return 5
	case strings.HasPrefix(key, "auth:application:"):
		return 6
	case strings.HasPrefix(key, "auth:session:"):
		return 7
	default:
		return 8
	}
}

func UserLockKeys(ids []uint64) []string {
	ordered := slices.Clone(ids)
	slices.Sort(ordered)
	ordered = slices.Compact(ordered)
	keys := make([]string, len(ordered))
	for i, id := range ordered {
		keys[i] = UserLockKey(id)
	}
	return keys
}
