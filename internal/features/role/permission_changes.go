package role

import (
	"slices"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

func permissionChanges(roleID uint64, pairs []domain.RolePermissionPair, desired []uint64) (domain.RolePermissionChanges, error) {
	changes := domain.RolePermissionChanges{}
	wanted := make(map[uint64]bool, len(desired))
	for _, id := range desired {
		wanted[id] = true
	}
	seen := make(map[uint64]bool, len(pairs))
	for _, pair := range pairs {
		id := pair.PermissionID
		if pair.RoleID != roleID || domain.ValidateManagementTarget(id) != nil || seen[id] {
			return changes, common.ErrDependencyUnavailable
		}
		seen[id] = true
		if wanted[id] && pair.DeletedAt != nil {
			changes.Restored = append(changes.Restored, id)
		}
		if !wanted[id] && pair.DeletedAt == nil {
			changes.Removed = append(changes.Removed, id)
		}
	}
	for _, id := range desired {
		if !seen[id] {
			changes.Inserted = append(changes.Inserted, id)
		}
	}
	slices.Sort(changes.Removed)
	slices.Sort(changes.Restored)
	slices.Sort(changes.Inserted)
	return changes, nil
}

func sortedPermissionIDs(items []domain.PermissionItem) []uint64 {
	ids := permissionIDs(items)
	slices.Sort(ids)
	return ids
}
