package test_scenarios

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Results() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "safe saved response preserves nullable metadata counts and final permissions", Run: savedRoleResult},
		{Name: "unknown unsafe mixed and inconsistent saved role fields rejected", Run: rejectedRoleResults},
		{Name: "fingerprints canonicalize set order but preserve presence and version", Run: roleFingerprint},
		{Name: "audit and event permission length matches registered schema150", Run: permissionPersistenceBounds},
	}
}

func roleDetailFixture() domain.RoleDetail {
	at := time.Unix(1735689600, 0).UTC()
	return domain.RoleDetail{
		Role:        domain.RoleSummary{ID: 7, Code: "clm.reviewer", Name: "Reviewer", ApplicationID: 2, ApplicationCode: "clm", Status: types.RoleActive, Version: 3, NumericCode: 15, CreatedAt: &at, AffectedUserCount: 9},
		Permissions: []domain.PermissionItem{{ID: 4, ApplicationID: 2, ApplicationCode: "clm", Code: "clm.contract.read", Description: "Read contracts"}},
	}
}

func savedRoleResult(t *testing.T) {
	for _, description := range []*string{nil, text(""), text("Description")} {
		for _, timestamp := range []*time.Time{nil, roleDetailFixture().Role.CreatedAt} {
			before := roleDetailFixture()
			before.Role.Description, before.Role.CreatedAt, before.Role.UpdatedAt = description, timestamp, timestamp
			result := domain.RoleCommandResult(before, true)
			data, err := json.Marshal(result)
			if err != nil || result.Validate() != nil {
				t.Fatal("safe result serialization failed", err)
			}
			record := &domain.IdempotencyRecord{SafeResponse: string(data), RequestHash: []byte("fixture-hash"), ExpiresAt: time.Unix(1735689700, 0)}
			replayed, err := featurecommon.Replay(record, record.RequestHash, time.Unix(1735689600, 0))
			if err != nil || replayed == nil {
				t.Fatal("replay failed", err)
			}
			after, err := replayed.RoleDetail()
			if err != nil || !reflect.DeepEqual(before, after) || !replayed.Changed {
				t.Fatal("original saved response lost", err)
			}
			before.Role.Version, before.Role.AffectedUserCount = 100, 0
			if after.Role.Version != 3 || after.Role.AffectedUserCount != 9 {
				t.Fatal("replay changed with current target state")
			}
		}
	}
	// Permission ordering in stored outcomes is canonical, independent of caller slices.
	detail := roleDetailFixture()
	detail.Permissions = append([]domain.PermissionItem{{ID: 5, ApplicationID: 2, ApplicationCode: "clm", Code: "clm.contract.update"}}, detail.Permissions...)
	r := domain.RoleCommandResult(detail, false)
	if r.Permissions[0].Code != "clm.contract.read" || detail.Permissions[0].Code != "clm.contract.update" {
		t.Fatal("result ordering or caller isolation failed")
	}
}

func rejectedRoleResults(t *testing.T) {
	for _, raw := range []string{
		`{"changed":false,"password":"hidden"}`,
		`{"changed":false,"role":{"token":"hidden"}}`,
		`{"changed":false,"permissions":[{"hash":"hidden"}]}`,
		`{"changed":false} {"changed":true}`,
	} {
		var r domain.SafeCommandResult
		if json.Unmarshal([]byte(raw), &r) == nil {
			t.Fatal("unsafe field or trailing object accepted")
		}
	}
	for _, change := range []func(*domain.SafeCommandResult){
		func(r *domain.SafeCommandResult) { r.RoleID++ },
		func(r *domain.SafeCommandResult) { r.Version++ },
		func(r *domain.SafeCommandResult) { r.Status = "DISABLED" },
		func(r *domain.SafeCommandResult) { r.UserID = 12 },
		func(r *domain.SafeCommandResult) { r.SessionID = "123" },
		func(r *domain.SafeCommandResult) { r.Role.ApplicationCode = "other" },
		func(r *domain.SafeCommandResult) { r.Role.NumericCode = -1 },
		func(r *domain.SafeCommandResult) { r.Role.Name = strings.Repeat("x", 51) },
		func(r *domain.SafeCommandResult) { r.Permissions[0].ApplicationID = 3 },
		func(r *domain.SafeCommandResult) { r.Permissions[0].Code = "iam.role.read" },
		func(r *domain.SafeCommandResult) { r.Permissions = append(r.Permissions, r.Permissions[0]) },
	} {
		r := domain.RoleCommandResult(roleDetailFixture(), true)
		change(&r)
		if r.Validate() == nil {
			t.Fatal("inconsistent safe role result accepted")
		}
	}
}

func roleFingerprint(t *testing.T) {
	scope := domain.IdempotencyScope{ActorID: 1, Method: "/role.v1.RoleService/ReplaceRolePermissions", Target: "role/7", Key: "key"}
	p := featurecommon.FingerprintPayload{ExpectedVersion: 3, Presence: []string{"permissions", "reason"}, PermissionCodes: []string{"clm.contract.update", "clm.contract.read", "clm.contract.read"}}
	a, err := featurecommon.Fingerprint(scope, p)
	if err != nil {
		t.Fatal(err)
	}
	p.PermissionCodes = []string{"clm.contract.read", "clm.contract.update"}
	p.Presence = []string{"reason", "permissions"}
	b, err := featurecommon.Fingerprint(scope, p)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("set ordering changed fingerprint")
	}
	for _, change := range []func(*featurecommon.FingerprintPayload){
		func(p *featurecommon.FingerprintPayload) { p.ExpectedVersion++ },
		func(p *featurecommon.FingerprintPayload) { p.Presence = nil },
		func(p *featurecommon.FingerprintPayload) { p.PermissionCodes = nil },
		func(p *featurecommon.FingerprintPayload) { p.Description = text("") },
	} {
		q := p
		change(&q)
		b, err := featurecommon.Fingerprint(scope, q)
		if err != nil || bytes.Equal(a, b) {
			t.Fatal("changed presence/version/input did not change fingerprint")
		}
	}
}

func permissionPersistenceBounds(t *testing.T) {
	for _, count := range []int{141, 142} {
		code := "clm." + strings.Repeat("a", count) + ".read"
		snapshot := domain.SafeSnapshot{PermissionCodes: []string{code}}
		event := domain.EventEnvelope{EventID: "00000000-0000-0000-0000-000000000001", EventType: types.EventRolePermissionsChanged, AggregateType: types.AggregateRole, AggregateID: "7", AggregateVersion: 3, SchemaVersion: 1, Producer: "grpc-auth", OccurredAt: time.Unix(1735689600, 0), Data: domain.EventData{RoleID: 7, PermissionCodes: []string{code}}}
		_, err := event.Record()
		if (snapshot.Validate() == nil) != (count == 141) || (err == nil) != (count == 141) {
			t.Fatal("permission persistence bound mismatch")
		}
	}
}
