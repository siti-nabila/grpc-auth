package test_scenarios

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	feature "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Idempotency() []testutils.Scenario {
	return []testutils.Scenario{{Name: "canonical scope fingerprint distinguishes target presence and version", Run: fingerprints}, {Name: "versioned password fingerprint retains old keys and rejects secret replay", Run: passwordFingerprint}, {Name: "replay conflict expiry and no op use controlled 24 hour boundary", Run: replay}}
}

func fingerprints(t *testing.T) {
	scope := fixtures.Scope()
	empty := ""
	payload := feature.FingerprintPayload{Email: " USER@Example.test ", Code: " IAM.ROLE ", ExpectedVersion: 1, RoleIDs: []uint64{3, 2, 3}, PermissionCodes: []string{"b", "a", "b"}, Presence: []string{"name", "phone", "name"}}
	a, err := feature.Fingerprint(scope, payload)
	if err != nil {
		t.Fatal(err)
	}
	equivalent := payload
	equivalent.Email = "user@example.test"
	equivalent.Code = "iam.role"
	equivalent.RoleIDs = []uint64{2, 3}
	equivalent.PermissionCodes = []string{"a", "b"}
	equivalent.Presence = []string{"phone", "name"}
	b, err := feature.Fingerprint(scope, equivalent)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("normalized sets differ")
	}
	if payload.RoleIDs[0] != 3 || payload.Presence[2] != "name" {
		t.Fatal("caller payload mutated")
	}
	for _, change := range []func(*domain.IdempotencyScope, *feature.FingerprintPayload){func(s *domain.IdempotencyScope, p *feature.FingerprintPayload) { s.Target = "user/2" }, func(s *domain.IdempotencyScope, p *feature.FingerprintPayload) { p.ExpectedVersion = 2 }, func(s *domain.IdempotencyScope, p *feature.FingerprintPayload) { p.Name = &empty }, func(s *domain.IdempotencyScope, p *feature.FingerprintPayload) { p.Presence = []string{"name"} }} {
		s, p := scope, payload
		change(&s, &p)
		hash, err := feature.Fingerprint(s, p)
		if err != nil || bytes.Equal(a, hash) {
			t.Fatal("business difference ignored")
		}
	}
	for _, target := range []string{"user/1", "role/2", "user/1/application/3", "user/1/session/" + fixtures.SessionID, "user/1/sessions", "account/user@example.test", "role-code/iam.custom"} {
		s := scope
		s.Target = target
		if err = s.Validate(); err != nil {
			t.Fatal(target, err)
		}
	}
	for _, target := range []string{"user/01", "role/0", "user/1/application/alias", "user/1/session/token", "account/ USER@EXAMPLE.TEST", "user/1/session/" + strings.ToUpper(fixtures.SessionID) + "z"} {
		s := scope
		s.Target = target
		if err = s.Validate(); err == nil {
			t.Fatal("noncanonical target accepted", target)
		}
	}
	for _, change := range []func(*domain.IdempotencyScope){func(s *domain.IdempotencyScope) { s.ActorID = 0 }, func(s *domain.IdempotencyScope) { s.Method = "UpdateUser" }, func(s *domain.IdempotencyScope) { s.Key = " " }, func(s *domain.IdempotencyScope) { s.Key = strings.Repeat("a", 201) }, func(s *domain.IdempotencyScope) { s.Target = strings.Repeat("a", 301) }} {
		s := scope
		change(&s)
		if _, err = feature.IdempotencyLockKey(s); err == nil {
			t.Fatal("invalid scope accepted")
		}
	}
	scope.Key = strings.Repeat("界", 200)
	if _, err = feature.IdempotencyLockKey(scope); err != nil {
		t.Fatal("200 character key rejected")
	}
	x := fixtures.Scope()
	first, _ := feature.IdempotencyLockKey(x)
	x.Target = "user/2"
	second, _ := feature.IdempotencyLockKey(x)
	if first == second || strings.Contains(first, "fixture-key") {
		t.Fatal("scope lock leaks raw key or misses target")
	}
}

func passwordFingerprint(t *testing.T) {
	oldKey := bytes.Repeat([]byte{1}, 32)
	newKey := bytes.Repeat([]byte{2}, 32)
	old, err := feature.NewFingerprinter("old", map[string][]byte{"old": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	scope := fixtures.Scope()
	payload := feature.FingerprintPayload{ExpectedVersion: 1}
	password := []byte("test-password-only")
	hash, err := old.PasswordFingerprint(scope, payload, password)
	if err != nil || bytes.Contains(hash, password) {
		t.Fatal("password exposed")
	}
	oldKey[0] = 9
	unchanged, _ := old.PasswordFingerprint(scope, payload, password)
	if !bytes.Equal(hash, unchanged) {
		t.Fatal("keyring aliases caller memory")
	}
	keys := map[string][]byte{"old": bytes.Repeat([]byte{1}, 32), "new": newKey}
	rotated, _ := feature.NewFingerprinter("new", keys)
	record := &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: `{"changed":true,"user_id":1}`, CreatedAt: fixtures.PersistenceTime, ExpiresAt: fixtures.PersistenceTime.Add(domain.IdempotencyWindow)}
	result, err := rotated.ReplayPassword(record, scope, payload, password, record.ExpiresAt.Add(-time.Nanosecond))
	if err != nil || result == nil || !result.Changed {
		t.Fatal("unexpired old key not retained")
	}
	if _, err = rotated.ReplayPassword(record, scope, payload, []byte("other-password"), fixtures.PersistenceTime); !errors.Is(err, user.ErrIdempotencyConflict) {
		t.Fatal("password conflict ignored")
	}
	removed, _ := feature.NewFingerprinter("new", map[string][]byte{"new": newKey})
	if _, err = removed.ReplayPassword(record, scope, payload, password, fixtures.PersistenceTime); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("missing retained key must fail closed")
	}
	if result, err = removed.ReplayPassword(record, scope, payload, password, record.ExpiresAt); err != nil || result != nil {
		t.Fatal("expired record still needs old key")
	}
	if _, err = feature.NewFingerprinter("new", map[string][]byte{"new": nil}); err == nil {
		t.Fatal("default secret accepted")
	}
}

func replay(t *testing.T) {
	hash, _ := feature.Fingerprint(fixtures.Scope(), feature.FingerprintPayload{ExpectedVersion: 1})
	r := &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: `{"changed":false,"version":1}`, CreatedAt: fixtures.PersistenceTime, ExpiresAt: fixtures.PersistenceTime.Add(domain.IdempotencyWindow)}
	result, err := feature.Replay(r, hash, r.ExpiresAt.Add(-time.Nanosecond))
	if err != nil || result == nil || result.Changed || result.Version != 1 {
		t.Fatal("no op replay differs")
	}
	if _, err = feature.Replay(r, []byte("other"), fixtures.PersistenceTime); !errors.Is(err, user.ErrIdempotencyConflict) {
		t.Fatal("conflict ignored")
	}
	if result, err = feature.Replay(r, hash, r.ExpiresAt); err != nil || result != nil {
		t.Fatal("expiry boundary slid")
	}
	r.SafeResponse = `{"changed":true,"access_token":"secret"}`
	if _, err = feature.Replay(r, hash, fixtures.PersistenceTime); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("unsafe saved result accepted")
	}
	var safe domain.SafeCommandResult
	if err = json.Unmarshal([]byte(`{"changed":false} {}`), &safe); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}
