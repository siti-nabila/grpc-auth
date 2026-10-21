package test_scenarios

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func RegisteredAssignments() []testutils.Scenario {
	return []testutils.Scenario{{Name: "production assignment registration uses controlled SQL for historical list grant revoke noop and retained replay", Run: registeredAssignments}}
}

func assignmentPermissionStep() mocks.SQLStep {
	s := permissionStep(true)
	s.Rows = nil
	for _, code := range []string{"iam.user.read", "iam.user.role.grant", "iam.user.role.revoke"} {
		s.Rows = append(s.Rows, []driver.Value{code, int64(1), int64(2), "iam.reader", int64(1), "ACTIVE", "ACTIVE", "iam", "ACTIVE", nil})
	}
	return s
}

func assignmentTargetStep() mocks.SQLStep {
	return mocks.SQLStep{Op: "query", SQL: "FROM auth", Args: []driver.Value{int64(80)}, Columns: []string{"id", "status", "version", "auth_version", "deleted_at"}, Rows: [][]driver.Value{{int64(80), "ACTIVE", int64(7), int64(3), nil}}}
}

func assignmentRowStep(id int64, state string, list bool) mocks.SQLStep {
	s := mocks.SQLStep{Op: "query", SQL: "ORDER BY r.id ASC", Args: []driver.Value{int64(80), int64(2)}, Columns: []string{"id", "role_id", "code", "role_code", "name", "status", "role_status", "application_id", "application_code", "application_status", "is_system", "application_access_status", "granted_by", "granted_at", "revoked_by", "revoked_at", "revoke_reason"}, Rows: [][]driver.Value{{id, int64(2), "iam.reader", int64(2), "Reader", state, "ACTIVE", int64(1), "iam", "ACTIVE", false, nil, nil, nil, nil, nil, nil}}}
	if list {
		s.SQL = "ORDER BY ur.id ASC"
		s.Args = []driver.Value{int64(80)}
	}
	return s
}

