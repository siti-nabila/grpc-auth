package test_scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/features/application"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/application/mocks"
	authmocks "github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	sessionmocks "github.com/siti-nabila/grpc-auth/tests/features/session/mocks"
	usermocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func Access() []testutils.Scenario {
	return []testutils.Scenario{{Name: "compound access authority snapshot mutation replay and atomic failure matrix", Run: accessMatrix}}
}

func accessMatrix(t *testing.T) {
	if _, e := application.NewService(application.Dependencies{}); e == nil {
		t.Fatal("incomplete constructor")
	}
	for _, cmd := range []string{"list", "grant", "revoke"} {
		cases := []string{"success", "denied", "stale actor", "restricted", "begin failure", "commit failure", "incomplete bind", "target absent", "target deleted", "SUSPENDED", "PENDING", "DEACTIVATED"}
		stages := []string{"state1", "permissions1", "target"}
		if cmd == "list" {
			cases = append(cases, "empty")
			stages = append(stages, "list")
		} else {
			cases = append(cases, "missing app permission", "missing role permission", "locked denied", "busy", "lock failure", "replay", "conflict", "expired", "foreign replay", "stale version", "no-op", "access-only", "absent access", "revoked access", "disabled app", "system app", "IAM", "app absent", "identity changed", "overflow", "zero access", "zero role", "zero version")
			stages = append(stages, "application1", "application2", "permissions2", "permissions3", "permissions4", "state2", "lookup", "access", "roles1", "roles2", "roles3", "access-write", "role-write1", "version", "audit", "event-id", "outbox", "save")
			if cmd == "grant" {
				cases = append(cases, "predecessor", "unstable", "restart identity", "unknown role", "disabled role", "cross app")
			} else {
				cases = append(cases, "more than20", "disabled role")
			}
		}
		for _, stage := range stages {
			for _, fail := range []string{"error", "panic", "cancel"} {
				cases = append(cases, fail+":"+stage)
			}
		}
		for _, kind := range cases {
			t.Run(cmd+"/"+kind, func(t *testing.T) { accessCase(t, cmd, kind) })
		}
	}
}

