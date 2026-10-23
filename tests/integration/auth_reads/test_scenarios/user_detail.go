package test_scenarios

import (
	"context"
	"crypto/tls"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"
)

func UserDetail() []testutils.Scenario {
	return []testutils.Scenario{{Name: "admin detail reads safe target and authority in one snapshot with cleanup on every failure", Run: userDetailSnapshot}}
}

func managementCallerContext(t *testing.T, method string) context.Context {
	t.Helper()
	cfg := fixtures.ServiceConfig()
	for _, call := range managementRPCs() {
		cfg.Callers[0].Methods = append(cfg.Callers[0].Methods, call.method)
	}
	v, err := serviceauth.NewVerifier(cfg, func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, fixtures.ServiceToken(1), "user_id", "999"))
	ctx, err = v.Authenticate(ctx, method)
	if err != nil {
		t.Fatal(err)
	}
	return fixtures.PrincipalContext(t, ctx)
}

func userDetailSnapshot(t *testing.T) {
	for _, kind := range []string{"PENDING", "ACTIVE", "SUSPENDED", "DEACTIVATED", "absent profile", "missing target", "deleted target", "denied", "restricted", "missing session", "state failure", "permission failure", "detail failure", "role failure", "application failure", "commit failure", "nil request", "invalid target"} {
		t.Run(kind, func(t *testing.T) {
			ctx := managementCallerContext(t, userv1.UserService_GetUser_FullMethodName)
			cause := errors.New("controlled detail failure")
			var want error
			steps := []mocks.SQLStep{}
			req := &userv1.GetUserRequest{Id: 80}
			if kind == "nil request" {
				req = nil
				want = common.ErrBadRequest
			}
			if kind == "invalid target" {
				req.Id = 2147483648
				want = common.ErrBadRequest
			}
			if want == nil {
				state := sessionStep(kind == "restricted")
				if kind == "missing session" {
					state.Rows = nil
					want = auth.ErrInvalidSession
				}
				if kind == "state failure" {
					state.Error = cause
					want = cause
				}
				if kind == "restricted" {
					want = common.ErrForbidden
				}
				steps = append(steps, mocks.SQLStep{Op: "begin"}, state)
				if want == nil {
					permission := permissionStep(kind != "denied")
					if kind == "denied" {
						want = common.ErrForbidden
					}
					if kind == "permission failure" {
						permission.Error = cause
						want = cause
					}
					steps = append(steps, permission)
				}
				if want == nil {
					status := kind
					if kind != "PENDING" && kind != "ACTIVE" && kind != "SUSPENDED" && kind != "DEACTIVATED" {
						status = "ACTIVE"
					}
					name := "Target user"
					if kind == "absent profile" {
						name = ""
					}
					detail := mocks.SQLStep{Op: "query", SQL: "a.deleted_at IS NULL", Args: []driver.Value{int64(80)}, Columns: []string{"id", "email", "name", "address", "phone", "country_code", "status", "version", "created_at", "must_change_password"}, Rows: [][]driver.Value{{int64(80), "target@example.test", name, "", "", "+62", status, int64(7), fixtures.ServiceTime, true}}}
					if kind == "missing target" || kind == "deleted target" {
						detail.Rows = nil
						want = auth.ErrNotFound
					}
					if kind == "detail failure" {
						detail.Error = cause
						want = cause
					}
					steps = append(steps, detail)
				}
				if want == nil {
					role := mocks.SQLStep{Op: "query", SQL: "FROM user_role ur", Args: []driver.Value{int64(80), "ACTIVE", "ACTIVE"}, Columns: []string{"user_id", "role_id", "code", "name", "status", "role_status"}, Rows: [][]driver.Value{{int64(80), int64(2), "iam.member", "Member", "ACTIVE", "ACTIVE"}}}
					if kind == "role failure" {
						role.Error = cause
						want = cause
					}
					steps = append(steps, role)
				}
				if want == nil {
					app := mocks.SQLStep{Op: "query", SQL: "FROM user_application_access ua", Args: []driver.Value{int64(80), "ACTIVE", "ACTIVE"}, Columns: []string{"user_id", "application_id", "code", "name", "status", "application_status"}, Rows: [][]driver.Value{{int64(80), int64(1), "iam", "IAM", "ACTIVE", "ACTIVE"}}}
					if kind == "application failure" {
						app.Error = cause
						want = cause
					}
					steps = append(steps, app)
				}
				if want != nil {
					steps = append(steps, mocks.SQLStep{Op: "rollback"})
				} else {
					commit := mocks.SQLStep{Op: "commit"}
					if kind == "commit failure" {
						commit.Error = cause
						want = cause
					}
					steps = append(steps, commit)
				}
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			defer db.Close()
			h, err := handler.NewSessionReadHandler(database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()), func() time.Time { return fixtures.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			res, err := h.GetUser(ctx, req)
			if want != nil {
				if !errors.Is(err, want) || res != nil {
					t.Fatal("unexpected detail failure", res, err)
				}
				if want == auth.ErrNotFound && errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal("not-found mapped as dependency")
				}
				if want == cause && !errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal("dependency identity lost", err)
				}
				verifyReadErrorLocale(t, ctx, err)
			} else {
				if err != nil || res.GetUser().GetId() != 80 || res.User.Email != "target@example.test" || res.User.CountryCode != "+62" || res.User.Version != 7 || !res.User.MustChangePassword || len(res.User.Roles) != 1 || len(res.User.ApplicationAccess) != 1 {
					t.Fatal("unsafe or incorrect target detail", res, err)
				}
				b, err := protojson.Marshal(res)
				if err != nil {
					t.Fatal(err)
				}
				for _, bad := range []string{`"password":`, `"authVersion":`, `"token":`, `"refreshToken":`} {
					if strings.Contains(string(b), bad) {
						t.Fatal("credential exposed", string(b))
					}
				}
			}
			if kind == "nil request" || kind == "invalid target" {
				if err := mock.Verify(); err != nil {
					t.Fatal(err)
				}
			} else {
				verify(t, mock)
			}
		})
	}
}