func assignmentLockSteps(t *testing.T, keys ...string) []mocks.SQLStep {
	t.Helper()
	var steps []mocks.SQLStep
	for _, key := range featurecommon.CanonicalLockKeys(keys...) {
		q, args, err := dialect.NewPostgres().TryLockQuery(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		steps = append(steps, mocks.SQLStep{Op: "query", SQL: q, Args: []driver.Value{args[0]}, Columns: []string{"acquired"}, Rows: [][]driver.Value{{true}}})
	}
	return steps
}

func registeredAssignments(t *testing.T) {
	for _, kind := range []string{"list", "list empty", "list missing", "list deleted", "list denied", "list dependency", "list canceled", "list deadline", "grant", "grant noop", "grant replay", "revoke", "revoke noop", "revoke replay"} {
		t.Run(kind, func(t *testing.T) {
			now := fixtures.ServiceTime
			isList := strings.HasPrefix(kind, "list")
			wantCode, wantReason, wantMessage := codes.OK, "", ""
			grant := kind == "grant" || kind == "grant noop" || kind == "grant replay"
			noop := kind == "grant noop" || kind == "revoke noop"
			replay := kind == "grant replay" || kind == "revoke replay"
			method := userv1.UserService_RevokeUserRoles_FullMethodName
			if grant {
				method = userv1.UserService_GrantUserRoles_FullMethodName
			}
			steps := []mocks.SQLStep{sessionStep(false), {Op: "begin"}}
			if isList {
				state, permission, target := sessionStep(false), assignmentPermissionStep(), assignmentTargetStep()
				count := mocks.SQLStep{Op: "query", SQL: "COUNT(DISTINCT ur.id)", Args: []driver.Value{int64(80)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(1)}}}
				rows := assignmentRowStep(8, "REVOKED", true)
				if kind == "list empty" {
					count.Rows[0][0] = int64(0)
					rows.Rows = nil
				}
				if kind == "list denied" {
					permission.Rows = nil
					wantCode = codes.PermissionDenied
					wantReason = common.ErrForbidden.Key()
					wantMessage = common.ErrForbidden.Message(locale.Indonesian)
				}
				if kind == "list missing" || kind == "list deleted" {
					if kind == "list missing" {
						target.Rows = nil
					} else {
						target.Rows[0][4] = now
					}
					wantCode = codes.NotFound
					wantReason = auth.ErrNotFound.Key()
					wantMessage = auth.ErrNotFound.Message(locale.Indonesian)
				}
				if kind == "list dependency" {
					count.Error = errors.New("private SQL cause")
					wantCode = codes.Unavailable
					wantReason = common.ErrDependencyUnavailable.Key()
					wantMessage = common.ErrDependencyUnavailable.Message(locale.Indonesian)
				}
				if kind == "list canceled" {
					count.Error = context.Canceled
					wantCode = codes.Canceled
				}
				if kind == "list deadline" {
					count.Error = context.DeadlineExceeded
					wantCode = codes.DeadlineExceeded
				}
				steps = append(steps, state, permission)
				if kind != "list denied" {
					steps = append(steps, target)
				}
				if kind != "list denied" && kind != "list missing" && kind != "list deleted" {
					steps = append(steps, count)
				}
				if wantCode == codes.OK {
					steps = append(steps, rows, mocks.SQLStep{Op: "commit"})
				} else {
					reason := strings.ToUpper(strings.ReplaceAll(wantReason, ".", "_"))
					if kind == "list canceled" {
						reason = string(types.AuditRequestCanceled)
					}
					if kind == "list deadline" {
						reason = string(types.AuditDeadlineExceeded)
					}
					steps = append(steps, mocks.SQLStep{Op: "rollback"})
					steps = append(steps, registeredAttempt(userv1.UserService_ListUserRoles_FullMethodName, reason)...)
				}
			} else {
				scope := domain.IdempotencyScope{ActorID: 42, Method: method, Target: "user/80", Key: "assignment-key"}
				hash, err := featurecommon.Fingerprint(scope, featurecommon.FingerprintPayload{RoleIDs: []uint64{2}, Reason: "approved", ExpectedVersion: 7, Presence: []string{"expected_user_version"}})
				if err != nil {
					t.Fatal(err)
				}
				lock, err := featurecommon.IdempotencyLockKey(scope)
				if err != nil {
					t.Fatal(err)
				}
				keys := []string{lock, "auth:guard:policy:global", "auth:user:42", "auth:user:80", "auth:role:2"}
				if !grant {
					keys = append(keys, "auth:guard:last-admin:global")
				}
				steps = append(steps, assignmentLockSteps(t, keys...)...)
				id, state := int64(8), "ACTIVE"
				if grant && !noop {
					id, state = 0, ""
				}
				if !grant && noop {
					state = "REVOKED"
				}
				roles := assignmentRowStep(id, state, false)
				steps = append(steps, assignmentPermissionStep(), roles)
				steps = append(steps, assignmentLockSteps(t, "auth:application:1", "auth:session:"+fixtures.SessionState().SessionID)...)
				steps = append(steps, assignmentPermissionStep(), roles, sessionStep(false), assignmentPermissionStep())
				lookup := mocks.SQLStep{Op: "query", SQL: "FROM idempotency_records", Args: []driver.Value{int64(42), method, scope.Target, scope.Key}, Columns: []string{"actor_id", "method", "route_scope", "key", "request_hash", "safe_response", "created_at", "expires_at"}}
				result := domain.SafeCommandResult{UserID: 80, Version: 8, Changed: true, ChangedRoleIDs: []uint64{2}}
				if noop {
					result.Version, result.Changed, result.ChangedRoleIDs = 7, false, nil
				}
				payload, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				if replay {
					lookup.Rows = [][]driver.Value{{int64(42), method, scope.Target, scope.Key, hash, string(payload), now, now.Add(domain.IdempotencyWindow)}}
				}
				steps = append(steps, lookup)
				if !replay {
					steps = append(steps, assignmentTargetStep(), roles)
					if !noop {
						write := mocks.SQLStep{Op: "exec", SQL: "INSERT INTO user_role", Args: []driver.Value{int64(80), int64(2), "ACTIVE", int64(42), now}}
						if !grant {
							write.SQL, write.Args = "UPDATE user_role SET status = $1", []driver.Value{"REVOKED", int64(42), now, "approved", int64(8), int64(80), int64(2), "ACTIVE"}
						}
						steps = append(steps, write, mocks.SQLStep{Op: "exec", SQL: "UPDATE auth SET", Args: []driver.Value{int64(8), now, int64(80), int64(7)}})
					}
					action, event := types.AuditUserRolesRevoked, types.EventUserRolesRevoked
					if grant {
						action, event = types.AuditUserRolesGranted, types.EventUserRolesGranted
					}
					if noop {
						action = types.AuditNoOp
					}
					after, _ := json.Marshal(domain.SafeSnapshot{Version: result.Version, RoleIDs: result.ChangedRoleIDs, NoOp: noop})
					steps = append(steps, mocks.SQLStep{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{int64(42), nil, string(action), "user", "80", `{"version":7}`, string(after), "approved", nil, nil, now}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(9)}}})
					if !noop {
						actor := uint64(42)
						e := featurecommon.UserEvent(context.Background(), "00000000-0000-4000-8000-000000000000", event, 80, 8, &actor, "", now)
						e.Data.ChangedRoleIDs = []uint64{2}
						r, err := e.Record()
						if err != nil {
							t.Fatal(err)
						}
						steps = append(steps, mocks.SQLStep{Op: "query", SQL: "INSERT INTO outbox_events", Args: []driver.Value{r.EventID, r.EventType, int64(1), "user", "80", int64(8), r.Payload, now, nil, int64(0), now}, Columns: []string{"event_id"}, Rows: [][]driver.Value{{r.EventID}}})
					}
					steps = append(steps, lookup, mocks.SQLStep{Op: "exec", SQL: "INSERT INTO idempotency_records", Args: []driver.Value{int64(42), method, scope.Target, scope.Key, hash, string(payload), now, now.Add(domain.IdempotencyWindow)}})
				}
				steps = append(steps, mocks.SQLStep{Op: "commit"})
			}
			mock := &mocks.SQL{Steps: steps}
			t.Cleanup(func() {
				if e := mock.Verify(); e != nil {
					t.Error(e)
				}
			})
			db := mock.Open()
			t.Cleanup(func() { _ = db.Close() })
			conn, foundation := fixtures.FoundationServer(t, db, func() time.Time { return now }, bytes.NewReader(make([]byte, 16)), userv1.UserService_ListUserRoles_FullMethodName, userv1.UserService_GrantUserRoles_FullMethodName, userv1.UserService_RevokeUserRoles_FullMethodName)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ctx = metadata.AppendToOutgoingContext(fixtures.RPCContext(ctx, fixtures.SignAccess(t, fixtures.AccessPayload())), "idempotency-key", "assignment-key")
			users := userv1.NewUserServiceClient(conn)
			v := uint64(7)
			if isList {
				res, err := users.ListUserRoles(ctx, &userv1.ListUserRolesRequest{UserId: 80})
				if wantCode != codes.OK {
					if status.Code(err) != wantCode || res != nil {
						t.Fatal("registered safe list error", res, err)
					}
					if wantReason != "" {
						registeredError(t, err, wantCode, wantReason, wantMessage)
					}
				} else if kind == "list empty" {
					if err != nil || res.Version != 7 || res.Meta.Total != 0 || res.Meta.TotalPages != 0 || len(res.Items) != 0 {
						t.Fatal("empty target", res, err)
					}
				} else if err != nil || res.UserId != 80 || res.Version != 7 || res.Meta.Total != 1 || len(res.Items) != 1 || res.Items[0].Status != "REVOKED" || res.Items[0].GrantedAt != nil || res.Items[0].GrantedBy != nil {
					t.Fatal("historical list", res, err)
				}
			} else if grant {
				res, err := users.GrantUserRoles(ctx, &userv1.GrantUserRolesRequest{UserId: 80, RoleIds: []uint64{2, 2}, Reason: " approved ", ExpectedUserVersion: &v})
				if err != nil || res.UserId != 80 || res.Changed == noop || res.Version != map[bool]uint64{true: 7, false: 8}[noop] || len(res.ChangedRoleIds) != map[bool]int{true: 0, false: 1}[noop] {
					t.Fatal("grant result", res, err)
				}
			} else {
				res, err := users.RevokeUserRoles(ctx, &userv1.RevokeUserRolesRequest{UserId: 80, RoleIds: []uint64{2, 2}, Reason: " approved ", ExpectedUserVersion: &v})
				if err != nil || res.UserId != 80 || res.Changed == noop || res.Version != map[bool]uint64{true: 7, false: 8}[noop] || len(res.ChangedRoleIds) != map[bool]int{true: 0, false: 1}[noop] {
					t.Fatal("revoke result", res, err)
				}
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
			if db.Stats().InUse != 0 || foundation.Login.AuditFailures() != 0 {
				t.Fatal("pool or independent audit failure")
			}
		})
	}
}
