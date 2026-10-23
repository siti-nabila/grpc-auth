package test_scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
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
	"github.com/siti-nabila/orm/orm"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func Assignments() []testutils.Scenario {
	return []testutils.Scenario{{Name: "grant revoke and snapshot strict transaction failure and policy matrix", Run: assignmentMatrix}, {Name: "missing predecessor restarts and bounded instability", Run: assignmentRestarts}}
}

func assignmentMatrix(t *testing.T) {
	for _, command := range []string{"grant", "revoke", "list"} {
		cases := []string{"success", "denied", "stale actor", "restricted", "begin failure", "commit failure", "target absent", "target deleted"}
		stages := []string{"state", "permissions3", "target"}
		if command == "list" {
			stages = []string{"state", "permissions1", "target", "list"}
			cases = append(cases, "empty", "SUSPENDED", "PENDING", "DEACTIVATED")
		} else {
			stages = append(stages, "permissions1", "permissions2", "roles1", "roles2", "roles3", "lookup", "version", "audit", "event-id", "outbox", "save", "write1")
			cases = append(cases, "busy", "lock failure", "replay", "conflict", "expired", "stale version", "no-op", "unknown role", "zero write", "zero version", "SUSPENDED", "PENDING", "DEACTIVATED")
			if command == "grant" {
				cases = append(cases, "disabled role", "disabled app", "missing access", "revoked access", "write2 failure")
				stages = append(stages, "write2")
			} else {
				cases = append(cases, "self admin", "self admin no-op", "last admin", "count failure", "disabled role", "disabled app", "missing access")
				stages = append(stages, "admin-count")
			}
		}
		for _, stage := range stages {
			for _, failure := range []string{"error", "panic", "cancel"} {
				cases = append(cases, failure+":"+stage)
			}
		}
		for _, kind := range cases {
			t.Run(command+"/"+kind, func(t *testing.T) { assignmentCase(t, command, kind, false) })
		}
	}
}

