package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/orm/orm"
)

const (
	UserSortID                = "id"
	UserSortName              = "name"
	UserSortEmail             = "email"
	UserSortAddress           = "address"
	UserSortPhone             = "phone"
	UserSortStatus            = "status"
	UserSortVersion           = "version"
	UserSortCreatedAt         = "created_at"
	UserSortRoles             = "roles"
	UserSortApplicationAccess = "application_access"
)

type UserListRole struct {
	UserID     int64                  `sql:"column:user_id" json:"-"`
	ID         uint64                 `sql:"column:role_id" json:"role_id"`
	Code       string                 `sql:"column:code" json:"code"`
	Name       string                 `sql:"column:name" json:"name"`
	Status     types.AssignmentStatus `sql:"column:status" json:"status"`
	RoleStatus types.RoleStatus       `sql:"column:role_status" json:"role_status"`
}

func (UserListRole) TableName() string { return "user_role ur" }

type UserListApplication struct {
	UserID            int64                         `sql:"column:user_id" json:"-"`
	ID                uint64                        `sql:"column:application_id" json:"application_id"`
	Code              string                        `sql:"column:code" json:"code"`
	Name              string                        `sql:"column:name" json:"name"`
	Status            types.ApplicationAccessStatus `sql:"column:status" json:"status"`
	ApplicationStatus types.ApplicationStatus       `sql:"column:application_status" json:"application_status"`
}

func (UserListApplication) TableName() string { return "user_application_access ua" }

// Cursor values carry an explicit type and a nullable value. They contain only
// public ordering values; neither column names nor SQL come from the caller.
type UserSortValue struct {
	Kind  string  `json:"kind"`
	Value *string `json:"value"`
}
type UserListCursor struct {
	Version int             `json:"v"`
	Query   string          `json:"query"`
	Values  []UserSortValue `json:"values"`
}

