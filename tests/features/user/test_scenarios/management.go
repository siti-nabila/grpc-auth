package test_scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	authmocks "github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	sessionmocks "github.com/siti-nabila/grpc-auth/tests/features/session/mocks"
	"github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Management() []testutils.Scenario {
	return []testutils.Scenario{{Name: "three admin commands serialize recheck replay and roll back every failed stage", Run: managementMatrix}}
}

func managementMatrix(t *testing.T) {
	for _, command := range []string{"create", "update", "status"} {
		cases := []string{"success", "replay", "conflict", "expired", "stale actor", "restricted", "denied", "busy", "begin failure", "lock failure", "commit failure"}
		stages := []string{"permissions1", "permissions2", "permissions3", "state", "lookup", "audit", "event-id", "outbox", "save"}
		switch command {
		case "create":
			stages = append(stages, "member", "role-app1", "role-app2", "account", "hash", "create", "profile-create", "member-assign", "detail")
			cases = append(cases, "duplicate", "PENDING")
		case "update":
			stages = append(stages, "ids1", "ids2", "detail", "profile-version", "profile-write")
			cases = append(cases, "stale version", "no-op", "zero version write", "zero profile write", "optional key")
		case "status":
			stages = append(stages, "ids1", "ids2", "target", "admin-count", "status-write", "sessions", "refresh")
			cases = append(cases, "stale version", "no-op", "last admin", "count failure", "zero status write", "activate", "illegal transition", "self")
		}
		for _, stage := range stages {
			for _, failure := range []string{"error", "panic", "cancel"} {
				cases = append(cases, failure+":"+stage)
			}
		}
		for _, kind := range cases {
			t.Run(command+"/"+kind, func(t *testing.T) { managementCommandCase(t, command, kind) })
		}
	}
}

