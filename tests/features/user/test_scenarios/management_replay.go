package test_scenarios

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func ManagementReplay() []testutils.Scenario {
	return []testutils.Scenario{{Name: "safe detail replay preserves original fields with retained digest rotation expiry and strict decode", Run: managementSafeReplay}}
}
func managementSafeReplay(t *testing.T) {
	scope := domain.IdempotencyScope{ActorID: 42, Method: "/user.v1.UserService/CreateUser", Target: "account/target@example.test", Key: "creation"}
	name, address, phone := "Target user", "", "123"
	payload := featurecommon.FingerprintPayload{Email: "target@example.test", Name: &name, Address: &address, Phone: &phone, Status: "ACTIVE", Presence: []string{"name", "address", "phone", "status"}}
	key := bytes.Repeat([]byte{1}, 32)
	old, e := featurecommon.NewFingerprinter("v1", map[string][]byte{"v1": key})
	if e != nil {
		t.Fatal(e)
	}
	hash, e := old.PasswordFingerprint(scope, payload, []byte("synthetic-password"))
	if e != nil {
		t.Fatal(e)
	}
	initial := domain.UserDetail{ID: 43, Email: payload.Email, Name: name, Address: address, Phone: phone, Status: types.UserActive, Version: 1, CreatedAt: fixtures.ServiceTime, MustChangePassword: true, Roles: []domain.UserListRole{{ID: 1, Code: "iam.member", Name: "Member", Status: types.AssignmentActive, RoleStatus: types.RoleActive}}, ApplicationAccess: []domain.UserListApplication{{ID: 1, Code: "iam", Name: "IAM", Status: types.ApplicationAccessActive, ApplicationStatus: types.ApplicationActive}}}
	saved := domain.UserDetailCommandResult(initial, true)
	data, e := json.Marshal(saved)
	if e != nil {
		t.Fatal(e)
	}
	record := &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: string(data), ExpiresAt: fixtures.ServiceTime.Add(domain.IdempotencyWindow)}
	rotated, e := featurecommon.NewFingerprinter("v2", map[string][]byte{"v1": key, "v2": bytes.Repeat([]byte{2}, 32)})
	if e != nil {
		t.Fatal(e)
	}
	result, e := rotated.ReplayPassword(record, scope, payload, []byte("synthetic-password"), fixtures.ServiceTime.Add(time.Hour))
	if e != nil || result == nil {
		t.Fatal("retained replay", e)
	}
	detail, e := result.UserDetail()
	if e != nil || !reflect.DeepEqual(initial, detail) {
		t.Fatal("saved safe fields changed", detail, e)
	}
	if _, e = rotated.ReplayPassword(record, scope, payload, []byte("different-password"), fixtures.ServiceTime); !errors.Is(e, usererrors.ErrIdempotencyConflict) {
		t.Fatal("password conflict", e)
	}
	changed := payload
	changed.Address = nil
	changed.Presence = []string{"name", "phone", "status"}
	if _, e = rotated.ReplayPassword(record, scope, changed, []byte("synthetic-password"), fixtures.ServiceTime); !errors.Is(e, usererrors.ErrIdempotencyConflict) {
		t.Fatal("presence conflict", e)
	}
	removed, _ := featurecommon.NewFingerprinter("v2", map[string][]byte{"v2": bytes.Repeat([]byte{2}, 32)})
	if _, e = removed.ReplayPassword(record, scope, payload, []byte("synthetic-password"), fixtures.ServiceTime); !errors.Is(e, common.ErrDependencyUnavailable) {
		t.Fatal("missing retained key accepted", e)
	}
	if r, e := removed.ReplayPassword(record, scope, payload, []byte("synthetic-password"), record.ExpiresAt); e != nil || r != nil {
		t.Fatal("expired record replayed", e)
	}
	for _, bad := range []string{`{"password":"private"}`, `{"email":"Target@example.test"}`, `{"roles":[{"ID":1,"Code":"iam.member","Name":"Member","Status":"REVOKED","RoleStatus":"ACTIVE"}]}`, `{"application_access":[{"ID":1,"Code":"iam","Name":"IAM","Status":"ACTIVE","ApplicationStatus":"DISABLED"}]}`, strings.TrimSuffix(string(data), "}") + `,"token":"private"}`} {
		var value domain.SafeCommandResult
		if json.Unmarshal([]byte(bad), &value) == nil {
			t.Fatal("unsafe result accepted")
		}
	}
}