func assignmentCase(t *testing.T, command, kind string, restart bool) {
	t.Helper()
	method := userv1.UserService_GrantUserRoles_FullMethodName
	permission := types.PermissionIAMUserRoleGrant
	if command == "revoke" {
		method = userv1.UserService_RevokeUserRoles_FullMethodName
		permission = types.PermissionIAMUserRoleRevoke
	}
	if command == "list" {
		method = userv1.UserService_ListUserRoles_FullMethodName
		permission = types.PermissionIAMUserRead
	}
	ctx := fixtures.CallerContext(t, context.Background(), method, method)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	state := fixtures.SessionState()
	p, e := sessions.PrincipalForState(fixtures.AccessClaims(t), state, fixtures.ServiceTime)
	if e != nil {
		t.Fatal(e)
	}
	ctx = p.Context(ctx)
	tx := &shared.Transaction{Context: ctx}
	provider := &mocks.AssignmentProvider{Tx: tx}
	cause := errors.New("controlled private assignment failure")
	if kind == "begin failure" {
		provider.Error = cause
	}
	if kind == "commit failure" {
		tx.CommitError = cause
	}
	if kind == "busy" {
		tx.Busy = true
	}
	if kind == "lock failure" {
		tx.LockError = cause
	}
	step := func(stage string) error {
		tx.Record(stage)
		switch kind {
		case "error:" + stage:
			return cause
		case "panic:" + stage:
			panic("private")
		case "cancel:" + stage:
			cancel()
			return context.Canceled
		}
		return nil
	}
	input := domain.UserRoleAssignmentInput{UserID: 43, RoleIDs: []uint64{3, 1, 2, 1}, ExpectedVersion: 7, Reason: " Review access ", Key: " exact key "}
	if restart {
		input.RoleIDs = []uint64{6, 4, 5, 4}
	}
	if strings.HasPrefix(kind, "self admin") {
		input.UserID = 42
	}
	before := domain.SessionUserState{ID: input.UserID, Status: types.UserActive, Version: 7, AuthVersion: 9}
	if slices.Contains([]string{"SUSPENDED", "PENDING", "DEACTIVATED"}, kind) {
		before.Status = types.UserStatus(kind)
	}
	if kind == "stale version" || kind == "replay" {
		before.Version = 8
	}
	if kind == "target deleted" {
		before.DeletedAt.Valid = true
	}
	active := "ACTIVE"
	rows := []domain.UserRoleAssignmentItem{{RoleID: 1, Code: "iam.member", ApplicationID: 1, ApplicationCode: "iam", RoleStatus: "ACTIVE", ApplicationStatus: "ACTIVE", IsSystem: true}, {ID: 12, RoleID: 2, Code: "sales.editor", ApplicationID: 2, ApplicationCode: "sales", ApplicationAccessStatus: &active, RoleStatus: "ACTIVE", ApplicationStatus: "ACTIVE", Status: "REVOKED"}, {ID: 13, RoleID: 3, Code: "sales.reader", ApplicationID: 2, ApplicationCode: "sales", ApplicationAccessStatus: &active, RoleStatus: "ACTIVE", ApplicationStatus: "ACTIVE", Status: "ACTIVE"}}
	changed := []uint64{1, 2}
	if command == "revoke" {
		rows[0].ID = 11
		rows[0].Status = "ACTIVE"
		rows[2].ID = 0
		rows[2].Status = ""
		changed = []uint64{1}
	}
	if kind == "no-op" {
		for i := range rows {
			if command == "grant" {
				rows[i].ID = uint64(11 + i)
				rows[i].Status = "ACTIVE"
			} else {
				rows[i].Status = "REVOKED"
				if rows[i].ID == 0 {
					rows[i].Status = ""
				}
			}
		}
		changed = nil
	}
	if kind == "unknown role" {
		rows = rows[:2]
	}
	if kind == "disabled role" {
		rows[1].RoleStatus = "DISABLED"
	}
	if kind == "disabled app" {
		rows[1].ApplicationStatus = "DISABLED"
	}
	if kind == "missing access" {
		rows[1].ApplicationAccessStatus = nil
	}
	if kind == "revoked access" {
		revoked := "REVOKED"
		rows[1].ApplicationAccessStatus = &revoked
	}
	if strings.Contains(kind, "admin") || strings.HasSuffix(kind, ":admin-count") || kind == "count failure" {
		rows[0].Code = string(types.RoleIAMPlatformAdmin)
	}
	if kind == "self admin no-op" {
		rows[0].ID = 0
		rows[0].Status = ""
	}
	if restart {
		for i := range rows {
			rows[i].RoleID += 3
		}
		changed = []uint64{4, 5}
	}
	scope := domain.IdempotencyScope{ActorID: 42, Method: method, Target: fmt.Sprintf("user/%d", input.UserID), Key: input.Key}
	norm := input.Normalized()
	hash, _ := featurecommon.Fingerprint(scope, featurecommon.FingerprintPayload{RoleIDs: norm.RoleIDs, Reason: norm.Reason, ExpectedVersion: 7, Presence: []string{"expected_user_version"}})
	var record *domain.IdempotencyRecord
	if slices.Contains([]string{"replay", "conflict", "expired"}, kind) {
		data, _ := json.Marshal(domain.SafeCommandResult{UserID: input.UserID, Version: 8, Changed: true, ChangedRoleIDs: changed})
		record = &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: string(data), ExpiresAt: fixtures.ServiceTime.Add(time.Hour)}
		if kind == "expired" {
			record.ExpiresAt = fixtures.ServiceTime
		}
		if kind == "conflict" {
			record.RequestHash = []byte("different")
		}
	}
	permissionCalls, roleCalls, writes := 0, 0, 0
	d := user.AssignmentDependencies{Transactions: provider, Snapshots: provider, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }}
	d.Bind = func(handle domain.Transaction) user.AssignmentRepositories {
		if handle != tx {
			t.Fatal("different handle")
		}
		tx.Record("bind")
		return user.AssignmentRepositories{
			State: authmocks.SessionStateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
				if got != ctx || id != fixtures.SessionID {
					t.Fatal("state scope")
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
					t.Fatal("actor scope")
				}
				permissionCalls++
				role := uint64(1)
				if restart {
					role = 2
					if kind == "unstable" {
						role = uint64(4 - provider.Calls)
					}
				}
				out := []domain.PermissionContribution{{PermissionCode: string(permission), PermissionApplicationID: 1, RoleID: role, RoleCode: "iam.editor", RoleApplicationID: 1, RoleStatus: types.RoleActive, AssignmentStatus: types.AssignmentActive, ApplicationCode: types.ApplicationIAM, ApplicationStatus: types.ApplicationActive}}
				if kind == "denied" {
					out[0].PermissionCode = "iam.other"
				}
				return out, step(fmt.Sprintf("permissions%d", permissionCalls))
			}},
			Users: sessionmocks.Users{Read: func(got context.Context, id uint64) (domain.SessionUserState, error) {
				if got != ctx || id != input.UserID {
					t.Fatal("target scope")
				}
				if e := step("target"); e != nil {
					return before, e
				}
				if kind == "target absent" {
					return before, auth.ErrNotFound
				}
				return before, nil
			}},
			Assignments: mocks.AssignmentReader{Roles: func(got context.Context, id uint64, ids []uint64) ([]domain.UserRoleAssignmentItem, error) {
				if got != ctx || id != input.UserID || !reflect.DeepEqual(ids, norm.RoleIDs) {
					t.Fatal("role scope", ids)
				}
				roleCalls++
				return rows, step(fmt.Sprintf("roles%d", roleCalls))
			}, List: func(got context.Context, q domain.CatalogQuery) (orm.PageData[domain.UserRoleAssignmentItem], error) {
				if got != ctx || q.TargetUserID != 43 || q.Page != 1 || q.Limit != 20 || !q.UsesKeywordSearch() {
					t.Fatal("list scope")
				}
				page := orm.PageData[domain.UserRoleAssignmentItem]{Items: rows, Total: 3, Page: 1, Limit: 20, TotalPages: 1}
				if kind == "empty" {
					page.Items = nil
					page.Total = 0
					page.TotalPages = 0
				}
				return page, step("list")
			}},
			Policy: mocks.Policy{Count: func(got context.Context, id uint64) (int64, bool, error) {
				if got != ctx || id != input.UserID {
					t.Fatal("guard scope")
				}
				if kind == "count failure" {
					return 0, false, cause
				}
				count := int64(2)
				if kind == "last admin" {
					count = 1
				}
				return count, true, step("admin-count")
			}},
			Mutation: mocks.AssignmentWriter{Grant: func(got context.Context, id uint64, row domain.UserRoleAssignmentItem, actor uint64, at time.Time) (bool, error) {
				if got != ctx || id != 43 || actor != 42 || !at.Equal(fixtures.ServiceTime) || row.Status == "ACTIVE" {
					t.Fatal("grant scope")
				}
				writes++
				if kind == "write2 failure" && writes == 2 {
					return false, cause
				}
				return kind != "zero write", step(fmt.Sprintf("write%d", writes))
			}, Remove: func(got context.Context, id uint64, row domain.UserRoleAssignmentItem, actor uint64, reason string, at time.Time) (bool, error) {
				if got != ctx || id != 43 || actor != 42 || row.ID != 11 || row.Status != "ACTIVE" || reason != "Review access" || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("revoke scope")
				}
				writes++
				return kind != "zero write", step(fmt.Sprintf("write%d", writes))
			}, Version: func(got context.Context, id uint64, v int64, at time.Time) (bool, error) {
				if got != ctx || id != 43 || v != 7 || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("version scope")
				}
				return kind != "zero version", step("version")
			}},
			Idempotency: sessionmocks.Idempotency{Read: func(got domain.IdempotencyScope) (*domain.IdempotencyRecord, error) {
				if got != scope {
					t.Fatal("scope", got, scope)
				}
				return record, step("lookup")
			}, Write: func(got domain.IdempotencyScope, h []byte, out domain.SafeCommandResult, at time.Time) error {
				if got != scope || !bytes.Equal(h, hash) || out.ValidateAssignmentResult(input.UserID) != nil || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("safe result")
				}
				return step("save")
			}},
			Audit: authmocks.AuditWriter{Write: func(a domain.AuditRequest) error {
				record, e := a.Record()
				if e != nil || record.ActorUserID == nil || *record.ActorUserID != 42 || a.TargetID != "43" || a.Reason != "Review access" || a.Before.Version != 7 || a.After.Version != func() int64 {
					if len(changed) > 0 {
						return 8
					}
					return 7
				}() {
					t.Fatal("audit", e)
				}
				if kind == "no-op" && (a.Action != types.AuditNoOp || !a.After.NoOp) {
					t.Fatal("noop audit")
				}
				return step("audit")
			}},
			Outbox: shared.OutboxWriter{Write: func(out domain.EventEnvelope) error {
				record, e := out.Record()
				if e != nil || record.PublishedAt != nil || out.AggregateID != "43" || out.AggregateVersion != 8 || !reflect.DeepEqual(out.Data.ChangedRoleIDs, changed) {
					t.Fatal("event", e, out)
				}
				return step("outbox")
			}},
		}
	}
	svc, e := user.NewAssignmentService(d)
	if e != nil {
		t.Fatal(e)
	}
	var result *domain.SafeCommandResult
	if command == "list" {
		out, err := svc.ListUserRoles(ctx, domain.CatalogQuery{TargetUserID: 43, Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: "reviewer", Mode: orm.SearchModeFullTextTrigram}})
		e = err
		if e == nil {
			if out.UserID != 43 || out.Version != before.Version || kind == "empty" && (out.Page.Total != 0 || len(out.Page.Items) != 0) {
				t.Fatal("list result", out)
			}
			result = &domain.SafeCommandResult{UserID: 43}
		} else if out.UserID != 0 || len(out.Page.Items) != 0 {
			t.Fatal("partial list")
		}
	} else if command == "grant" {
		result, e = svc.GrantUserRoles(ctx, input)
	} else {
		result, e = svc.RevokeUserRoles(ctx, input)
	}
	want := map[string]error{"denied": common.ErrForbidden, "busy": usererrors.ErrResourceBusy, "conflict": usererrors.ErrIdempotencyConflict, "stale version": usererrors.ErrVersionConflict, "target absent": auth.ErrNotFound, "target deleted": auth.ErrNotFound, "unknown role": roleerrors.ErrRoleNotFound, "zero write": usererrors.ErrVersionConflict, "zero version": usererrors.ErrVersionConflict, "last admin": usererrors.ErrLastAdminRequired, "self admin": usererrors.ErrSelfAdminRoleRevokeForbidden, "self admin no-op": usererrors.ErrSelfAdminRoleRevokeForbidden, "count failure": cause, "begin failure": cause, "lock failure": cause, "commit failure": cause, "write2 failure": cause}
	if command == "grant" {
		want["disabled role"] = roleerrors.ErrRoleDisabled
		want["disabled app"] = applicationerrors.ErrApplicationInactive
		want["missing access"] = applicationerrors.ErrApplicationAccessRequired
		want["revoked access"] = applicationerrors.ErrApplicationAccessRequired
		for _, s := range []string{"SUSPENDED", "PENDING", "DEACTIVATED"} {
			want[s] = usererrors.ErrGrantTargetInactive
		}
	}
	expectedFailure := want[kind] != nil || strings.Contains(kind, ":") || kind == "stale actor" || kind == "restricted" || kind == "unstable"
	if restart && kind == "unstable" && (provider.Calls != 3 || !errors.Is(e, usererrors.ErrResourceBusy)) {
		t.Fatal("bounded restart", provider.Calls, e, tx.Calls)
	}
	if restart && kind == "success" && provider.Calls != 2 {
		t.Fatal("predecessor did not restart", provider.Calls, tx.Calls)
	}
	if expectedFailure {
		if e == nil || result != nil {
			t.Fatal("failed request succeeded", result, e, tx.Calls)
		}
		if want[kind] != nil && !errors.Is(e, want[kind]) {
			t.Fatal("identity", e, want[kind], tx.Calls)
		}
		if strings.HasPrefix(kind, "error:") && !errors.Is(e, cause) {
			t.Fatal("cause", e)
		}
		if strings.HasPrefix(kind, "panic:") && !errors.Is(e, common.ErrInternalServerError) {
			t.Fatal("panic", e)
		}
		if strings.HasPrefix(kind, "cancel:") && !errors.Is(e, context.Canceled) {
			t.Fatal("cancel", e)
		}
		if kind != "begin failure" && tx.Calls[len(tx.Calls)-1] != "rollback" {
			t.Fatal("cleanup", tx.Calls)
		}
		if slices.Contains([]string{"denied", "stale actor", "restricted"}, kind) && (slices.Contains(tx.Calls, "lookup") || slices.Contains(tx.Calls, "target")) {
			t.Fatal("authority after disclosure", tx.Calls)
		}
		return
	}
	if e != nil || result == nil || tx.Calls[len(tx.Calls)-1] != "commit" {
		t.Fatal("success", result, e, tx.Calls)
	}
	if command == "list" {
		if !reflect.DeepEqual(tx.Calls, []string{"snapshot", "bind", "state", "permissions1", "target", "list", "commit"}) {
			t.Fatal("snapshot ordering", tx.Calls)
		}
		return
	}
	if result.Version != 8 && kind != "no-op" || result.Changed != (kind != "no-op") || !reflect.DeepEqual(result.ChangedRoleIDs, changed) {
		t.Fatal("result", result)
	}
	if kind == "replay" && (slices.Contains(tx.Calls, "target") || slices.Contains(tx.Calls, "audit") || writes != 0) {
		t.Fatal("replay effects", tx.Calls)
	}
	if kind == "no-op" && (writes != 0 || slices.Contains(tx.Calls, "version") || slices.Contains(tx.Calls, "outbox")) {
		t.Fatal("noop effects", tx.Calls)
	}
	locks := []string{}
	for _, call := range tx.Calls {
		if call == "rollback" {
			locks = nil
		}
		if strings.HasPrefix(call, "lock:") {
			locks = append(locks, strings.TrimPrefix(call, "lock:"))
		}
	}
	if !reflect.DeepEqual(locks, featurecommon.CanonicalLockKeys(locks...)) {
		t.Fatal("lock order", locks)
	}
	if command == "revoke" && !slices.Contains(locks, "auth:guard:last-admin:global") {
		t.Fatal("missing global admin guard")
	}
}

func assignmentRestarts(t *testing.T) {
	// Add a role preceding the initially held requested roles during discovery.
	// The regular command matrix tests append-only discovery; SQL-backed races
	// additionally exercise separate physical transactions and lock retention.
	t.Run("discovered actor role", func(t *testing.T) { assignmentCase(t, "grant", "success", true) })
	t.Run("bounded instability", func(t *testing.T) { assignmentCase(t, "grant", "unstable", true) })
}
