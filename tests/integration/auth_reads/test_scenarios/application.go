package test_scenarios

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
	"time"
)

func RegisteredApplicationAccess() []testutils.Scenario {
	return []testutils.Scenario{{Name: "D registered production SQL list grant revoke noop replay AND denial context and dependency", Run: registeredApplicationAccess}}
}
func accessPermissionStep() mocks.SQLStep {
	s := assignmentPermissionStep()
	for _, code := range []string{"iam.user.application.grant", "iam.user.application.revoke"} {
		s.Rows = append(s.Rows, []driver.Value{code, int64(1), int64(2), "iam.reader", int64(1), "ACTIVE", "ACTIVE", "iam", "ACTIVE", nil})
	}
	return s
}
func accessRowStep(state string, list bool) mocks.SQLStep {
	s := mocks.SQLStep{Op: "query", SQL: "FROM user_application_access access", Args: []driver.Value{int64(80), int64(2)}, Columns: []string{"user_id", "application_id", "application_code", "name", "application_status", "is_system", "status", "granted_by", "granted_at", "revoked_by", "revoked_at", "revoke_reason"}}
	if state != "" {
		s.Rows = [][]driver.Value{{int64(80), int64(2), "sales", "Sales", "ACTIVE", false, state, nil, fixtures.ServiceTime, nil, nil, nil}}
		if state == "REVOKED" {
			s.Rows[0][10] = fixtures.ServiceTime
			s.Rows[0][11] = "prior revoke"
		}
	}
	if list {
		s.SQL = "ORDER BY a.id ASC"
		s.Args = []driver.Value{int64(80)}
	}
	return s
}
func accessRoleStep(state string, grant bool) mocks.SQLStep {
	s := assignmentRowStep(8, state, false)
	s.Args = []driver.Value{int64(80), int64(2)}
	if grant {
		s.Args[1] = int64(1)
	}
	s.Rows[0][1] = int64(1)
	s.Rows[0][2] = "sales.reader"
	s.Rows[0][7] = int64(2)
	s.Rows[0][8] = "sales"
	s.Rows[0][13] = fixtures.ServiceTime
	if state == "" {
		s.Rows[0][0] = int64(0)
	}
	if state == "REVOKED" {
		s.Rows[0][15] = fixtures.ServiceTime
		s.Rows[0][16] = "prior revoke"
	}
	if !grant {
		s.Args = append(s.Args, "ACTIVE")
	}
	if !grant && state != "ACTIVE" {
		s.Rows = nil
	}
	return s
}

