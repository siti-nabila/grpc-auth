package domain

import (
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

const IdempotencyWindow = 24 * time.Hour

var rpcMethodPattern = regexp.MustCompile(`^/[A-Za-z][A-Za-z0-9_.]*\.[A-Za-z][A-Za-z0-9_]*/[A-Za-z][A-Za-z0-9_]*$`)
var digestVersionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// Accept only the foundation's versioned digests, never arbitrary password,
// authentication-hash or token bytes supplied as a fingerprint.
func ValidateRequestHash(hash []byte) error {
	parts := strings.Split(string(hash), ":")
	var digest string
	switch {
	case len(parts) == 2 && parts[0] == "sha256-v1":
		digest = parts[1]
	case len(parts) == 3 && parts[0] == "hmac-v1" && digestVersionPattern.MatchString(parts[1]):
		digest = parts[2]
	default:
		return common.ErrBadRequest
	}
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		return common.ErrBadRequest
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return common.ErrBadRequest
	}
	return nil
}

// Identity and current endpoint permission must be checked by the consumer.
// Application aliases must already be resolved to the authoritative numeric ID.
type IdempotencyScope struct {
	ActorID uint64
	Method  string
	Target  string
	Key     string
}

func (s IdempotencyScope) Validate() error {
	if s.ActorID == 0 || !boundedText(s.Method, 100, true) || !rpcMethodPattern.MatchString(s.Method) || !boundedText(s.Target, 300, true) || !boundedText(s.Key, 200, true) || !canonicalTarget(s.Target) {
		return common.ErrBadRequest
	}
	return nil
}

func canonicalTarget(target string) bool {
	p := strings.Split(target, "/")
	if len(p) == 2 && p[0] == "account" {
		return p[1] != "" && strings.Contains(p[1], "@") && strings.TrimSpace(strings.ToLower(p[1])) == p[1]
	}
	if len(p) == 2 && p[0] == "role-code" {
		return p[1] != "" && strings.TrimSpace(strings.ToLower(p[1])) == p[1]
	}
	if len(p) < 2 || (p[0] != "user" && p[0] != "role") || !positiveDecimal(p[1]) {
		return false
	}
	if len(p) == 2 {
		return true
	}
	if p[0] != "user" {
		return false
	}
	if len(p) == 3 {
		return p[2] == "sessions"
	}
	return len(p) == 4 && ((p[2] == "application" && positiveDecimal(p[3])) || (p[2] == "session" && canonicalUUID.MatchString(p[3])))
}

