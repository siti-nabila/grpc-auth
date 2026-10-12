package test_scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	readmocks "github.com/siti-nabila/grpc-auth/tests/features/catalog/mocks"
	"github.com/siti-nabila/grpc-auth/tests/features/role/mocks"
	sessionmocks "github.com/siti-nabila/grpc-auth/tests/features/session/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
)

func Management() []testutils.Scenario {
	return []testutils.Scenario{{Name: "four role commands recheck authority replay version and every required atomic write", Run: roleManagementMatrix}}
}

func roleManagementMatrix(t *testing.T) {
	for _, command := range []string{"create", "update", "status", "permissions"} {
		cases := []string{"success", "replay", "conflict", "expired", "denied", "lost permission", "stale actor", "restricted", "busy", "begin failure", "lock failure", "commit failure", "system app"}
		stages := []string{"permissions1", "permissions2", "state", "lookup", "audit", "event-id", "outbox", "save"}
		if command == "create" {
			cases = append(cases, "inactive app", "duplicate", "empty set", "unknown set")
			stages = append(stages, "app1", "app2", "app3", "permission-set", "create", "mapping")
		} else {
			cases = append(cases, "stale version", "stale no-op", "no-op", "system role", "protected no-op", "IAM legacy", "missing role", "disabled app", "zero update", "overflow")
			stages = append(stages, "permissions3", "permissions4", "target1", "target2", "target3", "target4", "app1", "update")
		}
		if command == "update" {
			cases = append(cases, "optional key", "clear description", "description only")
		}
		if command == "permissions" {
			cases = append(cases, "empty set", "unknown set", "cross app")
			stages = append(stages, "permission-set", "role-permissions", "mapping")
		}
		for _, stage := range stages {
			for _, failure := range []string{"error", "panic", "cancel"} {
				cases = append(cases, failure+":"+stage)
			}
		}
		for _, kind := range cases {
			t.Run(command+"/"+kind, func(t *testing.T) { roleManagementCase(t, command, kind) })
		}
	}
}

