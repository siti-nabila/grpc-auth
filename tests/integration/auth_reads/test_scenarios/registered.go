package test_scenarios

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

func Registered() []testutils.Scenario {
	return []testutils.Scenario{{Name: "registered production Login and consumers enforce session, permission and safe failure outcomes", Run: registeredReads}}
}

func registeredReads(t *testing.T) {
	for _, kind := range []string{"normal", "restricted", "dependency", "revoked", "legacy", "sid-less"} {
		t.Run(kind, func(t *testing.T) {
			now := fixtures.ServiceTime
			data := make([]byte, 96)
			for n := range data {
				data[n] = byte(n)
			}
			credentials, err := authfeature.GenerateInitialCredentials(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			restricted := kind == "restricted"
			state := sessionStep(restricted)
			state.Args = []driver.Value{credentials.SessionID}
			state.Rows[0][6] = credentials.SessionID
			state.Rows[0][9] = now
			state.Rows[0][10] = now.Add(7 * 24 * time.Hour)
			steps := registeredLoginSteps(t, data, restricted)
			if kind != "legacy" && kind != "sid-less" {
				first := state
				if kind == "revoked" {
					first.Rows = nil
				}
				steps = append(steps, first)
			}
			if kind == "normal" || restricted || kind == "dependency" {
				steps = append(steps, mocks.SQLStep{Op: "begin"}, state,
					mocks.SQLStep{Op: "query", SQL: "SELECT id, email, status, version, must_change_password FROM auth", Args: []driver.Value{int64(42)}, Columns: []string{"id", "email", "status", "version", "must_change_password"}, Rows: [][]driver.Value{{int64(42), "member@example.com", "ACTIVE", int64(4), restricted}}})
				profile := mocks.SQLStep{Op: "query", SQL: "FROM profile", Args: []driver.Value{int64(42)}, Columns: []string{"id", "user_id", "name", "address", "phone"}, Rows: [][]driver.Value{{int64(7), int64(42), "Member", "Address", "123"}}}
				if kind == "dependency" {
					profile.Error = errors.New("private controlled SQL failure")
				}
				steps = append(steps, profile)
				if kind == "dependency" {
					steps = append(steps, mocks.SQLStep{Op: "rollback"})
				} else {
					if !restricted {
						steps = append(steps, mocks.SQLStep{Op: "query", SQL: "ORDER BY r.id ASC", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE"}, Columns: []string{"id", "role_name", "role_code", "role_description"}, Rows: [][]driver.Value{{int64(1), "Member", int64(1), nil}}})
					}
					steps = append(steps, mocks.SQLStep{Op: "commit"})
				}
			}
			if kind != "normal" && !restricted {
				steps = append(steps, registeredAttempt(userv1.UserService_Me_FullMethodName)...)
			}
			if kind == "normal" || restricted {
				steps = append(steps, state)
				if !restricted {
					steps = append(steps, mocks.SQLStep{Op: "begin"}, state, permissionStep(false), mocks.SQLStep{Op: "rollback"})
				}
				steps = append(steps, registeredAttempt(userv1.UserService_ListUsers_FullMethodName)...)
			}
			mock := &mocks.SQL{Steps: steps}
			db := mock.Open()
			t.Cleanup(func() { _ = db.Close() })
			conn, foundation := fixtures.FoundationServer(t, db, func() time.Time { return now }, bytes.NewReader(data))
			users := userv1.NewUserServiceClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			// Neither an anonymous RPC nor TesRPC is allowed to touch the pool.
			if _, err := users.Login(ctx, &userv1.AuthRequest{}); err == nil {
				t.Fatal("anonymous Login accepted")
			}
			if _, err := users.TesRPC(fixtures.RPCContext(ctx, ""), &emptypb.Empty{}); err != nil {
				t.Fatal(err)
			}
			issued, err := users.Login(fixtures.RPCContext(ctx, ""), &userv1.AuthRequest{Email: " MEMBER@example.com ", Password: "secret12"})
			if err != nil || issued.SessionId != credentials.SessionID || issued.RefreshToken != credentials.RefreshToken {
				t.Fatal("registered Login failed", err)
			}
			token := issued.Token
			if kind == "legacy" {
				token, err = jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{"user_id": 42, "exp": now.Add(time.Minute).Unix()}).SignedString([]byte("test-only-legacy-key"))
				if err != nil {
					t.Fatal(err)
				}
			}
			if kind == "sid-less" {
				payload := fixtures.AccessPayload()
				delete(payload, "sid")
				token = fixtures.SignAccess(t, payload)
			}
			actor := fixtures.RPCContext(ctx, token)
			me, err := users.Me(actor, &emptypb.Empty{})
			switch kind {
			case "normal", "restricted":
				if err != nil || me.Id != 42 || me.Fullname != "Member" || me.MustChangePassword != restricted || restricted && len(me.RoleCodes) != 0 {
					t.Fatal("registered Me outcome", err)
				}
				_, err = users.ListUsers(actor, &userv1.ListUsersRequest{})
				registeredError(t, err, codes.PermissionDenied, common.ErrForbidden.Key(), common.ErrForbidden.Message(locale.Indonesian))
			case "dependency":
				registeredError(t, err, codes.Unavailable, common.ErrDependencyUnavailable.Key(), common.ErrDependencyUnavailable.Message(locale.Indonesian))
			default:
				registeredError(t, err, codes.Unauthenticated, auth.ErrInvalidSession.Key(), auth.ErrInvalidSession.Message(locale.Indonesian))
			}
			if foundation.Login.AuditFailures() != 0 {
				t.Fatal("attempt audit failed")
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
			if db.Stats().InUse != 0 {
				t.Fatal("registered flow leaked pool slot")
			}
		})
	}
}

func registeredAttempt(method string) []mocks.SQLStep {
	return []mocks.SQLStep{{Op: "begin"}, {Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{nil, "rest-orc", "SECURITY_ATTEMPT_FAILED", "rpc", method, nil, nil, "REQUEST_DENIED", nil, nil, fixtures.ServiceTime}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(9)}}}, {Op: "commit"}}
}

func registeredLoginSteps(t *testing.T, data []byte, restricted bool) []mocks.SQLStep {
	t.Helper()
	c, err := authfeature.GenerateInitialCredentials(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	now := fixtures.ServiceTime
	end := now.Add(7 * 24 * time.Hour)
	columns := []string{"id", "email", "password", "status", "auth_version", "version", "must_change_password", "deleted_at"}
	row := []driver.Value{int64(42), "member@example.com", fixtures.LoginPasswordHash(), "ACTIVE", int64(3), int64(4), restricted, nil}
	steps := []mocks.SQLStep{{Op: "query", SQL: "FROM auth", Args: []driver.Value{"member@example.com"}, Columns: columns, Rows: [][]driver.Value{row}}, {Op: "begin"}}
	for _, key := range []string{"auth:user:42", "auth:session:" + c.SessionID} {
		query, args, err := dialect.NewPostgres().TryLockQuery(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		steps = append(steps, mocks.SQLStep{Op: "query", SQL: query, Args: []driver.Value{args[0]}, Columns: []string{"acquired"}, Rows: [][]driver.Value{{true}}})
	}
	return append(steps, mocks.SQLStep{Op: "query", SQL: "FROM auth", Args: []driver.Value{int64(42)}, Columns: columns, Rows: [][]driver.Value{row}},
		mocks.SQLStep{Op: "query", SQL: "INSERT INTO sessions", Args: []driver.Value{c.SessionID, int64(42), "ACTIVE", now, now, end, nil, nil, nil, nil, nil, nil}, Columns: []string{"id"}, Rows: [][]driver.Value{{c.SessionID}}},
		mocks.SQLStep{Op: "query", SQL: "INSERT INTO refresh_tokens", Args: []driver.Value{c.RefreshID, c.SessionID, c.RefreshDigest, c.FamilyID, now, end, nil, nil, nil}, Columns: []string{"id"}, Rows: [][]driver.Value{{c.RefreshID}}},
		mocks.SQLStep{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{int64(42), nil, "SESSION_CREATED", "session", c.SessionID, nil, `{"status":"ACTIVE","session_id":"` + c.SessionID + `"}`, nil, nil, nil, now}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(1)}}}, mocks.SQLStep{Op: "commit"})
}

func registeredError(t *testing.T, err error, code codes.Code, reason, message string) {
	t.Helper()
	got, ok := grpcerror.Decode(err)
	if !ok || got.Code != code || got.Reason != reason || got.Message != message {
		t.Fatal("registered failure outcome", err)
	}
}
