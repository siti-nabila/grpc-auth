package domain

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/orm/orm"
)

// UserRoleAssignmentItem describes stored history, including inactive policy.
// Nullable legacy provenance stays absent; credentials cannot be represented.
type UserRoleAssignmentItem struct {
	ID                      uint64     `sql:"column:id"`
	RoleID                  uint64     `sql:"column:role_id"`
	Code                    string     `sql:"column:code"`
	RoleCode                int32      `sql:"column:role_code"`
	Name                    string     `sql:"column:name"`
	Status                  string     `sql:"column:status"`
	RoleStatus              string     `sql:"column:role_status"`
	ApplicationID           uint64     `sql:"column:application_id"`
	ApplicationCode         string     `sql:"column:application_code"`
	ApplicationStatus       string     `sql:"column:application_status"`
	IsSystem                bool       `sql:"column:is_system"`
	ApplicationAccessStatus *string    `sql:"column:application_access_status"`
	GrantedBy               *uint64    `sql:"column:granted_by"`
	GrantedAt               *time.Time `sql:"column:granted_at"`
	RevokedBy               *uint64    `sql:"column:revoked_by"`
	RevokedAt               *time.Time `sql:"column:revoked_at"`
	RevokeReason            *string    `sql:"column:revoke_reason"`
}

func (UserRoleAssignmentItem) TableName() string { return "user_role ur" }

type UserRoleAssignmentInput struct {
	UserID          uint64
	RoleIDs         []uint64
	ExpectedVersion int64
	Reason, Key     string
}

func (r UserRoleAssignmentInput) Validate() error {
	if ValidateManagementTarget(r.UserID) != nil || r.ExpectedVersion < 1 || len(r.RoleIDs) < 1 || len(r.RoleIDs) > 20 || ValidateAssignmentReason(r.Reason) != nil || ValidateManagementKey(r.Key, true) != nil {
		return common.ErrBadRequest
	}
	for _, id := range r.RoleIDs {
		if ValidateManagementTarget(id) != nil {
			return common.ErrBadRequest
		}
	}
	return nil
}

func ValidateAssignmentReason(reason string) error {
	if !userText(strings.TrimSpace(reason), 3, 500) {
		return common.ErrBadRequest
	}
	return nil
}

func (r UserRoleAssignmentInput) Normalized() UserRoleAssignmentInput {
	r.Reason = strings.TrimSpace(r.Reason)
	r.RoleIDs = slices.Clone(r.RoleIDs)
	slices.Sort(r.RoleIDs)
	r.RoleIDs = slices.Compact(r.RoleIDs)
	return r
}

// AssignmentRoles returns every registered requested role, including ID=0 for
// an absent pair. An unregistered role is absent from the result altogether.
type UserRoleAssignmentReader interface {
	AssignmentRoles(context.Context, uint64, []uint64) ([]UserRoleAssignmentItem, error)
	ListAssignments(context.Context, CatalogQuery) (orm.PageData[UserRoleAssignmentItem], error)
}

type UserRoleAssignmentWriter interface {
	Activate(context.Context, uint64, UserRoleAssignmentItem, uint64, time.Time) (bool, error)
	Revoke(context.Context, uint64, UserRoleAssignmentItem, uint64, string, time.Time) (bool, error)
	AdvanceVersion(context.Context, uint64, int64, time.Time) (bool, error)
}

type UserRoleAssignmentPage struct {
	UserID  uint64
	Version int64
	Page    orm.PageData[UserRoleAssignmentItem]
}

// ValidateAssignmentResult applies only to C results, preserving other command
// result shapes and rejecting corrupted or cross-target replay data.
func (r SafeCommandResult) ValidateAssignmentResult(target uint64) error {
	if r.Validate() != nil || ValidateManagementTarget(r.UserID) != nil || r.UserID != target || r.Version < 1 || r.Changed != (len(r.ChangedRoleIDs) > 0) || len(r.ChangedRoleIDs) > 20 {
		return common.ErrInternalServerError
	}
	for i, id := range r.ChangedRoleIDs {
		if ValidateManagementTarget(id) != nil || i > 0 && id <= r.ChangedRoleIDs[i-1] {
			return common.ErrInternalServerError
		}
	}
	return nil
}

func (r UserRoleAssignmentItem) CatalogValue(field string) CatalogValue {
	switch field {
	case "id":
		return catalogText("integer", strconv.FormatUint(r.ID, 10))
	case "role_id":
		return catalogText("integer", strconv.FormatUint(r.RoleID, 10))
	case "code":
		return catalogText("text", r.Code)
	case "role_code":
		return catalogText("integer", strconv.FormatInt(int64(r.RoleCode), 10))
	case "name":
		return catalogText("text", r.Name)
	case "status":
		return catalogText("text", r.Status)
	case "role_status":
		return catalogText("text", r.RoleStatus)
	case "application_id":
		return catalogText("integer", strconv.FormatUint(r.ApplicationID, 10))
	case "application_code":
		return catalogText("text", r.ApplicationCode)
	case "application_status":
		return catalogText("text", r.ApplicationStatus)
	case "is_system":
		return catalogText("bool", strconv.FormatBool(r.IsSystem))
	case "application_access_status":
		return CatalogValue{"text", r.ApplicationAccessStatus}
	case "granted_by":
		if r.GrantedBy == nil {
			return CatalogValue{Kind: "integer"}
		}
		return catalogText("integer", strconv.FormatUint(*r.GrantedBy, 10))
	case "granted_at":
		return catalogTime(r.GrantedAt)
	case "revoked_by":
		if r.RevokedBy == nil {
			return CatalogValue{Kind: "integer"}
		}
		return catalogText("integer", strconv.FormatUint(*r.RevokedBy, 10))
	case "revoked_at":
		return catalogTime(r.RevokedAt)
	case "revoke_reason":
		return CatalogValue{"text", r.RevokeReason}
	default:
		return CatalogValue{}
	}
}

func (r UserRoleAssignmentItem) Project(q CatalogQuery) UserRoleAssignmentItem {
	out := UserRoleAssignmentItem{ID: r.ID, RoleID: r.RoleID}
	if q.Projects("code") {
		out.Code = r.Code
	}
	if q.Projects("role_code") {
		out.RoleCode = r.RoleCode
	}
	if q.Projects("name") {
		out.Name = r.Name
	}
	if q.Projects("status") {
		out.Status = r.Status
	}
	if q.Projects("role_status") {
		out.RoleStatus = r.RoleStatus
	}
	if q.Projects("application_id") {
		out.ApplicationID = r.ApplicationID
	}
	if q.Projects("application_code") {
		out.ApplicationCode = r.ApplicationCode
	}
	if q.Projects("application_status") {
		out.ApplicationStatus = r.ApplicationStatus
	}
	if q.Projects("is_system") {
		out.IsSystem = r.IsSystem
	}
	if q.Projects("application_access_status") {
		out.ApplicationAccessStatus = r.ApplicationAccessStatus
	}
	if q.Projects("granted_by") {
		out.GrantedBy = r.GrantedBy
	}
	if q.Projects("granted_at") {
		out.GrantedAt = r.GrantedAt
	}
	if q.Projects("revoked_by") {
		out.RevokedBy = r.RevokedBy
	}
	if q.Projects("revoked_at") {
		out.RevokedAt = r.RevokedAt
	}
	if q.Projects("revoke_reason") {
		out.RevokeReason = r.RevokeReason
	}
	return out
}
