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

// UserID binds mutation reads; it is not a public list scalar.
type ApplicationAccessItem struct {
	UserID            uint64     `sql:"column:user_id"`
	ApplicationID     uint64     `sql:"column:application_id"`
	ApplicationCode   string     `sql:"column:application_code"`
	Name              string     `sql:"column:name"`
	ApplicationStatus string     `sql:"column:application_status"`
	IsSystem          bool       `sql:"column:is_system"`
	Status            string     `sql:"column:status"`
	GrantedBy         *uint64    `sql:"column:granted_by"`
	GrantedAt         *time.Time `sql:"column:granted_at"`
	RevokedBy         *uint64    `sql:"column:revoked_by"`
	RevokedAt         *time.Time `sql:"column:revoked_at"`
	RevokeReason      *string    `sql:"column:revoke_reason"`
}

func (ApplicationAccessItem) TableName() string { return "user_application_access access" }

type ApplicationAccessInput struct {
	UserID          uint64
	ApplicationCode string
	RoleIDs         []uint64
	ExpectedVersion int64
	Reason, Key     string
}

func (r ApplicationAccessInput) validate() error {
	if ValidateManagementTarget(r.UserID) != nil || r.ExpectedVersion < 1 || !applicationCodeValid(strings.ToLower(strings.TrimSpace(r.ApplicationCode))) || ValidateAssignmentReason(r.Reason) != nil || ValidateManagementKey(r.Key, true) != nil {
		return common.ErrBadRequest
	}
	return nil
}

func (r ApplicationAccessInput) ValidateGrant() error {
	if r.validate() != nil || len(r.RoleIDs) < 1 || len(r.RoleIDs) > 20 {
		return common.ErrBadRequest
	}
	for _, id := range r.RoleIDs {
		if ValidateManagementTarget(id) != nil {
			return common.ErrBadRequest
		}
	}
	return nil
}

func (r ApplicationAccessInput) ValidateRevoke() error {
	if r.validate() != nil || len(r.RoleIDs) != 0 {
		return common.ErrBadRequest
	}
	return nil
}

func (r ApplicationAccessInput) Normalized() ApplicationAccessInput {
	r.ApplicationCode = strings.ToLower(strings.TrimSpace(r.ApplicationCode))
	r.Reason = strings.TrimSpace(r.Reason)
	r.RoleIDs = slices.Clone(r.RoleIDs)
	slices.Sort(r.RoleIDs)
	r.RoleIDs = slices.Compact(r.RoleIDs)
	return r
}

type ApplicationLookup interface {
	Application(context.Context, string) (ApplicationItem, error)
}

type ApplicationAccessReader interface {
	Access(context.Context, uint64, uint64) (*ApplicationAccessItem, error)
	ApplicationAssignments(context.Context, uint64, uint64) ([]UserRoleAssignmentItem, error)
	ListAccess(context.Context, CatalogQuery) (orm.PageData[ApplicationAccessItem], error)
}

type ApplicationAccessWriter interface {
	Activate(context.Context, uint64, uint64, *ApplicationAccessItem, uint64, time.Time) (bool, error)
	Revoke(context.Context, uint64, ApplicationAccessItem, uint64, string, time.Time) (bool, error)
}

type ApplicationAccessPage struct {
	UserID  uint64
	Version int64
	Page    orm.PageData[ApplicationAccessItem]
}

// D permits access-only changes and more than20 revoked roles; C remains bounded.
func (r SafeCommandResult) ValidateApplicationAccessResult(userID, applicationID uint64) error {
	if r.Validate() != nil || ValidateManagementTarget(userID) != nil || ValidateManagementTarget(applicationID) != nil || r.UserID != userID || r.ApplicationID != applicationID || r.Version < 1 || !r.Changed && len(r.ChangedRoleIDs) != 0 {
		return common.ErrInternalServerError
	}
	if r.Status != "" && r.Status != "ACTIVE" && r.Status != "REVOKED" {
		return common.ErrInternalServerError
	}
	if r.RoleID != 0 || r.SessionID != "" || r.Name != nil || r.Address != nil || r.Phone != nil || r.LoginRequired || r.Email != nil || r.CreatedAt != nil || r.MustChangePassword != nil || len(r.Roles) != 0 || len(r.ApplicationAccess) != 0 || r.Role != nil || len(r.Permissions) != 0 {
		return common.ErrInternalServerError
	}
	for i, id := range r.ChangedRoleIDs {
		if ValidateManagementTarget(id) != nil || i > 0 && id <= r.ChangedRoleIDs[i-1] {
			return common.ErrInternalServerError
		}
	}
	return nil
}

func (r ApplicationAccessItem) CatalogValue(field string) CatalogValue {
	switch field {
	case "application_id":
		return catalogText("integer", strconv.FormatUint(r.ApplicationID, 10))
	case "application_code":
		return catalogText("text", r.ApplicationCode)
	case "name":
		return catalogText("text", r.Name)
	case "application_status":
		return catalogText("text", r.ApplicationStatus)
	case "is_system":
		return catalogText("bool", strconv.FormatBool(r.IsSystem))
	case "status":
		return catalogText("text", r.Status)
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
		return CatalogValue{Kind: "text", Value: r.RevokeReason}
	default:
		return CatalogValue{}
	}
}

func (r ApplicationAccessItem) Project(q CatalogQuery) ApplicationAccessItem {
	out := ApplicationAccessItem{ApplicationID: r.ApplicationID}
	if q.Projects("application_code") {
		out.ApplicationCode = r.ApplicationCode
	}
	if q.Projects("name") {
		out.Name = r.Name
	}
	if q.Projects("application_status") {
		out.ApplicationStatus = r.ApplicationStatus
	}
	if q.Projects("is_system") {
		out.IsSystem = r.IsSystem
	}
	if q.Projects("status") {
		out.Status = r.Status
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