func managementCommandCase(t *testing.T, command, kind string) {
	t.Helper()
	method := userv1.UserService_CreateUser_FullMethodName
	permission := types.PermissionIAMUserCreate
	if command == "update" {
		method = userv1.UserService_UpdateUser_FullMethodName
		permission = types.PermissionIAMUserUpdate
	}
	if command == "status" {
		method = userv1.UserService_ChangeUserStatus_FullMethodName
		permission = types.PermissionIAMUserStatusUpdate
	}
	ctx := fixtures.CallerContext(t, context.Background(), method, method)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	state := fixtures.SessionState()
	p, err := sessions.PrincipalForState(fixtures.AccessClaims(t), state, fixtures.ServiceTime)
	if err != nil {
		t.Fatal(err)
	}
	ctx = p.Context(ctx)
	tx := &shared.Transaction{Context: ctx}
	provider := &shared.TransactionProvider{Tx: tx}
	cause := errors.New("controlled private command failure")
	if kind == "begin failure" {
		provider.Error = cause
	}
	step := func(stage string) error {
		tx.Record(stage)
		switch kind {
		case "error:" + stage:
			return cause
		case "panic:" + stage:
			panic("private failure")
		case "cancel:" + stage:
			cancel()
			return context.Canceled
		}
		return nil
	}
	if kind == "busy" {
		tx.Busy = true
	}
	if kind == "lock failure" {
		tx.LockError = cause
	}
	if kind == "commit failure" {
		tx.CommitError = cause
	}
	status := types.UserActive
	if kind == "PENDING" {
		status = types.UserPending
	}
	name := "Updated target"
	if kind == "no-op" {
		name = "Target user"
	}
	create := domain.CreateUserInput{Name: "Target user", Email: " TARGET@EXAMPLE.TEST ", Password: "synthetic-password", Status: &status, Key: "test-key"}
	update := domain.UpdateUserInput{UserID: 43, ExpectedVersion: 7, Name: &name, Fields: []string{"name"}, Key: "test-key"}
	status = types.UserSuspended
	if command == "create" {
		status = types.UserActive
		if kind == "PENDING" {
			status = types.UserPending
		}
	}
	if kind == "activate" || kind == "no-op" && command == "status" {
		status = types.UserActive
	}
	if kind == "illegal transition" {
		status = types.UserPending
	}
	change := domain.ChangeUserStatusInput{UserID: 43, ExpectedVersion: 7, Status: &status, Reason: "Policy review", Key: "test-key"}
	if kind == "self" {
		change.UserID = 42
	}
	if kind == "optional key" {
		update.Key = ""
	}
	detail := domain.UserDetail{ID: 43, Email: "target@example.test", Name: "Target user", Status: types.UserActive, Version: 7, CreatedAt: fixtures.ServiceTime, MustChangePassword: true}
	if command == "create" {
		detail.Version = 1
		detail.Status = status
	}
	current := domain.SessionUserState{ID: change.UserID, Status: types.UserActive, Version: 7, AuthVersion: 3}
	if kind == "activate" {
		current.Status = types.UserSuspended
	}
	if kind == "stale version" {
		detail.Version = 8
		current.Version = 8
	}
	fingerprinter, err := featurecommon.NewFingerprinter("v1", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.IdempotencyScope{ActorID: 42, Method: method, Target: "user/43", Key: "test-key"}
	payload := featurecommon.FingerprintPayload{ExpectedVersion: 7, Name: &name, Presence: []string{"expected_user_version", "name"}}
	if command == "create" {
		scope.Target = "account/target@example.test"
		email := "target@example.test"
		address, phone := "", ""
		payload = featurecommon.FingerprintPayload{Email: email, Name: &create.Name, Address: &address, Phone: &phone, Status: string(status), Presence: []string{"status", "name", "address", "phone"}}
	}
	if command == "status" {
		if kind == "self" {
			scope.Target = "user/42"
		}
		payload = featurecommon.FingerprintPayload{ExpectedVersion: 7, Status: string(status), Reason: change.Reason, Presence: []string{"status", "expected_user_version"}}
	}
	var hash []byte
	if command == "create" {
		hash, err = fingerprinter.PasswordFingerprint(scope, payload, []byte(create.Password))
	} else {
		hash, err = featurecommon.Fingerprint(scope, payload)
	}
	if err != nil {
		t.Fatal(err)
	}
	var record *domain.IdempotencyRecord
	if kind == "replay" || kind == "expired" || kind == "conflict" {
		saved := domain.UserDetailCommandResult(detail, true)
		if command != "create" {
			saved.Version = 8
		}
		if command == "status" {
			saved.Status = string(status)
		}
		data, _ := json.Marshal(saved)
		record = &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: string(data), ExpiresAt: fixtures.ServiceTime.Add(time.Hour)}
		if kind == "expired" {
			record.ExpiresAt = fixtures.ServiceTime
		}
		if kind == "conflict" {
			record.RequestHash = bytes.Clone(hash)
			record.RequestHash[len(record.RequestHash)-1] = '0'
			if hash[len(hash)-1] == '0' {
				record.RequestHash[len(record.RequestHash)-1] = '1'
			}
		}
	}
	permissionsCalls, idsCalls, roleAppCalls := 0, 0, 0
	d := user.ManagementDependencies{Transactions: provider, Fingerprinter: fingerprinter, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }, HashPassword: func(password string) (string, error) {
		if password != create.Password {
			t.Fatal("password mapping")
		}
		return "synthetic-argon-hash", step("hash")
	}}
	d.Bind = func(handle domain.Transaction) user.ManagementRepositories {
		if handle != tx {
			t.Fatal("tx identity")
		}
		tx.Record("bind")
		return user.ManagementRepositories{
			State: authmocks.SessionStateReader{Read: func(got context.Context, sid string) (domain.SessionState, error) {
				if got != ctx || sid != fixtures.SessionID {
					t.Fatal("state context")
				}
				fresh := state
				if kind == "stale actor" {
					fresh.AuthVersion++
				}
				if kind == "restricted" {
					fresh.MustChangePassword = true
				}
				return fresh, step("state")
			}},
			Permissions: sessionmocks.Permissions{Read: func(got context.Context, id uint64) ([]domain.PermissionContribution, error) {
				if got != ctx || id != 42 {
					t.Fatal("permission scope")
				}
				permissionsCalls++
				e := step("permissions" + string(rune('0'+permissionsCalls)))
				rows := []domain.PermissionContribution{{PermissionCode: string(permission), PermissionApplicationID: 1, RoleID: 2, RoleCode: "iam.editor", RoleApplicationID: 1, RoleStatus: types.RoleActive, AssignmentStatus: types.AssignmentActive, ApplicationCode: types.ApplicationIAM, ApplicationStatus: types.ApplicationActive}}
				if kind == "denied" {
					rows[0].PermissionCode = "iam.other"
				}
				return rows, e
			}},
			Targets: sessionmocks.Targets{IDs: func(got context.Context, id uint64) ([]string, error) {
				if got != ctx || id != change.UserID && command != "update" {
					t.Fatal("session scope")
				}
				idsCalls++
				return nil, step("ids" + string(rune('0'+idsCalls)))
			}},
			Policy: mocks.Policy{Role: func(got context.Context, id uint64) (uint64, error) {
				if got != ctx || id != 1 {
					t.Fatal("member role scope")
				}
				roleAppCalls++
				return 1, step("role-app" + string(rune('0'+roleAppCalls)))
			}, Count: func(got context.Context, id uint64) (int64, bool, error) {
				if got != ctx || id != 43 {
					t.Fatal("admin count scope")
				}
				count := int64(2)
				if kind == "last admin" {
					count = 1
				}
				if kind == "count failure" {
					return 0, false, cause
				}
				return count, true, step("admin-count")
			}},
			Account: authmocks.AuthReader{Get: func(email string) (domain.Auth, error) {
				if email != "target@example.test" {
					t.Fatal("account normalization")
				}
				if e := step("account"); e != nil {
					return domain.Auth{}, e
				}
				if kind == "duplicate" {
					return domain.Auth{}, nil
				}
				return domain.Auth{}, auth.ErrNotFound
			}},
			Role: authmocks.RegistrationRole{Read: func() (uint64, error) { return 1, step("member") }},
			Create: authmocks.RegistrationWriter{Write: func(r *domain.PendingRegistration) error {
				if r.ID != 0 || r.Status != status || !r.MustChangePassword || r.Version != 1 || r.AuthVersion != 1 || r.Password != "synthetic-argon-hash" || r.Email != "target@example.test" {
					t.Fatal("creation invariants")
				}
				r.ID = 43
				return step("create")
			}},
			Profile: authmocks.ProfileWriter{Write: func(r *domain.ProfileRequest) error {
				if r.UserId != 43 || r.Name != create.Name {
					t.Fatal("profile scope")
				}
				return step("profile-create")
			}},
			Member: authmocks.RegistrationMember{Write: func(id, role uint64) error {
				if id != 43 || role != 1 {
					t.Fatal("default role scope")
				}
				return step("member-assign")
			}},
			Detail: mocks.Detail{Read: func(got context.Context, id uint64) (domain.UserDetail, error) {
				if got != ctx || id != 43 {
					t.Fatal("target detail scope")
				}
				return detail, step("detail")
			}},
			Users: sessionmocks.Users{Read: func(got context.Context, id uint64) (domain.SessionUserState, error) {
				if got != ctx || id != change.UserID {
					t.Fatal("target scope")
				}
				return current, step("target")
			}},
			ProfileMutation: mocks.ProfileMutation{Version: func(got context.Context, id uint64, v int64, at time.Time) (bool, error) {
				if got != ctx || id != 43 || v != 7 || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("version mutation scope")
				}
				return kind != "zero version write", step("profile-version")
			}, Write: func(got context.Context, id uint64, r domain.UserDetail) (bool, error) {
				if got != ctx || id != 43 || r.Name != name {
					t.Fatal("profile mutation scope")
				}
				return kind != "zero profile write", step("profile-write")
			}},
			StatusMutation: mocks.StatusMutation{Write: func(got context.Context, b domain.SessionUserState, to types.UserStatus, invalidate bool, at time.Time) (bool, error) {
				if got != ctx || b.ID != 43 || to != status || invalidate != (kind != "activate") || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("status mutation scope")
				}
				return kind != "zero status write", step("status-write")
			}},
			Sessions: authmocks.SessionRevocation{All: func(got context.Context, r domain.SessionRevocation) (int64, error) {
				if got != ctx || r.UserID != 43 || r.ActorID != 42 || r.Validate() != nil {
					t.Fatal("revocation scope")
				}
				return 0, step("sessions")
			}},
			Refresh: authmocks.RefreshRevocation{All: func(got context.Context, id uint64, at time.Time) (int64, error) {
				if got != ctx || id != 43 || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("refresh scope")
				}
				return 0, step("refresh")
			}},
			Audit: authmocks.AuditWriter{Write: func(r domain.AuditRequest) error {
				a, e := r.Record()
				if e != nil || a.ActorUserID == nil || *a.ActorUserID != 42 || r.TargetID != "43" {
					t.Fatal("audit identity", e)
				}
				if kind == "no-op" && (r.Action != types.AuditNoOp || !r.After.NoOp) {
					t.Fatal("no-op audit")
				}
				return step("audit")
			}},
			Outbox: shared.OutboxWriter{Write: func(r domain.EventEnvelope) error {
				if _, e := r.Record(); e != nil || r.ActorUserID == nil || *r.ActorUserID != 42 || r.Data.UserID != 43 {
					t.Fatal("event metadata", e)
				}
				if command == "status" && (r.EventType != types.EventUserStatusChanged || r.Data.SessionsRevoked == nil || *r.Data.SessionsRevoked != (kind != "activate")) {
					t.Fatal("single status invalidation event")
				}
				return step("outbox")
			}},
			Idempotency: sessionmocks.Idempotency{Read: func(got domain.IdempotencyScope) (*domain.IdempotencyRecord, error) {
				if got != scope {
					t.Fatal("idempotency scope", got, scope)
				}
				return record, step("lookup")
			}, Write: func(got domain.IdempotencyScope, h []byte, r domain.SafeCommandResult, at time.Time) error {
				if got != scope || !bytes.Equal(h, hash) || r.Validate() != nil || r.UserID != 43 || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("safe saved result")
				}
				if command != "status" {
					if _, e := r.UserDetail(); e != nil {
						t.Fatal("incomplete saved detail", e)
					}
				}
				return step("save")
			}},
		}
	}
	svc, err := user.NewManagementService(d)
	if err != nil {
		t.Fatal(err)
	}
	var result *domain.SafeCommandResult
	switch command {
	case "create":
		result, err = svc.CreateUser(ctx, create)
	case "update":
		result, err = svc.UpdateUser(ctx, update)
	case "status":
		result, err = svc.ChangeUserStatus(ctx, change)
	}
	if strings.Contains(kind, ":") || slices.Contains([]string{"stale actor", "restricted", "denied", "busy", "begin failure", "lock failure", "commit failure", "conflict", "duplicate", "stale version", "last admin", "count failure", "zero version write", "zero profile write", "zero status write", "illegal transition", "self"}, kind) {
		if err == nil || result != nil {
			t.Fatal("failed command succeeded", result, err)
		}
		if kind == "begin failure" {
			if !reflect.DeepEqual(tx.Calls, []string{"begin"}) || !errors.Is(err, cause) {
				t.Fatal("failed begin acquired resources or lost error", err, tx.Calls)
			}
			return
		}
		if tx.Calls[len(tx.Calls)-1] != "rollback" {
			t.Fatal("failed command did not roll back", tx.Calls)
		}
		if strings.HasPrefix(kind, "error:") || kind == "lock failure" || kind == "commit failure" || kind == "count failure" {
			if !errors.Is(err, cause) {
				t.Fatal("cause lost", err)
			}
		}
		if strings.HasPrefix(kind, "cancel:") && !errors.Is(err, context.Canceled) {
			t.Fatal("cancel lost", err)
		}
		if strings.HasPrefix(kind, "panic:") && !errors.Is(err, common.ErrInternalServerError) {
			t.Fatal("panic not mapped", err)
		}
		want := map[string]error{"last admin": usererrors.ErrLastAdminRequired, "stale version": usererrors.ErrVersionConflict, "zero version write": usererrors.ErrVersionConflict, "zero status write": usererrors.ErrVersionConflict, "zero profile write": auth.ErrNotFound, "conflict": usererrors.ErrIdempotencyConflict, "duplicate": auth.ErrDataExists, "illegal transition": usererrors.ErrInvalidStatusTransition, "self": usererrors.ErrSelfInactivationForbidden, "busy": usererrors.ErrResourceBusy, "denied": common.ErrForbidden}
		if want[kind] != nil && !errors.Is(err, want[kind]) {
			t.Fatal("business error identity", err)
		}
		return
	}
	if err != nil || result == nil || result.UserID != 43 || result.Changed != (kind != "no-op") {
		t.Fatal("result", result, err, tx.Calls)
	}
	if tx.Calls[len(tx.Calls)-1] != "commit" {
		t.Fatal("success not committed")
	}
	if kind == "replay" {
		if !reflect.DeepEqual(tx.Calls[len(tx.Calls)-2:], []string{"lookup", "commit"}) || slices.Contains(tx.Calls, "audit") || slices.Contains(tx.Calls, "hash") || slices.Contains(tx.Calls, "target") || slices.Contains(tx.Calls, "detail") {
			t.Fatal("replay performed business effects", tx.Calls)
		}
	}
	if kind == "no-op" && (slices.Contains(tx.Calls, "outbox") || slices.Contains(tx.Calls, "status-write") || slices.Contains(tx.Calls, "profile-write")) {
		t.Fatal("no-op mutated state", tx.Calls)
	}
	if kind == "optional key" && (slices.Contains(tx.Calls, "lookup") || slices.Contains(tx.Calls, "save")) {
		t.Fatal("optional key triggered cache")
	}
	locks := []string{}
	for _, call := range tx.Calls {
		if strings.HasPrefix(call, "lock:") {
			locks = append(locks, strings.TrimPrefix(call, "lock:"))
		}
	}
	if !reflect.DeepEqual(locks, featurecommon.CanonicalLockKeys(locks...)) {
		t.Fatal("noncanonical or duplicated locks", locks)
	}
}
