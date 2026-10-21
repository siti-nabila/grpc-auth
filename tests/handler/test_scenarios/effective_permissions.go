package test_scenarios

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/protobuf/encoding/protojson"
)

func EffectivePermissions() []testutils.Scenario {
	return []testutils.Scenario{{Name: "existing read handler maps safe self and admin permission views and rejects invalid inputs", Run: effectivePermissionHandlers}}
}

func effectivePermissionHandlers(t *testing.T) {
	for _, method := range []string{userv1.UserService_GetMyEffectivePermissions_FullMethodName, userv1.UserService_GetUserEffectivePermissions_FullMethodName} {
		for _, kind := range []string{"normal", "empty", "inactive target", "nil request", "missing principal", "bad filter", "invalid ID", "invalid result"} {
			admin := method == userv1.UserService_GetUserEffectivePermissions_FullMethodName
			if !admin && (kind == "invalid ID" || kind == "inactive target") {
				continue
			}
			t.Run(method+"/"+kind, func(t *testing.T) {
				ctx := shared.PrincipalContext(t, shared.PermissionCallerContext(t, method))
				var want error
				filter := ""
				id := uint64(42)
				state := "ACTIVE"
				if admin {
					id = 80
				}
				if kind == "inactive target" {
					state = "SUSPENDED"
				}
				steps := []mocks.SQLStep{{Op: "begin"}, shared.PermissionSessionStep()}
				if admin {
					steps = append(steps, shared.PermissionRowsStep(42, shared.PermissionRows()))
				}
				steps = append(steps, shared.PermissionTargetStep(id, state), shared.PermissionRowsStep(id, shared.PermissionRows()), mocks.SQLStep{Op: "commit"})
				switch kind {
				case "empty":
					steps[len(steps)-2].Rows = nil
				case "nil request":
					steps = nil
					want = common.ErrBadRequest
				case "missing principal":
					ctx = context.Background()
					steps = nil
					want = auth.ErrInvalidSession
				case "bad filter":
					filter = "IAM"
					steps = nil
					want = common.ErrBadRequest
				case "invalid ID":
					id = 0
					steps = nil
					want = common.ErrBadRequest
				case "invalid result":
					i := len(steps) - 3
					steps[i].Rows[0][2] = int64(0)
					steps = append(steps[:i+1], mocks.SQLStep{Op: "rollback"})
					want = common.ErrDependencyUnavailable
				}
				mock := &mocks.SQL{Context: ctx, Steps: steps}
				db := mock.Open()
				t.Cleanup(func() { _ = db.Close() })
				clockCalls := 0
				h, e := handler.NewSessionReadHandler(database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()), func() time.Time { clockCalls++; return shared.ServiceTime })
				if e != nil {
					t.Fatal(e)
				}
				var res *userv1.GetEffectivePermissionsResponse
				if admin {
					req := &userv1.GetUserEffectivePermissionsRequest{UserId: id, ApplicationCode: filter}
					if kind == "nil request" {
						req = nil
					}
					res, e = h.GetUserEffectivePermissions(ctx, req)
				} else {
					req := &userv1.GetMyEffectivePermissionsRequest{ApplicationCode: filter}
					if kind == "nil request" {
						req = nil
					}
					res, e = h.GetMyEffectivePermissions(ctx, req)
				}
				if want != nil {
					if !errors.Is(e, want) || res != nil {
						t.Fatal(res, e, want)
					}
				} else {
					if e != nil || res.UserId != id || res.UserStatus != state || res.Version != 7 {
						t.Fatal(res, e)
					}
					if kind == "empty" {
						if len(res.Permissions) != 0 {
							t.Fatal("empty mapping")
						}
					} else if len(res.Permissions) != 3 || res.Permissions[0].Code != "clm.contract.read" || len(res.Permissions[0].Roles) != 2 || res.Permissions[0].Roles[0].RoleId != 4 || res.Permissions[2].Roles[1].Code != "iam.auditor" {
						t.Fatal("mapping lost", res)
					}
					b, e := protojson.Marshal(res)
					if e != nil {
						t.Fatal(e)
					}
					for _, bad := range []string{"password", "token", "refresh", "sessionId", "authVersion"} {
						if strings.Contains(string(b), bad) {
							t.Fatal("unsafe response", string(b))
						}
					}
				}
				if kind == "nil request" || kind == "missing principal" {
					if clockCalls != 0 {
						t.Fatal("early rejection used clock")
					}
				} else if clockCalls != 1 {
					t.Fatal("clock call count", clockCalls)
				}
				if e := mock.Verify(); e != nil {
					t.Fatal(e)
				}
				if db.Stats().InUse != 0 {
					t.Fatal("lease leak")
				}
			})
		}
	}
}