func UserSortKind(field string) string {
	switch field {
	case UserSortID, UserSortVersion:
		return "integer"
	case UserSortCreatedAt:
		return "time"
	case UserSortName, UserSortEmail, UserSortAddress, UserSortPhone, UserSortStatus, UserSortRoles, UserSortApplicationAccess:
		return "text"
	default:
		return ""
	}
}
func NormalizeUserListSort(in []orm.SortField) ([]orm.SortField, error) {
	out := make([]orm.SortField, 0, len(in)+1)
	seen := map[string]bool{}
	for _, s := range in {
		if s.Field == UserListCursorField {
			s.Field = UserSortID
		}
		if UserSortKind(s.Field) == "" || seen[s.Field] {
			return nil, common.ErrBadRequest
		}
		seen[s.Field] = true
		out = append(out, s)
	}
	if !seen[UserSortID] {
		out = append(out, orm.SortField{Field: UserSortID})
	}
	return out, nil
}
func NormalizeUserListFilter(f UserListFilter) (UserListFilter, error) {
	if f.CreatedFrom != nil && f.CreatedTo != nil && !f.CreatedFrom.Before(*f.CreatedTo) {
		return f, common.ErrBadRequest
	}
	if f.CreatedFrom != nil {
		at := f.CreatedFrom.UTC()
		f.CreatedFrom = &at
	}
	if f.CreatedTo != nil {
		at := f.CreatedTo.UTC()
		f.CreatedTo = &at
	}
	roles := map[uint64]bool{}
	for _, id := range f.RoleCodes {
		if id == 0 || id > math.MaxInt32 {
			return f, common.ErrBadRequest
		}
		roles[id] = true
	}
	f.RoleCodes = make([]uint64, 0, len(roles))
	for id := range roles {
		f.RoleCodes = append(f.RoleCodes, id)
	}
	sort.Slice(f.RoleCodes, func(i, j int) bool { return f.RoleCodes[i] < f.RoleCodes[j] })
	statuses := map[types.UserStatus]bool{}
	for _, s := range f.Statuses {
		switch s {
		case types.UserPending, types.UserActive, types.UserSuspended, types.UserDeactivated:
		default:
			return f, common.ErrBadRequest
		}
		statuses[s] = true
	}
	f.Statuses = make([]types.UserStatus, 0, len(statuses))
	for s := range statuses {
		f.Statuses = append(f.Statuses, s)
	}
	sort.Slice(f.Statuses, func(i, j int) bool { return f.Statuses[i] < f.Statuses[j] })
	if len(f.ApplicationCode) > 50 || strings.TrimSpace(f.ApplicationCode) != f.ApplicationCode {
		return f, common.ErrBadRequest
	}
	return f, nil
}
func UserListQueryHash(opts orm.QueryOptions, f UserListFilter) string {
	b, _ := json.Marshal(struct {
		Sort   []orm.SortField
		Filter UserListFilter
		Search *orm.SearchQuery
	}{opts.Sort, f, opts.Search})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func ParseUserListCursor(raw string, opts orm.QueryOptions, f UserListFilter) (any, error) {
	if IsEmptyUserListLastID(raw) {
		return "", nil
	}
	if len(raw) > 16384 {
		return nil, common.ErrBadRequest
	}
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if id < 1 || id > math.MaxInt32 || len(opts.Sort) != 1 || opts.Sort[0].Field != UserSortID {
			return nil, common.ErrBadRequest
		}
		return id, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(b) != raw {
		return nil, common.ErrBadRequest
	}
	var c UserListCursor
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Version != 1 || c.Query != UserListQueryHash(opts, f) || len(c.Values) != len(opts.Sort) {
		return nil, common.ErrBadRequest
	}
	canonical, _ := json.Marshal(c)
	if !bytes.Equal(canonical, b) {
		return nil, common.ErrBadRequest
	}
	for i, v := range c.Values {
		field := opts.Sort[i].Field
		if v.Kind != UserSortKind(field) {
			return nil, common.ErrBadRequest
		}
		if v.Value == nil {
			if field != UserSortName && field != UserSortAddress && field != UserSortPhone && field != UserSortRoles && field != UserSortApplicationAccess {
				return nil, common.ErrBadRequest
			}
			continue
		}
		switch v.Kind {
		case "integer":
			n, e := strconv.ParseInt(*v.Value, 10, 64)
			if e != nil || n < 1 || strconv.FormatInt(n, 10) != *v.Value || (field == UserSortID && n > math.MaxInt32) {
				return nil, common.ErrBadRequest
			}
		case "time":
			t, e := time.Parse(time.RFC3339Nano, *v.Value)
			if e != nil || t.IsZero() || t.UTC().Format(time.RFC3339Nano) != *v.Value {
				return nil, common.ErrBadRequest
			}
		}
	}
	return c, nil
}
func UserListNextCursor(row UserSearchRow, opts orm.QueryOptions, f UserListFilter) string {
	if len(opts.Sort) == 1 && opts.Sort[0].Field == UserSortID {
		return strconv.FormatInt(row.AuthID, 10)
	}
	c := UserListCursor{Version: 1, Query: UserListQueryHash(opts, f), Values: make([]UserSortValue, 0, len(opts.Sort))}
	for _, s := range opts.Sort {
		c.Values = append(c.Values, row.SortValue(s.Field))
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (r UserSearchRow) SortValue(field string) UserSortValue {
	v := UserSortValue{Kind: UserSortKind(field)}
	text := ""
	valid := true
	switch field {
	case UserSortID:
		text = strconv.FormatInt(r.AuthID, 10)
	case UserSortVersion:
		text = strconv.FormatInt(r.Version, 10)
	case UserSortCreatedAt:
		text = r.CreatedAt.UTC().Format(time.RFC3339Nano)
	case UserSortEmail:
		text = r.Email
	case UserSortStatus:
		text = string(r.Status)
	case UserSortName:
		text, valid = r.NameKey.String, r.NameKey.Valid
	case UserSortAddress:
		text, valid = r.AddressKey.String, r.AddressKey.Valid
	case UserSortPhone:
		text, valid = r.PhoneKey.String, r.PhoneKey.Valid
	case UserSortRoles:
		text, valid = r.RolesKey.String, r.RolesKey.Valid
	case UserSortApplicationAccess:
		text, valid = r.ApplicationsKey.String, r.ApplicationsKey.Valid
	}
	if valid {
		v.Value = &text
	}
	return v
}
