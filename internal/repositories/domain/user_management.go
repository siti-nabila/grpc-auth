package domain

import (
	"context"
	"math"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// UserDetail is an allowlisted view; credentials and auth generation cannot be
// represented. The reader projects only these columns from the primary snapshot.
type UserDetail struct {
	ID                 uint64                `sql:"column:id"`
	Email              string                `sql:"column:email"`
	Name               string                `sql:"column:name"`
	Address            string                `sql:"column:address"`
	Phone              string                `sql:"column:phone"`
	Status             types.UserStatus      `sql:"column:status"`
	Version            int64                 `sql:"column:version"`
	CreatedAt          time.Time             `sql:"column:created_at"`
	MustChangePassword bool                  `sql:"column:must_change_password"`
	Roles              []UserListRole        `sql:"-"`
	ApplicationAccess  []UserListApplication `sql:"-"`
}

func (UserDetail) TableName() string { return "auth a" }

type UserDetailReader interface {
	Detail(context.Context, uint64) (UserDetail, error)
}

type ManagementPolicyReader interface {
	RoleApplication(context.Context, uint64) (uint64, error)
	AdminCount(context.Context, uint64) (int64, bool, error)
}

type ManagementProfileWriter interface {
	UpdateProfile(context.Context, uint64, UserDetail) (bool, error)
	AdvanceProfileVersion(context.Context, uint64, int64, time.Time) (bool, error)
}

type ManagementStatusWriter interface {
	ChangeStatus(context.Context, SessionUserState, types.UserStatus, bool, time.Time) (bool, error)
}

type CreateUserInput struct {
	Name, Email, Password, Address, Phone string
	Status                                *types.UserStatus
	Key                                   string
}

func (r CreateUserInput) Normalized() CreateUserInput {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	return r
}

func (r CreateUserInput) Validate() error {
	r = r.Normalized()
	address, err := mail.ParseAddress(r.Email)
	if err != nil || address.Address != r.Email || !userText(r.Email, 6, 50) || !userText(r.Password, 6, 50) || !userText(r.Name, 3, 100) || !userText(r.Address, 0, 250) || !userText(r.Phone, 0, 20) || r.Status == nil || (*r.Status != types.UserPending && *r.Status != types.UserActive) || ValidateManagementKey(r.Key, true) != nil {
		return common.ErrBadRequest
	}
	return nil
}

type UpdateUserInput struct {
	UserID               uint64
	ExpectedVersion      int64
	Name, Address, Phone *string
	Fields               []string
	Key                  string
}

func (r UpdateUserInput) Validate() error {
	if ValidateManagementTarget(r.UserID) != nil || r.ExpectedVersion < 1 || ValidateManagementKey(r.Key, false) != nil || len(r.Fields) == 0 {
		return common.ErrBadRequest
	}
	selected := map[string]bool{}
	for _, field := range r.Fields {
		if selected[field] {
			return common.ErrBadRequest
		}
		selected[field] = true
		switch field {
		case "name", "address", "phone":
		default:
			return common.ErrBadRequest
		}
	}
	for field, value := range map[string]*string{"name": r.Name, "address": r.Address, "phone": r.Phone} {
		if selected[field] != (value != nil) {
			return common.ErrBadRequest
		}
	}
	if r.Name != nil && !userText(*r.Name, 3, 100) || r.Address != nil && !userText(*r.Address, 0, 250) || r.Phone != nil && !userText(*r.Phone, 0, 20) {
		return common.ErrBadRequest
	}
	return nil
}

type ChangeUserStatusInput struct {
	UserID          uint64
	ExpectedVersion int64
	Status          *types.UserStatus
	Reason, Key     string
}

func (r ChangeUserStatusInput) Validate() error {
	if ValidateManagementTarget(r.UserID) != nil || r.ExpectedVersion < 1 || r.Status == nil || !ManagementUserStatus(*r.Status) || !userText(strings.TrimSpace(r.Reason), 3, 500) || ValidateManagementKey(r.Key, true) != nil {
		return common.ErrBadRequest
	}
	return nil
}

func ValidateManagementTarget(id uint64) error {
	if id < 1 || id > math.MaxInt32 {
		return common.ErrBadRequest
	}
	return nil
}

func ValidateManagementKey(key string, required bool) error {
	if key == "" && !required {
		return nil
	}
	if strings.TrimSpace(key) == "" || !userText(key, 1, 200) {
		return common.ErrBadRequest
	}
	return nil
}

func ManagementUserStatus(status types.UserStatus) bool {
	switch status {
	case types.UserPending, types.UserActive, types.UserSuspended, types.UserDeactivated:
		return true
	}
	return false
}

func UserStatusTransitionAllowed(before, after types.UserStatus) bool {
	if before == after {
		return ManagementUserStatus(before)
	}
	switch before {
	case types.UserPending:
		return after == types.UserActive || after == types.UserDeactivated
	case types.UserActive:
		return after == types.UserSuspended || after == types.UserDeactivated
	case types.UserSuspended:
		return after == types.UserActive || after == types.UserDeactivated
	case types.UserDeactivated:
		return after == types.UserActive
	}
	return false
}

func userText(value string, min, max int) bool {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return false
	}
	n := utf8.RuneCountInString(value)
	return n >= min && n <= max
}