func positiveDecimal(value string) bool {
	id, err := strconv.ParseUint(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatUint(id, 10) == value
}

// SafeCommandResult deliberately cannot represent a token or credential response.
type SafeCommandResult struct {
	UserID             uint64                `json:"user_id,omitempty"`
	RoleID             uint64                `json:"role_id,omitempty"`
	Version            int64                 `json:"version,omitempty"`
	Changed            bool                  `json:"changed"`
	ChangedRoleIDs     []uint64              `json:"changed_role_ids,omitempty"`
	ApplicationID      uint64                `json:"application_id,omitempty"`
	SessionID          string                `json:"session_id,omitempty"`
	Status             string                `json:"status,omitempty"`
	Name               *string               `json:"name,omitempty"`
	Address            *string               `json:"address,omitempty"`
	Phone              *string               `json:"phone,omitempty"`
	LoginRequired      bool                  `json:"login_required,omitempty"`
	Email              *string               `json:"email,omitempty"`
	CreatedAt          *time.Time            `json:"created_at,omitempty"`
	MustChangePassword *bool                 `json:"must_change_password,omitempty"`
	Roles              []UserListRole        `json:"roles,omitempty"`
	ApplicationAccess  []UserListApplication `json:"application_access,omitempty"`
}

func (r SafeCommandResult) Validate() error {
	if r.Email != nil && (!userText(*r.Email, 6, 50) || strings.TrimSpace(strings.ToLower(*r.Email)) != *r.Email || !strings.Contains(*r.Email, "@")) {
		return common.ErrBadRequest
	}
	if r.CreatedAt != nil && r.CreatedAt.IsZero() {
		return common.ErrBadRequest
	}
	for _, role := range r.Roles {
		if ValidateManagementTarget(role.ID) != nil || !boundedText(role.Code, 100, true) || !boundedText(role.Name, 100, true) || role.Status != types.AssignmentActive || role.RoleStatus != types.RoleActive {
			return common.ErrBadRequest
		}
	}
	for _, app := range r.ApplicationAccess {
		if ValidateManagementTarget(app.ID) != nil || !boundedText(app.Code, 50, true) || !boundedText(app.Name, 100, true) || app.Status != types.ApplicationAccessActive || app.ApplicationStatus != types.ApplicationActive {
			return common.ErrBadRequest
		}
	}
	return (SafeSnapshot{Version: r.Version, Status: r.Status, Name: r.Name, Address: r.Address, Phone: r.Phone, RoleIDs: r.ChangedRoleIDs, SessionID: r.SessionID}).Validate()
}

// UserDetail returns the stored initial safe response, never mutable current data.
func (r SafeCommandResult) UserDetail() (UserDetail, error) {
	if err := r.Validate(); err != nil {
		return UserDetail{}, err
	}
	if ValidateManagementTarget(r.UserID) != nil || r.Version < 1 || !ManagementUserStatus(types.UserStatus(r.Status)) || r.Email == nil || r.CreatedAt == nil || r.MustChangePassword == nil || r.Name == nil || r.Address == nil || r.Phone == nil {
		return UserDetail{}, common.ErrInternalServerError
	}
	return UserDetail{ID: r.UserID, Email: *r.Email, Name: *r.Name, Address: *r.Address, Phone: *r.Phone, Status: types.UserStatus(r.Status), Version: r.Version, CreatedAt: *r.CreatedAt, MustChangePassword: *r.MustChangePassword, Roles: r.Roles, ApplicationAccess: r.ApplicationAccess}, nil
}

func UserDetailCommandResult(r UserDetail, changed bool) SafeCommandResult {
	return SafeCommandResult{UserID: r.ID, Version: r.Version, Changed: changed, Status: string(r.Status), Email: &r.Email, Name: &r.Name, Address: &r.Address, Phone: &r.Phone, CreatedAt: &r.CreatedAt, MustChangePassword: &r.MustChangePassword, Roles: r.Roles, ApplicationAccess: r.ApplicationAccess}
}

func (r *SafeCommandResult) UnmarshalJSON(data []byte) error {
	type fields SafeCommandResult
	var value fields
	if err := decodeSafeJSON(data, &value); err != nil {
		return err
	}
	if err := SafeCommandResult(value).Validate(); err != nil {
		return err
	}
	*r = SafeCommandResult(value)
	return nil
}

type IdempotencyRecord struct {
	ActorID      uint64    `sql:"column:actor_id"`
	Method       string    `sql:"column:method"`
	RouteScope   string    `sql:"column:route_scope"`
	Key          string    `sql:"column:key"`
	RequestHash  []byte    `sql:"column:request_hash"`
	SafeResponse string    `sql:"column:safe_response"`
	CreatedAt    time.Time `sql:"column:created_at"`
	ExpiresAt    time.Time `sql:"column:expires_at"`
}

func (IdempotencyRecord) TableName() string { return "idempotency_records" }
func (r IdempotencyRecord) Result() (SafeCommandResult, error) {
	var result SafeCommandResult
	err := json.Unmarshal([]byte(r.SafeResponse), &result)
	if err != nil {
		return SafeCommandResult{}, common.ErrInternalServerError
	}
	return result, nil
}

type IdempotencyStore interface {
	Lookup(IdempotencyScope) (*IdempotencyRecord, error)
	Save(IdempotencyScope, []byte, SafeCommandResult, time.Time) error
}