func registeredApplicationAccess(t *testing.T) {
	for _, kind := range []string{"list", "list empty", "list denied", "list dependency", "list canceled", "grant", "grant noop", "grant replay", "revoke", "revoke noop", "revoke replay", "missing app permission", "missing role permission"} {
		t.Run(kind, func(t *testing.T) {
			now := fixtures.ServiceTime
			isList := strings.HasPrefix(kind, "list")
			grant := strings.HasPrefix(kind, "grant") || strings.HasPrefix(kind, "missing")
			noop := strings.HasSuffix(kind, "noop")
			replay := strings.HasSuffix(kind, "replay")
			method := applicationv1.ApplicationService_RevokeApplicationAccess_FullMethodName
			if grant {
				method = applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName
			}
			if isList {
				method = userv1.UserService_ListUserApplicationAccess_FullMethodName
			}
			want := codes.OK
			steps := []mocks.SQLStep{sessionStep(false), {Op: "begin"}}
			permission := accessPermissionStep()
			if kind == "list denied" {
				permission.Rows = nil
				want = codes.PermissionDenied
			}
			if strings.HasPrefix(kind, "missing") {
				code := "iam.user.application.grant"
				if kind == "missing role permission" {
					code = "iam.user.role.grant"
				}
				filtered := [][]driver.Value{}
				for _, r := range permission.Rows {
					if r[0] != code {
						filtered = append(filtered, r)
					}
				}
				permission.Rows = filtered
				want = codes.PermissionDenied
			}
			if isList {
				steps = append(steps, sessionStep(false), permission)
				if want == codes.OK {
					steps = append(steps, assignmentTargetStep())
					count := mocks.SQLStep{Op: "query", SQL: "COUNT(DISTINCT a.id)", Args: []driver.Value{int64(80)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(1)}}}
					if kind == "list empty" {
						count.Rows[0][0] = int64(0)
					}
					if kind == "list dependency" {
						count.Error = errors.New("private SQL")
						want = codes.Unavailable
					}
					if kind == "list canceled" {
						count.Error = context.Canceled
						want = codes.Canceled
					}
					steps = append(steps, count)
					if want == codes.OK {
						row := accessRowStep("REVOKED", true)
						if kind == "list empty" {
							row.Rows = nil
						}
						steps = append(steps, row)
					}
				}
			} else {
				app := mocks.SQLStep{Op: "query", SQL: "FROM applications a", Args: []driver.Value{"sales"}, Columns: []string{"id", "code", "name", "status", "is_system"}, Rows: [][]driver.Value{{int64(2), "sales", "Sales", "ACTIVE", false}}}
				steps = append(steps, sessionStep(false), permission)
				if want == codes.OK {
					scope := domain.IdempotencyScope{ActorID: 42, Method: method, Target: "user/80/application/2", Key: "access-key"}
					ids := []uint64(nil)
					if grant {
						ids = []uint64{1}
					}
					hash, _ := featurecommon.Fingerprint(scope, featurecommon.FingerprintPayload{ApplicationID: 2, RoleIDs: ids, Reason: "approved", ExpectedVersion: 7, Presence: []string{"expected_user_version"}})
					lock, _ := featurecommon.IdempotencyLockKey(scope)
					keys := []string{lock, "auth:guard:policy:global", "auth:user:42", "auth:user:80"}
					if grant {
						keys = append(keys, "auth:role:1")
					}
					state := "ACTIVE"
					accessState := "ACTIVE"
					if grant && !noop {
						state = ""
						accessState = ""
					}
					if !grant && noop {
						state = "REVOKED"
						accessState = "REVOKED"
					}
					roles := accessRoleStep(state, grant)
					steps = append(steps, app)
					steps = append(steps, assignmentLockSteps(t, keys...)...)
					steps = append(steps, accessPermissionStep(), roles)
					discovery := []string{"auth:role:2", "auth:application:1", "auth:application:2", "auth:session:" + fixtures.SessionState().SessionID}
					if !grant && !noop {
						discovery = append(discovery, "auth:role:1")
					}
					steps = append(steps, assignmentLockSteps(t, discovery...)...)
					steps = append(steps, accessPermissionStep(), roles, sessionStep(false), accessPermissionStep(), app)
					lookup := mocks.SQLStep{Op: "query", SQL: "FROM idempotency_records", Args: []driver.Value{int64(42), method, scope.Target, scope.Key}, Columns: []string{"actor_id", "method", "route_scope", "key", "request_hash", "safe_response", "created_at", "expires_at"}}
					result := domain.SafeCommandResult{UserID: 80, ApplicationID: 2, Status: "ACTIVE", Version: 8, Changed: true, ChangedRoleIDs: []uint64{1}}
					if !grant {
						result.Status = "REVOKED"
					}
					if noop {
						result.Version = 7
						result.Changed = false
						result.ChangedRoleIDs = nil
					}
					payload, _ := json.Marshal(result)
					if replay {
						lookup.Rows = [][]driver.Value{{int64(42), method, scope.Target, scope.Key, hash, string(payload), now, now.Add(domain.IdempotencyWindow)}}
					}
					steps = append(steps, lookup)
					if !replay {
						steps = append(steps, assignmentTargetStep(), accessRowStep(accessState, false), roles)
						if !noop {
							access := mocks.SQLStep{Op: "exec", SQL: "INSERT INTO user_application_access", Args: []driver.Value{int64(80), int64(2), "ACTIVE", int64(42), now}}
							role := mocks.SQLStep{Op: "exec", SQL: "INSERT INTO user_role", Args: []driver.Value{int64(80), int64(1), "ACTIVE", int64(42), now}}
							if !grant {
								access.SQL = "UPDATE user_application_access SET status = $1"
								access.Args = []driver.Value{"REVOKED", int64(42), now, "approved", int64(80), int64(2), "ACTIVE"}
								role.SQL = "UPDATE user_role SET status = $1"
								role.Args = []driver.Value{"REVOKED", int64(42), now, "approved", int64(8), int64(80), int64(1), "ACTIVE"}
							}
							steps = append(steps, access, role, mocks.SQLStep{Op: "exec", SQL: "UPDATE auth SET", Args: []driver.Value{int64(8), now, int64(80), int64(7)}})
						}
						action, event := types.AuditApplicationAccessRevoked, types.EventUserApplicationAccessRevoked
						if grant {
							action, event = types.AuditApplicationAccessGranted, types.EventUserApplicationAccessGranted
						}
						if noop {
							action = types.AuditNoOp
						}
						before, _ := json.Marshal(domain.SafeSnapshot{Version: 7, ApplicationID: 2, Status: accessState})
						after, _ := json.Marshal(domain.SafeSnapshot{Version: result.Version, ApplicationID: 2, Status: result.Status, RoleIDs: result.ChangedRoleIDs, NoOp: noop})
						steps = append(steps, mocks.SQLStep{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{int64(42), nil, string(action), "user", "80", string(before), string(after), "approved", nil, nil, now}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(9)}}})
						if !noop {
							actor := uint64(42)
							envelope := featurecommon.UserEvent(context.Background(), "00000000-0000-4000-8000-000000000000", event, 80, 8, &actor, "", now)
							envelope.Data.ApplicationID = 2
							envelope.Data.ApplicationCode = "sales"
							envelope.Data.Status = result.Status
							envelope.Data.ChangedRoleIDs = result.ChangedRoleIDs
							rec, e := envelope.Record()
							if e != nil {
								t.Fatal(e)
							}
							steps = append(steps, mocks.SQLStep{Op: "query", SQL: "INSERT INTO outbox_events", Args: []driver.Value{rec.EventID, rec.EventType, int64(1), "user", "80", int64(8), rec.Payload, now, nil, int64(0), now}, Columns: []string{"event_id"}, Rows: [][]driver.Value{{rec.EventID}}})
						}
						steps = append(steps, lookup, mocks.SQLStep{Op: "exec", SQL: "INSERT INTO idempotency_records", Args: []driver.Value{int64(42), method, scope.Target, scope.Key, hash, string(payload), now, now.Add(domain.IdempotencyWindow)}})
					}
				}
			}
			if want == codes.OK {
				steps = append(steps, mocks.SQLStep{Op: "commit"})
			} else {
				steps = append(steps, mocks.SQLStep{Op: "rollback"})
				reason := "COMMON_FORBIDDEN"
				if want == codes.Unavailable {
					reason = "COMMON_DEPENDENCY_UNAVAILABLE"
				}
				if want == codes.Canceled {
					reason = string(types.AuditRequestCanceled)
				}
				steps = append(steps, registeredAttempt(method, reason)...)
			}
			mock := &mocks.SQL{Steps: steps}
			db := mock.Open()
			t.Cleanup(func() {
				db.Close()
				if e := mock.Verify(); e != nil {
					t.Error(e)
				}
			})
			conn, foundation := fixtures.FoundationServer(t, db, func() time.Time { return now }, bytes.NewReader(make([]byte, 32)), method)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ctx = metadata.AppendToOutgoingContext(fixtures.RPCContext(ctx, fixtures.SignAccess(t, fixtures.AccessPayload())), "idempotency-key", "access-key")
			v := uint64(7)
			var e error
			if isList {
				out, err := userv1.NewUserServiceClient(conn).ListUserApplicationAccess(ctx, &userv1.ListUserApplicationAccessRequest{UserId: 80})
				e = err
				if e == nil && (out.UserId != 80 || out.Version != 7 || kind != "list empty" && (len(out.Items) != 1 || out.Items[0].Status != "REVOKED")) {
					t.Fatal(out)
				}
			} else if grant {
				out, err := applicationv1.NewApplicationServiceClient(conn).GrantApplicationAccess(ctx, &applicationv1.GrantApplicationAccessRequest{UserId: 80, ApplicationCode: " Sales ", RoleIds: []uint64{1, 1}, Reason: " approved ", ExpectedUserVersion: &v})
				e = err
				if e == nil && (out.Changed == noop || out.ApplicationId != 2) {
					t.Fatal(out)
				}
			} else {
				out, err := applicationv1.NewApplicationServiceClient(conn).RevokeApplicationAccess(ctx, &applicationv1.RevokeApplicationAccessRequest{UserId: 80, ApplicationCode: "sales", Reason: "approved", ExpectedUserVersion: &v})
				e = err
				if e == nil && (out.Changed == noop || out.GetAccessStatus() != "REVOKED") {
					t.Fatal(out)
				}
			}
			if status.Code(e) != want {
				t.Fatal("registered error", e, want)
			}
			if want != codes.OK && strings.Contains(e.Error(), "private") {
				t.Fatal("private error leaked")
			}
			if e := mock.Verify(); e != nil {
				t.Fatal(e)
			}
			if db.Stats().InUse != 0 || foundation.Login.AuditFailures() != 0 {
				t.Fatal("cleanup or independent audit")
			}
		})
	}
}