func accessCase(t *testing.T, cmd, kind string) {
	t.Helper()
	method := applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName
	required := []types.PermissionCode{types.PermissionIAMUserApplicationGrant, types.PermissionIAMUserRoleGrant}
	if cmd == "revoke" {
		method = applicationv1.ApplicationService_RevokeApplicationAccess_FullMethodName
		required = []types.PermissionCode{types.PermissionIAMUserApplicationRevoke, types.PermissionIAMUserRoleRevoke}
	}
	if cmd == "list" {
		method = userv1.UserService_ListUserApplicationAccess_FullMethodName
		required = []types.PermissionCode{types.PermissionIAMUserRead}
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
	provider := &usermocks.AssignmentProvider{Tx: tx}
	cause := errors.New("controlled private failure")
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
	counts := map[string]int{}
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
	numbered := func(stage string) error { counts[stage]++; return step(fmt.Sprintf("%s%d", stage, counts[stage])) }
	input := domain.ApplicationAccessInput{UserID: 43, ApplicationCode: " Sales ", ExpectedVersion: 7, Reason: " Review access ", Key: " exact key "}
	if cmd == "grant" {
		input.RoleIDs = []uint64{2, 1, 2}
	}
	restart := slices.Contains([]string{"predecessor", "unstable", "restart identity"}, kind)
	if restart {
		input.RoleIDs = []uint64{4, 5}
	}
	norm := input.Normalized()
	before := domain.SessionUserState{ID: 43, Status: types.UserActive, Version: 7, AuthVersion: 9}
	if slices.Contains([]string{"SUSPENDED", "PENDING", "DEACTIVATED"}, kind) {
		before.Status = types.UserStatus(kind)
	}
	if kind == "stale version" || kind == "replay" {
		before.Version = 8
	}
	if kind == "target deleted" {
		before.DeletedAt.Valid = true
	}
	if kind == "overflow" {
		before.Version = math.MaxInt64
		input.ExpectedVersion = math.MaxInt64
		norm = input.Normalized()
	}
	app := domain.ApplicationItem{ID: 2, Code: "sales", Name: "Sales", Status: types.ApplicationActive}
	if kind == "disabled app" {
		app.Status = types.ApplicationDisabled
	}
	if kind == "system app" {
		app.IsSystem = true
	}
	if kind == "IAM" {
		app.Code = types.ApplicationIAM
		input.ApplicationCode = "iam"
		norm = input.Normalized()
	}
	rows := []domain.UserRoleAssignmentItem{{RoleID: 1, Code: "sales.editor", ApplicationID: 2, RoleStatus: "ACTIVE"}, {ID: 12, RoleID: 2, Code: "sales.reader", ApplicationID: 2, RoleStatus: "ACTIVE", Status: "ACTIVE"}}
	if restart {
		for i := range rows {
			rows[i].RoleID += 3
		}
	}
	var access *domain.ApplicationAccessItem
	if cmd == "revoke" {
		rows[0].ID = 11
		rows[0].Status = "ACTIVE"
		access = &domain.ApplicationAccessItem{UserID: 43, ApplicationID: 2, Status: "ACTIVE"}
	}
	if kind == "more than20" {
		rows = nil
		for i := uint64(1); i <= 25; i++ {
			rows = append(rows, domain.UserRoleAssignmentItem{ID: i + 10, RoleID: i, Code: "sales.role", ApplicationID: 2, RoleStatus: "ACTIVE", Status: "ACTIVE"})
		}
	}
	if kind == "unknown role" {
		rows = rows[:1]
	}
	if kind == "disabled role" {
		rows[0].RoleStatus = "DISABLED"
	}
	if kind == "cross app" {
		rows[0].ApplicationID = 3
	}
	if kind == "no-op" || kind == "access-only" {
		if cmd == "grant" {
			for i := range rows {
				rows[i].ID = uint64(i + 11)
				rows[i].Status = "ACTIVE"
			}
			if kind == "no-op" {
				access = &domain.ApplicationAccessItem{UserID: 43, ApplicationID: 2, Status: "ACTIVE"}
			}
		} else {
			rows = nil
			if kind == "no-op" {
				access.Status = "REVOKED"
			}
		}
	}
	if kind == "absent access" {
		access = nil
	}
	if kind == "revoked access" {
		access = &domain.ApplicationAccessItem{UserID: 43, ApplicationID: 2, Status: "REVOKED"}
	}
	wantIDs := []uint64{}
	for _, row := range rows {
		if cmd == "revoke" || row.Status != "ACTIVE" {
			wantIDs = append(wantIDs, row.RoleID)
		}
	}
	changed := len(wantIDs) > 0 || cmd == "grant" && (access == nil || access.Status != "ACTIVE") || cmd == "revoke" && access != nil && access.Status == "ACTIVE"
	status := "ACTIVE"
	if cmd == "revoke" {
		status = "REVOKED"
		if access == nil {
			status = ""
		}
	}
	scope := domain.IdempotencyScope{ActorID: 42, Method: method, Target: "user/43/application/2", Key: input.Key}
	hash, _ := featurecommon.Fingerprint(scope, featurecommon.FingerprintPayload{ApplicationID: 2, RoleIDs: norm.RoleIDs, Reason: norm.Reason, ExpectedVersion: norm.ExpectedVersion, Presence: []string{"expected_user_version"}})
	var record *domain.IdempotencyRecord
	if slices.Contains([]string{"replay", "conflict", "expired", "foreign replay"}, kind) {
		result := domain.SafeCommandResult{UserID: 43, ApplicationID: 2, Status: status, Version: 8, Changed: true, ChangedRoleIDs: wantIDs}
		if kind == "foreign replay" {
			result.ApplicationID = 3
		}
		data, _ := json.Marshal(result)
		record = &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: string(data), ExpiresAt: fixtures.ServiceTime.Add(time.Hour)}
		if kind == "conflict" {
			record.RequestHash = []byte("different")
		}
		if kind == "expired" {
			record.ExpiresAt = fixtures.ServiceTime
		}
	}
	checkCtx := func(got context.Context) {
		if got != ctx {
			t.Fatal("request context lost")
		}
	}
	checkPair := func(got context.Context, u, a uint64) {
		checkCtx(got)
		if u != 43 || a != 2 {
			t.Fatal("foreign pair", u, a)
		}
	}
	d := application.Dependencies{Transactions: provider, Snapshots: provider, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }}
	d.Bind = func(handle domain.Transaction) application.Repositories {
		if handle != tx {
			t.Fatal("foreign handle")
		}
		tx.Record("bind")
		if kind == "incomplete bind" {
			return application.Repositories{}
		}
		return application.Repositories{
			State: authmocks.SessionStateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
				checkCtx(got)
				if id != fixtures.SessionID {
					t.Fatal("session")
				}
				fresh := state
				if kind == "stale actor" {
					fresh.AuthVersion++
				}
				if kind == "restricted" {
					fresh.MustChangePassword = true
				}
				return fresh, numbered("state")
			}},
			Permissions: sessionmocks.Permissions{Read: func(got context.Context, id uint64) ([]domain.PermissionContribution, error) {
				checkCtx(got)
				if id != 42 {
					t.Fatal("actor")
				}
				var out []domain.PermissionContribution
				for i, permission := range required {
					if kind == "missing app permission" && i == 0 || kind == "missing role permission" && i == 1 {
						continue
					}
					code := string(permission)
					if kind == "denied" || kind == "locked denied" && counts["permissions"] >= 3 {
						code = "iam.other"
					}
					out = append(out, domain.PermissionContribution{PermissionCode: code, PermissionApplicationID: 1, RoleID: func() uint64 {
						if kind == "unstable" {
							return uint64(4 - provider.Calls)
						}
						return 1
					}(), RoleCode: "iam.editor", RoleApplicationID: 1, RoleStatus: types.RoleActive, AssignmentStatus: types.AssignmentActive, ApplicationCode: types.ApplicationIAM, ApplicationStatus: types.ApplicationActive})
				}
				return out, numbered("permissions")
			}},
			Users: sessionmocks.Users{Read: func(got context.Context, id uint64) (domain.SessionUserState, error) {
				checkCtx(got)
				if id != 43 {
					t.Fatal("target")
				}
				if err := step("target"); err != nil {
					return before, err
				}
				if kind == "target absent" {
					return before, auth.ErrNotFound
				}
				return before, nil
			}},
			Applications: mocks.Lookup{Read: func(got context.Context, code string) (domain.ApplicationItem, error) {
				checkCtx(got)
				if code != norm.ApplicationCode {
					t.Fatal("code")
				}
				if err := numbered("application"); err != nil {
					return app, err
				}
				if kind == "app absent" {
					return app, applicationerrors.ErrApplicationNotFound
				}
				out := app
				if kind == "identity changed" && counts["application"] > 1 || kind == "restart identity" && provider.Calls > 1 {
					out.ID = 3
				}
				return out, nil
			}},
			Access: mocks.Reader{Pair: func(got context.Context, u, a uint64) (*domain.ApplicationAccessItem, error) {
				checkPair(got, u, a)
				return access, step("access")
			}, Roles: func(got context.Context, u, a uint64) ([]domain.UserRoleAssignmentItem, error) {
				checkPair(got, u, a)
				return rows, numbered("roles")
			}, List: func(got context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationAccessItem], error) {
				checkCtx(got)
				if q.TargetUserID != 43 || q.Page != 1 || q.Limit != 20 {
					t.Fatal("query")
				}
				page := orm.PageData[domain.ApplicationAccessItem]{Items: []domain.ApplicationAccessItem{{ApplicationID: 2, Status: "REVOKED"}}, Page: 1, Limit: 20, Total: 1}
				if kind == "empty" {
					page.Items = nil
					page.Total = 0
				}
				return page, step("list")
			}},
			Assignments: usermocks.AssignmentReader{Roles: func(got context.Context, u uint64, ids []uint64) ([]domain.UserRoleAssignmentItem, error) {
				checkCtx(got)
				if u != 43 || !reflect.DeepEqual(ids, norm.RoleIDs) {
					t.Fatal("selection")
				}
				return rows, numbered("roles")
			}},
			AccessMutation: mocks.Writer{Grant: func(got context.Context, u, a uint64, b *domain.ApplicationAccessItem, actor uint64, at time.Time) (bool, error) {
				checkPair(got, u, a)
				if b != access || actor != 42 || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("access grant")
				}
				return kind != "zero access", step("access-write")
			}, Remove: func(got context.Context, u uint64, b domain.ApplicationAccessItem, actor uint64, reason string, at time.Time) (bool, error) {
				checkPair(got, u, b.ApplicationID)
				if b.Status != "ACTIVE" || actor != 42 || reason != norm.Reason || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("access revoke")
				}
				return kind != "zero access", step("access-write")
			}},
			Mutation: usermocks.AssignmentWriter{Grant: func(got context.Context, u uint64, row domain.UserRoleAssignmentItem, actor uint64, at time.Time) (bool, error) {
				checkPair(got, u, row.ApplicationID)
				if actor != 42 || row.Status == "ACTIVE" || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("role grant")
				}
				return kind != "zero role", numbered("role-write")
			}, Remove: func(got context.Context, u uint64, row domain.UserRoleAssignmentItem, actor uint64, reason string, at time.Time) (bool, error) {
				checkPair(got, u, row.ApplicationID)
				if row.Status != "ACTIVE" || actor != 42 || reason != norm.Reason || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("role revoke")
				}
				return kind != "zero role", numbered("role-write")
			}, Version: func(got context.Context, u uint64, v int64, at time.Time) (bool, error) {
				checkCtx(got)
				if u != 43 || v != before.Version || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("version")
				}
				return kind != "zero version", step("version")
			}},
			Idempotency: sessionmocks.Idempotency{Read: func(got domain.IdempotencyScope) (*domain.IdempotencyRecord, error) {
				if got != scope {
					t.Fatal("scope", got)
				}
				return record, step("lookup")
			}, Write: func(got domain.IdempotencyScope, h []byte, out domain.SafeCommandResult, at time.Time) error {
				if got != scope || !bytes.Equal(h, hash) || out.ValidateApplicationAccessResult(43, 2) != nil || !at.Equal(fixtures.ServiceTime) {
					t.Fatal("safe result")
				}
				return step("save")
			}},
			Audit: authmocks.AuditWriter{Write: func(a domain.AuditRequest) error {
				rec, e := a.Record()
				if e != nil || rec.ActorUserID == nil || *rec.ActorUserID != 42 || a.TargetID != "43" || a.Reason != types.AuditReason(norm.Reason) || a.Before.Version != before.Version || a.After.NoOp == changed || a.After.Version != before.Version+func() int64 {
					if changed {
						return 1
					}
					return 0
				}() {
					t.Fatal("audit", e, a)
				}
				if !changed && a.Action != types.AuditNoOp {
					t.Fatal("no-op audit")
				}
				return step("audit")
			}},
			Outbox: shared.OutboxWriter{Write: func(out domain.EventEnvelope) error {
				rec, e := out.Record()
				logical := status
				if logical == "" {
					logical = "REVOKED"
				}
				if e != nil || rec.PublishedAt != nil || out.Data.ApplicationID != 2 || out.Data.ApplicationCode != app.Code || out.Data.Status != logical || out.AggregateVersion != before.Version+1 || !slices.Equal(out.Data.ChangedRoleIDs, wantIDs) {
					t.Fatal("event", e, out)
				}
				return step("outbox")
			}},
		}
	}
	svc, e := application.NewService(d)
	if e != nil {
		t.Fatal(e)
	}
	var result *domain.SafeCommandResult
	if cmd == "list" {
		out, err := svc.ListUserApplicationAccess(ctx, domain.CatalogQuery{TargetUserID: 43})
		e = err
		if e == nil {
			if out.UserID != 43 || out.Version != before.Version || kind == "empty" && out.Page.Total != 0 {
				t.Fatal("list", out)
			}
			result = &domain.SafeCommandResult{UserID: 43}
		} else if out.UserID != 0 || len(out.Page.Items) != 0 {
			t.Fatal("partial list")
		}
	} else if cmd == "grant" {
		result, e = svc.GrantApplicationAccess(ctx, input)
	} else {
		result, e = svc.RevokeApplicationAccess(ctx, input)
	}
	want := map[string]error{"denied": common.ErrForbidden, "missing app permission": common.ErrForbidden, "missing role permission": common.ErrForbidden, "locked denied": common.ErrForbidden, "busy": usererrors.ErrResourceBusy, "lock failure": cause, "begin failure": cause, "incomplete bind": common.ErrInternalServerError, "commit failure": cause, "target absent": auth.ErrNotFound, "target deleted": auth.ErrNotFound, "conflict": usererrors.ErrIdempotencyConflict, "foreign replay": common.ErrInternalServerError, "stale version": usererrors.ErrVersionConflict, "system app": applicationerrors.ErrApplicationAccessSystemProtected, "IAM": applicationerrors.ErrApplicationAccessSystemProtected, "app absent": applicationerrors.ErrApplicationNotFound, "identity changed": common.ErrDependencyUnavailable, "restart identity": common.ErrDependencyUnavailable, "unstable": usererrors.ErrResourceBusy, "overflow": common.ErrInternalServerError, "zero access": usererrors.ErrVersionConflict, "zero role": usererrors.ErrVersionConflict, "zero version": usererrors.ErrVersionConflict}
	if cmd == "grant" {
		want["unknown role"] = roleerrors.ErrRoleNotFound
		want["disabled role"] = roleerrors.ErrRoleDisabled
		want["disabled app"] = applicationerrors.ErrApplicationInactive
		want["cross app"] = common.ErrBadRequest
		for _, status := range []string{"PENDING", "SUSPENDED", "DEACTIVATED"} {
			want[status] = usererrors.ErrGrantTargetInactive
		}
	}
	failure := want[kind] != nil || strings.Contains(kind, ":") || kind == "stale actor" || kind == "restricted"
	if failure {
		if e == nil || result != nil {
			t.Fatal("failed command succeeded", e, result, tx.Calls)
		}
		if want[kind] != nil && !errors.Is(e, want[kind]) {
			t.Fatal("error identity", e, want[kind])
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
		if slices.Contains([]string{"denied", "missing app permission", "missing role permission", "locked denied", "stale actor", "restricted"}, kind) && (slices.Contains(tx.Calls, "target") || slices.Contains(tx.Calls, "lookup")) {
			t.Fatal("disclosed before authority", tx.Calls)
		}
		return
	}
	if e != nil || result == nil || tx.Calls[len(tx.Calls)-1] != "commit" {
		t.Fatal("success", e, result, tx.Calls)
	}
	if cmd == "list" {
		if !reflect.DeepEqual(tx.Calls, []string{"snapshot", "bind", "state1", "permissions1", "target", "list", "commit"}) {
			t.Fatal("snapshot", tx.Calls)
		}
		return
	}
	if kind == "replay" {
		if slices.Contains(tx.Calls, "target") || slices.Contains(tx.Calls, "save") || slices.Contains(tx.Calls, "audit") {
			t.Fatal("replay effects", tx.Calls)
		}
	} else {
		if result.Changed != changed || result.Version != before.Version+func() int64 {
			if changed {
				return 1
			}
			return 0
		}() || !slices.Equal(result.ChangedRoleIDs, wantIDs) || result.Status != status {
			t.Fatal("result", result)
		}
		if !changed && (counts["role-write"] > 0 || slices.Contains(tx.Calls, "access-write") || slices.Contains(tx.Calls, "version") || slices.Contains(tx.Calls, "outbox")) {
			t.Fatal("no-op effects", tx.Calls)
		}
		if !restart && (counts["permissions"] != 4 || counts["state"] != 2 || counts["application"] != 2 || counts["roles"] != 3) {
			t.Fatal("read count", counts)
		}
	}
	if kind == "predecessor" && provider.Calls != 2 {
		t.Fatal("predecessor retries", provider.Calls)
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
	if slices.Contains(locks, "auth:guard:last-admin:global") {
		t.Fatal("unexpected last-admin guard")
	}
}
