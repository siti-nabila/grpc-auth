package common

import (
	"sort"
	"strings"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// ResolvePermissions never reads JWT roles, numeric admin codes or a cache.
// Its input must come from the caller's current primary snapshot.
func ResolvePermissions(rows []domain.PermissionContribution) []domain.EffectivePermission {
	byCode := make(map[string]map[uint64]string)
	for _, row := range rows {
		if !contributes(row) {
			continue
		}
		if byCode[row.PermissionCode] == nil {
			byCode[row.PermissionCode] = make(map[uint64]string)
		}
		byCode[row.PermissionCode][row.RoleID] = row.RoleCode
	}
	result := make([]domain.EffectivePermission, 0, len(byCode))
	for code, roles := range byCode {
		permission := domain.EffectivePermission{Code: code}
		for id, roleCode := range roles {
			permission.Roles = append(permission.Roles, domain.PermissionRole{ID: id, Code: roleCode})
		}
		sort.Slice(permission.Roles, func(i, j int) bool { return permission.Roles[i].ID < permission.Roles[j].ID })
		result = append(result, permission)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result
}

func contributes(row domain.PermissionContribution) bool {
	if row.AssignmentStatus != types.AssignmentActive || row.RoleStatus != types.RoleActive || row.ApplicationStatus != types.ApplicationActive {
		return false
	}
	if row.RoleID == 0 || row.RoleCode == "" || row.RoleApplicationID == 0 || row.RoleApplicationID != row.PermissionApplicationID {
		return false
	}
	namespace, _, ok := strings.Cut(row.PermissionCode, ".")
	if !ok || types.ApplicationCode(namespace) != row.ApplicationCode || strings.Contains(row.PermissionCode, "*") {
		return false
	}
	if row.ApplicationCode != types.ApplicationIAM && (!row.AccessStatus.Valid || row.AccessStatus.String != string(types.ApplicationAccessActive)) {
		return false
	}
	return true
}

func HasPermission(permissions []domain.EffectivePermission, code string) bool {
	for _, permission := range permissions {
		if permission.Code == code && len(permission.Roles) > 0 {
			return true
		}
	}
	return false
}
