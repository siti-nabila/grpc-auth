package domain

import (
	"slices"
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
)

func RoleCommandResult(detail RoleDetail, changed bool) SafeCommandResult {
	permissions := slices.Clone(detail.Permissions)
	slices.SortFunc(permissions, func(a, b PermissionItem) int {
		return strings.Compare(a.Code, b.Code)
	})
	return SafeCommandResult{RoleID: detail.Role.ID, Version: detail.Role.Version, Status: string(detail.Role.Status), Changed: changed, Role: &detail.Role, Permissions: permissions}
}

// RoleDetail reconstructs the original response, including its historical count
// and nullable metadata. Replay must not query current mutable role state.
func (r SafeCommandResult) RoleDetail() (RoleDetail, error) {
	if err := r.Validate(); err != nil || r.Role == nil {
		return RoleDetail{}, common.ErrInternalServerError
	}
	return RoleDetail{Role: *r.Role, Permissions: slices.Clone(r.Permissions)}, nil
}

func (r SafeCommandResult) validateRoleResult() error {
	if r.Role == nil {
		if len(r.Permissions) > 0 {
			return common.ErrBadRequest
		}
		return nil
	}
	role := r.Role
	if role.Validate() != nil || r.RoleID != role.ID || r.Version != role.Version || r.Status != string(role.Status) || r.UserID != 0 || r.SessionID != "" || r.Email != nil || r.Name != nil || r.Address != nil || r.Phone != nil || r.MustChangePassword != nil || len(r.Roles) > 0 || len(r.ApplicationAccess) > 0 || r.CreatedAt != nil || r.LoginRequired || len(r.ChangedRoleIDs) > 0 || r.ApplicationID != 0 {
		return common.ErrBadRequest
	}
	ids, codes := map[uint64]bool{}, map[string]bool{}
	for _, p := range r.Permissions {
		if ValidateManagementTarget(p.ID) != nil || p.ApplicationID != role.ApplicationID || p.ApplicationCode != role.ApplicationCode || ValidatePermissionCodes([]string{p.Code}, string(role.ApplicationCode)) != nil || !userText(p.Description, 0, len(p.Description)) || ids[p.ID] || codes[p.Code] {
			return common.ErrBadRequest
		}
		ids[p.ID], codes[p.Code] = true, true
	}
	return nil
}
