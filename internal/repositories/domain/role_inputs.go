package domain

import (
	"regexp"
	"slices"
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

var applicationCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*[.][a-z][a-z0-9_]*$`)
var permissionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*){2,}$`)

type CreateRoleInput struct {
	ApplicationCode, Code, Name string
	Description                 *string
	PermissionCodes             []string
	Key                         string
}

func (r CreateRoleInput) Normalized() CreateRoleInput {
	r.ApplicationCode = strings.TrimSpace(r.ApplicationCode)
	r.Code = strings.TrimSpace(r.Code)
	r.Name = strings.TrimSpace(r.Name)
	r.Description = normalizedRoleText(r.Description)
	r.PermissionCodes = NormalizePermissionCodes(r.PermissionCodes)
	return r
}

func (r CreateRoleInput) Validate() error {
	if err := ValidateManagementKey(r.Key, true); err != nil {
		return err
	}
	return r.ValidateDefinition()
}

// ValidateDefinition checks persisted role content independently of transport
// idempotency metadata, which the feature validates before starting a command.
func (r CreateRoleInput) ValidateDefinition() error {
	r = r.Normalized()
	if !applicationCodeValid(r.ApplicationCode) || !roleCodeValid(r.Code) || !strings.HasPrefix(r.Code, r.ApplicationCode+".") || !userText(r.Name, 1, 50) || (r.Description != nil && !userText(*r.Description, 0, 100)) {
		return common.ErrBadRequest
	}
	return ValidatePermissionCodes(r.PermissionCodes, r.ApplicationCode)
}

type UpdateRoleInput struct {
	RoleID            uint64
	ExpectedVersion   int64
	Name, Description *string
	Fields            []string
	Key               string
}

func (r UpdateRoleInput) Normalized() UpdateRoleInput {
	r.Name, r.Description = normalizedRoleText(r.Name), normalizedRoleText(r.Description)
	r.Fields = slices.Clone(r.Fields)
	return r
}

func (r UpdateRoleInput) Validate() error {
	r = r.Normalized()
	if ValidateManagementTarget(r.RoleID) != nil || r.ExpectedVersion < 1 || ValidateManagementKey(r.Key, false) != nil || len(r.Fields) == 0 {
		return common.ErrBadRequest
	}
	selected := map[string]bool{}
	for _, field := range r.Fields {
		if selected[field] || (field != "name" && field != "description") {
			return common.ErrBadRequest
		}
		selected[field] = true
	}
	if selected["name"] != (r.Name != nil) || selected["description"] != (r.Description != nil) || (r.Name != nil && !userText(*r.Name, 1, 50)) || (r.Description != nil && !userText(*r.Description, 0, 100)) {
		return common.ErrBadRequest
	}
	return nil
}

type ChangeRoleStatusInput struct {
	RoleID          uint64
	ExpectedVersion int64
	Status          *types.RoleStatus
	Reason, Key     string
}

func (r ChangeRoleStatusInput) Normalized() ChangeRoleStatusInput {
	r.Reason = strings.TrimSpace(r.Reason)
	return r
}

func (r ChangeRoleStatusInput) Validate() error {
	r = r.Normalized()
	if ValidateManagementTarget(r.RoleID) != nil || r.ExpectedVersion < 1 || r.Status == nil || !RoleStatusValid(*r.Status) || !userText(r.Reason, 3, 500) || ValidateManagementKey(r.Key, true) != nil {
		return common.ErrBadRequest
	}
	return nil
}

type PermissionSet struct{ Codes []string }

type ReplaceRolePermissionsInput struct {
	RoleID          uint64
	ExpectedVersion int64
	Permissions     *PermissionSet
	Reason, Key     string
}

func (r ReplaceRolePermissionsInput) Normalized() ReplaceRolePermissionsInput {
	r.Reason = strings.TrimSpace(r.Reason)
	if r.Permissions != nil {
		r.Permissions = &PermissionSet{Codes: NormalizePermissionCodes(r.Permissions.Codes)}
	}
	return r
}

func (r ReplaceRolePermissionsInput) Validate() error {
	r = r.Normalized()
	if ValidateManagementTarget(r.RoleID) != nil || r.ExpectedVersion < 1 || r.Permissions == nil || !userText(r.Reason, 3, 500) || ValidateManagementKey(r.Key, true) != nil {
		return common.ErrBadRequest
	}
	return ValidatePermissionCodes(r.Permissions.Codes, "")
}

func NormalizePermissionCodes(codes []string) []string {
	codes = slices.Clone(codes)
	for i := range codes {
		codes[i] = strings.TrimSpace(codes[i])
	}
	slices.Sort(codes)
	return slices.Compact(codes)
}

func ValidatePermissionCodes(codes []string, application string) error {
	for _, code := range codes {
		if !userText(code, 1, 150) || !permissionCodePattern.MatchString(code) || (application != "" && !strings.HasPrefix(code, application+".")) {
			return roleerrors.ErrInvalidPermissionSet
		}
	}
	return nil
}

func applicationCodeValid(code string) bool {
	return userText(code, 1, 50) && applicationCodePattern.MatchString(code)
}

func roleCodeValid(code string) bool {
	return userText(code, 1, 100) && roleCodePattern.MatchString(code)
}

func normalizedRoleText(value *string) *string {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(*value)
	return &text
}
