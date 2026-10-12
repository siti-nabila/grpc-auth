package test_scenarios

import (
	"context"
	"errors"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"math"
	"reflect"
	"strings"
	"testing"
)

func RoleManagementHandlers() []testutils.Scenario {
	return []testutils.Scenario{{Name: "four mutation handlers preserve presence keys context and original safe results", Run: roleManagementHandlers}}
}
func roleManagementHandlers(t *testing.T) {
	for _, method := range []string{"create", "update", "status", "replace"} {
		for _, kind := range []string{"success", "nil", "auth", "missing key", "duplicate key", "long key", "missing version", "overflow version", "zero id", "presence", "invalid", "feature error", "unsafe result", "nil result", "optional key", "empty"} {
			if method == "create" && (kind == "missing version" || kind == "overflow version" || kind == "zero id" || kind == "optional key" || kind == "presence") {
				continue
			}
			if kind == "optional key" && method != "update" {
				continue
			}
			if kind == "empty" && method == "status" {
				continue
			}
			t.Run(method+"/"+kind, func(t *testing.T) {
				name, desc, state := "  Renamed  ", "", "DISABLED"
				version := uint64(3)
				key := []string{"key"}
				switch kind {
				case "missing key", "optional key":
					key = nil
				case "duplicate key":
					key = []string{"key", "other"}
				case "long key":
					key = []string{strings.Repeat("k", 201)}
				case "overflow version":
					version = math.MaxUint64
				}
				md := metadata.Pairs("accept-language", "id")
				md.Set("idempotency-key", key...)
				ctx := fixtures.PrincipalContext(t, metadata.NewIncomingContext(context.Background(), md))
				if kind == "auth" {
					ctx = context.Background()
				}
				saved := domain.RoleCommandResult(domain.RoleDetail{Role: domain.RoleSummary{ID: 7, Code: "sales.editor", Name: "Original saved", ApplicationID: 2, ApplicationCode: "sales", Status: "DISABLED", Version: 4, NumericCode: 81, AffectedUserCount: 99}, Permissions: []domain.PermissionItem{{ID: 9, ApplicationID: 2, ApplicationCode: "sales", Code: "sales.order.read", Description: "Read"}}}, true)
				cause := errors.New("controlled dependency")
				calls := 0
				check := func(got context.Context) (*domain.SafeCommandResult, error) {
					calls++
					if got != ctx {
						t.Fatal("context lost")
					}
					md, _ := metadata.FromIncomingContext(got)
					if md.Get("accept-language")[0] != "id" {
						t.Fatal("locale lost")
					}
					if kind == "feature error" {
						return nil, cause
					}
					if kind == "nil result" {
						return nil, nil
					}
					if kind == "unsafe result" {
						saved.UserID = 8
					}
					return &saved, nil
				}
				h, e := handler.NewRoleManagementHandler(mocks.RoleManagement{
					Create: func(c context.Context, i domain.CreateRoleInput) (*domain.SafeCommandResult, error) {
						if i.Code != "sales.editor" || i.Name != "Renamed" || i.Description == nil || *i.Description != "" || i.Key != "key" {
							t.Fatal("create normalized presence", i)
						}
						return check(c)
					},
					Update: func(c context.Context, i domain.UpdateRoleInput) (*domain.SafeCommandResult, error) {
						if i.RoleID != 7 || i.ExpectedVersion != 3 || i.Description == nil || *i.Description != "" {
							t.Fatal("update presence")
						}
						if kind != "empty" && (i.Name == nil || *i.Name != "Renamed") {
							t.Fatal("name normalization")
						}
						if kind == "optional key" && i.Key != "" {
							t.Fatal("optional key")
						}
						return check(c)
					},
					Status: func(c context.Context, i domain.ChangeRoleStatusInput) (*domain.SafeCommandResult, error) {
						if i.RoleID != 7 || i.ExpectedVersion != 3 || i.Status == nil || *i.Status != "DISABLED" || i.Reason != "Review done" {
							t.Fatal("status presence")
						}
						return check(c)
					},
					Replace: func(c context.Context, i domain.ReplaceRolePermissionsInput) (*domain.SafeCommandResult, error) {
						if i.Permissions == nil {
							t.Fatal("wrapper lost")
						}
						want := 1
						if kind == "empty" {
							want = 0
						}
						if len(i.Permissions.Codes) != want || i.Reason != "Review done" {
							t.Fatal("canonical final set")
						}
						return check(c)
					},
				})
				if e != nil {
					t.Fatal(e)
				}
				id := uint64(7)
				if kind == "zero id" {
					id = 0
				}
				vp := &version
				if kind == "missing version" {
					vp = nil
				}
				var response proto.Message
				switch method {
				case "create":
					r := &rolev1.CreateRoleRequest{ApplicationCode: "sales", Code: "sales.editor", Name: name, Description: &desc, PermissionCodes: []string{"sales.order.read", "sales.order.read"}}
					if kind == "invalid" {
						r.Code = "iam.admin"
					}
					if kind == "empty" {
						r.PermissionCodes = nil
					}
					if kind == "nil" {
						r = nil
					}
					response, e = h.CreateRole(ctx, r)
				case "update":
					r := &rolev1.UpdateRoleRequest{RoleId: id, ExpectedRoleVersion: vp, Name: &name, Description: &desc, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "description"}}}
					if kind == "presence" {
						r.FieldMask = nil
					}
					if kind == "invalid" {
						r.FieldMask.Paths = []string{"status"}
					}
					if kind == "empty" {
						r.Name = nil
						r.FieldMask.Paths = []string{"description"}
					}
					if kind == "nil" {
						r = nil
					}
					response, e = h.UpdateRole(ctx, r)
				case "status":
					r := &rolev1.ChangeRoleStatusRequest{RoleId: id, ExpectedRoleVersion: vp, Status: &state, Reason: "  Review done  "}
					if kind == "presence" {
						r.Status = nil
					}
					if kind == "invalid" {
						state = "UNKNOWN"
					}
					if kind == "nil" {
						r = nil
					}
					response, e = h.ChangeRoleStatus(ctx, r)
				case "replace":
					r := &rolev1.ReplaceRolePermissionsRequest{RoleId: id, ExpectedRoleVersion: vp, Permissions: &rolev1.PermissionSet{Codes: []string{"sales.order.read", "sales.order.read"}}, Reason: "  Review done  "}
					if kind == "presence" {
						r.Permissions = nil
					}
					if kind == "invalid" {
						r.Permissions.Codes = []string{"invalid"}
					}
					if kind == "empty" {
						r.Permissions.Codes = nil
					}
					if kind == "nil" {
						r = nil
					}
					response, e = h.ReplaceRolePermissions(ctx, r)
				}
				valid := kind == "success" || kind == "optional key" || kind == "empty" || method == "update" && kind == "missing key"
				wantCalls := 0
				if valid || kind == "feature error" || kind == "unsafe result" || kind == "nil result" {
					wantCalls = 1
				}
				if calls != wantCalls {
					t.Fatalf("calls %d want %d err %v", calls, wantCalls, e)
				}
				if !valid {
					if e == nil || !reflect.ValueOf(response).IsNil() {
						t.Fatal("invalid result escaped")
					}
					if kind == "feature error" && !errors.Is(e, cause) {
						t.Fatal("cause lost")
					}
					if (kind == "unsafe result" || kind == "nil result") && !errors.Is(e, common.ErrInternalServerError) {
						t.Fatal("unsafe classification")
					}
					return
				}
				if e != nil {
					t.Fatal(e)
				}
				switch r := response.(type) {
				case *rolev1.CreateRoleResponse:
					if r.Role.Name != "Original saved" || r.Role.AffectedUserCount != 99 || len(r.Permissions) != 1 {
						t.Fatal("saved result recomputed")
					}
				case *rolev1.UpdateRoleResponse:
					if r.Role.Name != "Original saved" || r.Role.AffectedUserCount != 99 || !r.Changed {
						t.Fatal("saved update lost")
					}
				case *rolev1.ChangeRoleStatusResponse:
					if r.RoleId != 7 || r.Version != 4 || r.Status != "DISABLED" || !r.Changed {
						t.Fatal("saved status lost")
					}
				case *rolev1.ReplaceRolePermissionsResponse:
					if r.Role.AffectedUserCount != 99 || len(r.Permissions) != 1 || !r.Changed {
						t.Fatal("saved final set lost")
					}
				}
				b, e := protojson.Marshal(response)
				if e != nil {
					t.Fatal(e)
				}
				for _, s := range []string{"password", "token", "sessionId", "userId", "authVersion"} {
					if strings.Contains(string(b), s) {
						t.Fatal("credential field escaped")
					}
				}
			})
		}
	}
	if _, e := handler.NewRoleManagementHandler(nil); e == nil {
		t.Fatal("missing service accepted")
	}
}
