package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func CatalogReadHandlers() []testutils.Scenario {
	return []testutils.Scenario{{Name: "read handlers preserve safe nullable metadata projection defaults and request context", Run: catalogHandlers}}
}

func catalogHandlers(t *testing.T) {
	for _, method := range []string{"roles", "role", "permissions", "applications"} {
		for _, kind := range []string{"success", "defaults", "projection", "nil", "auth", "invalid query", "feature error", "forbidden"} {
			if method == "role" && (kind == "projection" || kind == "defaults") {
				continue
			}
			t.Run(method+"/"+kind, func(t *testing.T) {
				ctx := fixtures.PrincipalContext(t, metadata.NewIncomingContext(context.Background(), metadata.Pairs("accept-language", "id", "user_id", "999")))
				if kind == "auth" {
					ctx = context.Background()
				}
				query := &paginatorv1.PageQuery{Page: 2, Limit: 5, Sort: []*paginatorv1.Sort{{Field: "code", Desc: true}}, Search: &paginatorv1.Search{Fields: []string{"code"}, Keyword: "  sales  ", Mode: paginatorv1.SearchMode_SEARCH_MODE_PREFIX}}
				if kind == "defaults" {
					query = nil
				}
				if kind == "projection" {
					query.Fields = []string{"name"}
					if method == "permissions" {
						query.Fields = []string{"code"}
					}
				}
				if kind == "invalid query" {
					query.Fields = []string{"password"}
				}
				calls := 0
				cause := errors.New("controlled handler dependency")
				role := domain.RoleSummary{ID: 7, Code: "sales.editor", Name: "Editor", ApplicationID: 2, ApplicationCode: "sales", Status: "DISABLED", Version: 3, NumericCode: 81, AffectedUserCount: 8}
				check := func(got context.Context, q domain.CatalogQuery) error {
					calls++
					if got != ctx {
						t.Fatal("context/locale lost")
					}
					md, _ := metadata.FromIncomingContext(got)
					if md.Get("accept-language")[0] != "id" {
						t.Fatal("locale lost")
					}
					if kind == "feature error" {
						return cause
					}
					if kind == "forbidden" {
						return common.ErrForbidden
					}
					if kind == "defaults" {
						if q.Page != 1 || q.Limit != 20 || len(q.Sort) != 1 || q.Sort[0].Field != "id" {
							t.Fatal("defaults", q)
						}
					} else if q.Page != 2 || q.Limit != 5 || q.Search.Keyword != "sales" || q.Search.Mode != orm.SearchModePrefix || !q.Sort[0].Desc {
						t.Fatal("query mapping", q)
					}
					return nil
				}
				roleHandler, err := handler.NewRoleReadHandler(mocks.RoleRead{List: func(got context.Context, q domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error) {
					return orm.PageData[domain.RoleSummary]{Items: []domain.RoleSummary{role}, Total: 8, Page: q.Page, Limit: q.Limit, TotalPages: 2, HasNext: true, HasPrev: true, NextCursor: "opaque"}, check(got, q)
				}, Get: func(got context.Context, id uint64) (domain.RoleDetail, error) {
					calls++
					if got != ctx || id != 7 {
						t.Fatal("role target/context mapping")
					}
					if kind == "feature error" {
						return domain.RoleDetail{}, cause
					}
					if kind == "forbidden" {
						return domain.RoleDetail{}, common.ErrForbidden
					}
					return domain.RoleDetail{Role: role, Permissions: []domain.PermissionItem{{ID: 9, ApplicationID: 2, ApplicationCode: "sales", Code: "sales.order.read", Description: "Read"}}}, nil
				}})
				if err != nil {
					t.Fatal(err)
				}
				catalogHandler, err := handler.NewCatalogHandler(mocks.Catalog{Permissions: func(got context.Context, q domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error) {
					return orm.PageData[domain.PermissionItem]{Items: []domain.PermissionItem{{ID: 9, ApplicationID: 2, ApplicationCode: "sales", Code: "sales.order.read", Description: "Read"}}, Page: q.Page, Limit: q.Limit, Total: 1, TotalPages: 1}, check(got, q)
				}, Applications: func(got context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error) {
					return orm.PageData[domain.ApplicationItem]{Items: []domain.ApplicationItem{{ID: 2, Code: "sales", Name: "Sales", Status: "DISABLED", IsSystem: true}}, Page: q.Page, Limit: q.Limit, Total: 1, TotalPages: 1}, check(got, q)
				}})
				if err != nil {
					t.Fatal(err)
				}
				var response proto.Message
				switch method {
				case "roles":
					in := &rolev1.ListRolesRequest{Query: query, Filter: &rolev1.RoleFilter{ApplicationCode: "sales", Statuses: []string{"DISABLED", "ACTIVE"}}}
					if kind == "nil" {
						in = nil
					}
					r, e := roleHandler.ListRoles(ctx, in)
					response = r
					err = e
					if err == nil {
						if r.Items[0].Id != 7 || r.Meta.Total != 8 || r.NextCursor != "opaque" {
							t.Fatal("role page mapping")
						}
						if kind == "projection" {
							if r.Items[0].Name != "Editor" || r.Items[0].Code != "" || r.Items[0].Version != 0 || r.Items[0].AffectedUserCount != 0 {
								t.Fatal("projection leaked")
							}
						} else if r.Items[0].AffectedUserCount != 8 || r.Items[0].RoleCode != 81 {
							t.Fatal("role count/code")
						}
						if r.Items[0].Description != nil || r.Items[0].CreatedAt != nil || r.Items[0].UpdatedAt != nil {
							t.Fatal("null metadata changed")
						}
					}
				case "role":
					in := &rolev1.GetRoleRequest{RoleId: 7}
					if kind == "nil" {
						in = nil
					}
					if kind == "invalid query" {
						in.RoleId = 0
					}
					r, e := roleHandler.GetRole(ctx, in)
					response = r
					err = e
					if err == nil && (r.Role.Id != 7 || r.Role.AffectedUserCount != 8 || len(r.Permissions) != 1) {
						t.Fatal("detail mapping")
					}
				case "permissions":
					in := &permissionv1.ListPermissionsRequest{Query: query, ApplicationCode: "sales"}
					if kind == "nil" {
						in = nil
					}
					r, e := catalogHandler.ListPermissions(ctx, in)
					response = r
					err = e
					if err == nil {
						if r.Items[0].Id != 9 || r.Meta.Total != 1 {
							t.Fatal("permission mapping")
						}
						if kind == "projection" && (r.Items[0].Description != "" || r.Items[0].ApplicationId != 0 || r.Items[0].Code != "sales.order.read") {
							t.Fatal("permission projection")
						}
					}
				case "applications":
					in := &applicationv1.ListApplicationsRequest{Query: query, Filter: &applicationv1.ApplicationFilter{Statuses: []string{"ACTIVE", "DISABLED"}}}
					if kind == "nil" {
						in = nil
					}
					r, e := catalogHandler.ListApplications(ctx, in)
					response = r
					err = e
					if err == nil {
						if r.Items[0].Id != 2 || r.Meta.Total != 1 {
							t.Fatal("app mapping")
						}
						if kind == "projection" && (r.Items[0].Code != "" || r.Items[0].Status != "" || r.Items[0].IsSystem) {
							t.Fatal("app projection")
						}
					}
				}
				var want error
				wantCalls := 1
				switch kind {
				case "nil", "invalid query":
					want = common.ErrBadRequest
					wantCalls = 0
				case "auth":
					want = auth.ErrInvalidSession
					wantCalls = 0
				case "feature error":
					want = cause
				case "forbidden":
					want = common.ErrForbidden
				}
				if !errors.Is(err, want) || calls != wantCalls {
					t.Fatalf("err %v want %v calls %d want %d", err, want, calls, wantCalls)
				}
				if err != nil {
					if !reflect.ValueOf(response).IsNil() {
						t.Fatal("partial response escaped")
					}
					return
				}
				b, err := protojson.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				for _, unsafe := range []string{"password", "token", "session_id", "user_id", "auth_version"} {
					if strings.Contains(string(b), unsafe) {
						t.Fatal("unsafe response field")
					}
				}
			})
		}
	}
	// Explicit empty description and timestamps remain present when supplied.
	ctx := fixtures.PrincipalContext(t, context.Background())
	empty := ""
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	h, _ := handler.NewRoleReadHandler(mocks.RoleRead{Get: func(context.Context, uint64) (domain.RoleDetail, error) {
		return domain.RoleDetail{Role: domain.RoleSummary{ID: 7, Description: &empty, CreatedAt: &at, UpdatedAt: &at}}, nil
	}})
	r, err := h.GetRole(ctx, &rolev1.GetRoleRequest{RoleId: 7})
	if err != nil || r.Role.Description == nil || *r.Role.Description != "" || r.Role.CreatedAt == nil || !r.Role.UpdatedAt.AsTime().Equal(at) {
		t.Fatal("nullable presence", err)
	}
}