func roleManagementCase(t *testing.T, command, kind string) {
	t.Helper()
	method := rolev1.RoleService_CreateRole_FullMethodName
	permission := types.PermissionIAMRoleCreate
	switch command {
	case "update":
		method = rolev1.RoleService_UpdateRole_FullMethodName
		permission = types.PermissionIAMRoleUpdate
	case "status":
		method = rolev1.RoleService_ChangeRoleStatus_FullMethodName
		permission = types.PermissionIAMRoleUpdate
	case "permissions":
		method = rolev1.RoleService_ReplaceRolePermissions_FullMethodName
		permission = types.PermissionIAMRolePermissionUpdate
	}
	ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, context.Background(), method, method))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cause := errors.New("controlled role command failure")
	calls := []string{}
	counts := map[string]int{}
	transactions := []*shared.Transaction{}
	step := func(stage string) error {
		calls = append(calls, stage)
		switch kind {
		case "error:" + stage:
			return cause
		case "panic:" + stage:
			panic("controlled role panic")
		case "cancel:" + stage:
			cancel()
			return context.Canceled
		}
		return nil
	}
	numbered := func(stage string) error { counts[stage]++; return step(stage + strconv.Itoa(counts[stage])) }
	checkContext := func(got context.Context) {
		if got != ctx {
			t.Fatal("request context changed")
		}
	}
	provider := mocks.Provider{BeginTransaction: func(got context.Context) (domain.Transaction, error) {
		checkContext(got)
		calls = append(calls, "begin")
		if kind == "begin failure" {
			return nil, cause
		}
		tx := &shared.Transaction{Context: ctx}
		tx.Hook = func(call string) { calls = append(calls, call) }
		tx.Busy = kind == "busy"
		if kind == "lock failure" {
			tx.LockError = cause
		}
		if kind == "commit failure" {
			tx.CommitError = cause
		}
		transactions = append(transactions, tx)
		return tx, nil
	}}
	oldDescription := "Old"
	name := "Edited"
	empty := ""
	status := types.RoleDisabled
	current := domain.RoleSummary{ID: 7, Code: "sales.editor", Name: "Editor", Description: &oldDescription, ApplicationID: 2, ApplicationCode: "sales", Status: types.RoleActive, Version: 7, NumericCode: 81, AffectedUserCount: 3}
	app := domain.ApplicationItem{ID: 2, Code: "sales", Name: "Sales", Status: types.ApplicationActive}
	if kind == "system app" {
		app.IsSystem = true
	}
	if kind == "inactive app" || kind == "disabled app" {
		app.Status = types.ApplicationDisabled
	}
	if kind == "system role" || kind == "protected no-op" {
		current.IsSystem = true
	}
	if kind == "IAM legacy" {
		current.Code = "iam.legacy"
		current.ApplicationCode = "iam"
		current.ApplicationID = 1
	}
	if kind == "stale version" || kind == "stale no-op" || kind == "replay" {
		current.Version = 8
	}
	if kind == "overflow" {
		current.Version = 9223372036854775807
	}
	if kind == "no-op" || kind == "protected no-op" || kind == "stale no-op" {
		name = current.Name
		status = types.RoleActive
	}
	create := domain.CreateRoleInput{ApplicationCode: "sales", Code: "sales.editor", Name: "New", PermissionCodes: []string{"sales.order.read", "sales.order.read"}, Key: "key"}
	update := domain.UpdateRoleInput{RoleID: 7, ExpectedVersion: 7, Name: &name, Fields: []string{"name"}, Key: "key"}
	change := domain.ChangeRoleStatusInput{RoleID: 7, ExpectedVersion: 7, Status: &status, Reason: "Policy review", Key: "key"}
	replace := domain.ReplaceRolePermissionsInput{RoleID: 7, ExpectedVersion: 7, Permissions: &domain.PermissionSet{Codes: []string{"sales.order.read", "sales.order.read"}}, Reason: "Policy review", Key: "key"}
	if kind == "overflow" {
		update.ExpectedVersion = current.Version
		change.ExpectedVersion = current.Version
		replace.ExpectedVersion = current.Version
	}
	if kind == "optional key" {
		update.Key = ""
	}
	if kind == "clear description" {
		update.Description = &empty
		update.Fields = append(update.Fields, "description")
	}
	if kind == "description only" {
		update.Name = nil
		update.Description = &empty
		update.Fields = []string{"description"}
	}
	newPermissions := []domain.PermissionItem{{ID: 9, ApplicationID: 2, ApplicationCode: "sales", Code: "sales.order.read", Description: "Read"}}
	oldPermissions := []domain.PermissionItem{{ID: 10, ApplicationID: 2, ApplicationCode: "sales", Code: "sales.order.update", Description: "Update"}}
	if kind == "empty set" {
		create.PermissionCodes = nil
		replace.Permissions.Codes = nil
		newPermissions = []domain.PermissionItem{}
	}
	if kind == "no-op" || kind == "protected no-op" || kind == "stale no-op" {
		oldPermissions = append([]domain.PermissionItem(nil), newPermissions...)
	}
	if kind == "cross app" {
		replace.Permissions.Codes = []string{"other.order.read"}
	}
	scope := domain.IdempotencyScope{ActorID: 42, Method: method, Target: "role/7", Key: "key"}
	payload := featurecommon.FingerprintPayload{Name: update.Name, Description: update.Description, ExpectedVersion: update.ExpectedVersion, Presence: append([]string{"expected_role_version"}, update.Fields...)}
	switch command {
	case "create":
		scope.Target = "role-code/sales.editor"
		payload = featurecommon.FingerprintPayload{Code: create.Code, Name: &create.Name, PermissionCodes: create.PermissionCodes, Presence: []string{"name", "permission_codes"}}
	case "status":
		payload = featurecommon.FingerprintPayload{Status: string(status), Reason: change.Reason, ExpectedVersion: change.ExpectedVersion, Presence: []string{"status", "expected_role_version"}}
	case "permissions":
		payload = featurecommon.FingerprintPayload{PermissionCodes: replace.Permissions.Codes, Reason: replace.Reason, ExpectedVersion: replace.ExpectedVersion, Presence: []string{"permissions", "expected_role_version"}}
	}
	hash, err := featurecommon.Fingerprint(scope, payload)
	if err != nil {
		t.Fatal(err)
	}
	var record *domain.IdempotencyRecord
	savedRole := current
	savedRole.Version = 8
	savedRole.AffectedUserCount = 99
	savedRole.Name = "Saved original"
	if command == "create" {
		savedRole.Version = 1
	}
	saved := domain.RoleCommandResult(domain.RoleDetail{Role: savedRole, Permissions: newPermissions}, true)
	if kind == "replay" || kind == "expired" || kind == "conflict" {
		data, _ := json.Marshal(saved)
		record = &domain.IdempotencyRecord{RequestHash: bytes.Clone(hash), SafeResponse: string(data), ExpiresAt: fixtures.ServiceTime.Add(time.Hour)}
		if kind == "expired" {
			record.ExpiresAt = fixtures.ServiceTime
		}
		if kind == "conflict" {
			record.RequestHash = []byte("different")
		}
	}
	d := role.ManagementDependencies{Transactions: provider, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }}
	d.Bind = func(tx domain.Transaction) role.ManagementRepositories {
		if len(transactions) == 0 || tx != transactions[len(transactions)-1] {
			t.Fatal("transaction binding changed")
		}
		calls = append(calls, "bind")
		return role.ManagementRepositories{
			State: shared.StateReader{Read: func(got context.Context, sid string) (domain.SessionState, error) {
				checkContext(got)
				if sid != fixtures.SessionID {
					t.Fatal("session scope")
				}
				state := fixtures.SessionState()
				if kind == "stale actor" {
					state.AuthVersion++
				}
				if kind == "restricted" {
					state.MustChangePassword = true
				}
				return state, step("state")
			}},
			Permissions: sessionmocks.Permissions{Read: func(got context.Context, id uint64) ([]domain.PermissionContribution, error) {
				checkContext(got)
				if id != 42 {
					t.Fatal("actor target mixed")
				}
				rows := readmocks.Contribution(string(permission))
				if kind == "denied" || kind == "lost permission" && counts["permissions"] >= 2 {
					rows = readmocks.Contribution("iam.other.read")
				}
				return rows, numbered("permissions")
			}},
			Roles: mocks.Roles{
				Read: func(got context.Context, id uint64) (domain.RoleSummary, error) {
					checkContext(got)
					if id != 7 {
						t.Fatal("role target")
					}
					err := numbered("target")
					if err == nil && kind == "missing role" {
						err = roleerrors.ErrRoleNotFound
					}
					return current, err
				},
				App: func(got context.Context, code string) (domain.ApplicationItem, error) {
					checkContext(got)
					if code != "sales" {
						t.Fatal("application target")
					}
					return app, numbered("app")
				},
				Set: func(got context.Context, id uint64, codes []string) ([]domain.PermissionItem, error) {
					checkContext(got)
					if id != 2 || !slices.Equal(codes, domain.NormalizePermissionCodes(create.PermissionCodes)) && command == "create" || !slices.Equal(codes, domain.NormalizePermissionCodes(replace.Permissions.Codes)) && command == "permissions" {
						t.Fatal("permission set scope/canonicalization")
					}
					err := step("permission-set")
					if err == nil && kind == "unknown set" {
						err = roleerrors.ErrInvalidPermissionSet
					}
					return newPermissions, err
				},
				Permissions: func(got context.Context, id uint64) ([]domain.PermissionItem, error) {
					checkContext(got)
					if id != 7 {
						t.Fatal("role permission target")
					}
					return oldPermissions, step("role-permissions")
				},
			},
			Writer: mocks.Writer{
				Create: func(got context.Context, r domain.RoleSummary) (domain.RoleSummary, error) {
					checkContext(got)
					if r.ID != 0 || r.NumericCode != 0 || r.Version != 1 || r.Status != types.RoleActive || r.IsSystem || r.ApplicationID != 2 || r.Code != "sales.editor" || r.Name != "New" {
						t.Fatal("create invariants")
					}
					r.ID = 7
					r.NumericCode = 81
					err := step("create")
					if err == nil && kind == "duplicate" {
						err = roleerrors.ErrRoleCodeAlreadyExists
					}
					return r, err
				},
				Update: func(got context.Context, r domain.RoleSummary, expected int64) (bool, error) {
					checkContext(got)
					if expected != 7 || r.ID != 7 || r.Version != 8 || r.Code != current.Code || r.ApplicationID != current.ApplicationID || r.NumericCode != current.NumericCode || r.AffectedUserCount != 3 || r.UpdatedAt == nil || !r.UpdatedAt.Equal(fixtures.ServiceTime) {
						t.Fatal("conditional update invariants")
					}
					if command == "update" && update.Description != nil && r.Description != nil {
						t.Fatal("description not SQL NULL")
					}
					return kind != "zero update", step("update")
				},
				Replace: func(got context.Context, id uint64, ids []uint64) error {
					checkContext(got)
					expected := []uint64{}
					for _, p := range newPermissions {
						expected = append(expected, p.ID)
					}
					if id != 7 || !slices.Equal(ids, expected) {
						t.Fatal("final mapping set")
					}
					return step("mapping")
				},
			},
			Audit: shared.AuditWriter{Write: func(r domain.AuditRequest) error {
				record, err := r.Record()
				if err != nil || r.TargetID != "7" || r.TargetType != types.AuditTargetRole || record.ActorUserID == nil || *record.ActorUserID != 42 {
					t.Fatal("audit target/actor", err)
				}
				if kind == "no-op" && (r.Action != types.AuditNoOp || !r.After.NoOp) {
					t.Fatal("no-op audit")
				}
				return step("audit")
			}},
			Outbox: shared.OutboxWriter{Write: func(e domain.EventEnvelope) error {
				if _, err := e.Record(); err != nil || e.AggregateType != types.AggregateRole || e.AggregateID != "7" || e.Data.RoleID != 7 || e.Data.UserID != 0 || e.ActorUserID == nil || *e.ActorUserID != 42 || e.Data.ApplicationID != 2 {
					t.Fatal("role event scope", err)
				}
				return step("outbox")
			}},
			Idempotency: sessionmocks.Idempotency{Read: func(got domain.IdempotencyScope) (*domain.IdempotencyRecord, error) {
				if got != scope {
					t.Fatal("replay scope", got, scope)
				}
				return record, step("lookup")
			}, Write: func(got domain.IdempotencyScope, h []byte, r domain.SafeCommandResult, at time.Time) error {
				if got != scope || !bytes.Equal(h, hash) || r.Validate() != nil || r.RoleID != 7 || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("safe save")
				}
				return step("save")
			}},
		}
	}
	svc, err := role.NewManagementService(d)
	if err != nil {
		t.Fatal(err)
	}
	auditCalls, auditFailures := 0, 0
	boundary, err := interceptors.NewSecurityAttemptInterceptor(func(auditCtx context.Context, request domain.AuditRequest) error {
		auditCalls++
		if kind != "begin failure" && (len(calls) == 0 || calls[len(calls)-1] != "rollback") {
			t.Fatal("independent audit preceded rollback", calls)
		}
		if auditCtx.Err() != nil || request.TargetID != method || request.Action != types.AuditSecurityAttemptFailed {
			t.Fatal("unsafe or canceled independent audit")
		}
		if _, ok := auditCtx.Deadline(); !ok {
			t.Fatal("audit lacks bounded deadline")
		}
		calls = append(calls, "failure-audit")
		return cause
	}, func() { auditFailures++ }, func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	value, err := boundary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) {
		switch command {
		case "create":
			return svc.CreateRole(ctx, create)
		case "update":
			return svc.UpdateRole(ctx, update)
		case "status":
			return svc.ChangeRoleStatus(ctx, change)
		default:
			return svc.ReplaceRolePermissions(ctx, replace)
		}
	})
	var result *domain.SafeCommandResult
	if value != nil {
		result, _ = value.(*domain.SafeCommandResult)
	}

	want := map[string]error{"denied": common.ErrForbidden, "stale actor": auth.ErrInvalidSession, "restricted": common.ErrForbidden, "busy": roleerrors.ErrResourceBusy, "begin failure": cause, "lock failure": cause, "commit failure": cause, "conflict": roleerrors.ErrIdempotencyConflict, "inactive app": applicationerrors.ErrApplicationInactive, "system app": applicationerrors.ErrSystemApplicationProtected, "duplicate": roleerrors.ErrRoleCodeAlreadyExists, "unknown set": roleerrors.ErrInvalidPermissionSet, "cross app": roleerrors.ErrInvalidPermissionSet, "system role": roleerrors.ErrSystemRoleProtected, "IAM legacy": roleerrors.ErrSystemRoleProtected, "missing role": roleerrors.ErrRoleNotFound, "stale version": roleerrors.ErrVersionConflict, "zero update": roleerrors.ErrVersionConflict, "overflow": common.ErrInternalServerError}[kind]
	if kind == "lost permission" {
		want = common.ErrForbidden
	}
	if kind == "stale no-op" {
		want = roleerrors.ErrVersionConflict
	}
	if kind == "protected no-op" {
		want = roleerrors.ErrSystemRoleProtected
	}
	if strings.HasPrefix(kind, "error:") {
		want = cause
	}
	if strings.HasPrefix(kind, "cancel:") {
		want = context.Canceled
	}
	if strings.HasPrefix(kind, "panic:") {
		want = common.ErrInternalServerError
	}
	if !errors.Is(err, want) {
		t.Fatalf("err %v want %v calls %v", err, want, calls)
	}
	if want != nil {
		if result != nil {
			t.Fatal("partial success escaped")
		}
		if auditCalls != 1 || auditFailures != 1 || calls[len(calls)-1] != "failure-audit" {
			t.Fatal("independent audit count or telemetry", calls)
		}
		if kind != "begin failure" && calls[len(calls)-2] != "rollback" {
			t.Fatal("failure cleanup", calls)
		}
		if kind != "commit failure" && slices.Contains(calls, "commit") {
			t.Fatal("failed precommit committed")
		}
		return
	}
	if auditCalls != 0 || auditFailures != 0 {
		t.Fatal("success audited as failure")
	}
	if result == nil || result.RoleID != 7 || result.Changed != (kind != "no-op") || calls[len(calls)-1] != "commit" {
		t.Fatalf("success result %+v %v", result, calls)
	}
	if kind == "replay" {
		if result.Role.AffectedUserCount != 99 || result.Role.Name != "Saved original" || slices.Contains(calls, "audit") || slices.Contains(calls, "update") || slices.Contains(calls, "create") || slices.Contains(calls, "save") {
			t.Fatal("replay changed original response/effects", calls)
		}
	}
	if kind == "no-op" && (slices.Contains(calls, "outbox") || slices.Contains(calls, "mapping") || slices.Contains(calls, "update") || result.Version != 7) {
		t.Fatal("no-op writes", calls)
	}
	if kind == "optional key" && (slices.Contains(calls, "lookup") || slices.Contains(calls, "save")) {
		t.Fatal("unkeyed replay")
	}
	for _, tx := range transactions {
		locks := []string{}
		for _, call := range tx.Calls {
			if strings.HasPrefix(call, "lock:") {
				locks = append(locks, strings.TrimPrefix(call, "lock:"))
			}
		}
		if !reflect.DeepEqual(locks, featurecommon.CanonicalLockKeys(locks...)) {
			t.Fatal("noncanonical locks", locks)
		}
	}
	if command != "create" && len(transactions) != 2 {
		t.Fatal("predecessor discovery did not restart onto a new transaction", len(transactions))
	}
}
