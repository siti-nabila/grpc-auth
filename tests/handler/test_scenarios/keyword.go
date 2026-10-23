package test_scenarios

import (
	"context"
	"errors"
	"strings"
	"testing"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
)

func KeywordHandlers() []testutils.Scenario {
	return []testutils.Scenario{{Name: "four keyword handlers preserve query metadata context and dependency error without invalid calls", Run: keywordHandlers}}
}

func keywordHandlers(t *testing.T) {
	for _, method := range []string{"roles", "permissions", "applications", "assignments"} {
		for _, kind := range []string{"full", "combined", "empty", "omitted", "bad mode", "unknown mode", "mixed", "overflow", "dependency"} {
			t.Run(method+"/"+kind, func(t *testing.T) {
				ctx := fixtures.PrincipalContext(t, context.Background())
				calls := 0
				cause := errors.New("controlled keyword dependency")
				search := &paginatorv1.Search{Fields: []string{"keyword"}, Keyword: "  Senior.-Reviewer  ", Mode: paginatorv1.SearchMode_SEARCH_MODE_FULL_TEXT_TRIGRAM}
				switch kind {
				case "full":
					search.Mode = paginatorv1.SearchMode_SEARCH_MODE_FULL_TEXT
				case "empty":
					search.Keyword = " \t "
				case "omitted":
					search = nil
				case "bad mode":
					search.Mode = paginatorv1.SearchMode_SEARCH_MODE_UNSPECIFIED
				case "unknown mode":
					search.Mode = paginatorv1.SearchMode(999)
				case "mixed":
					search.Fields = append(search.Fields, "name")
				case "overflow":
					search.Keyword = strings.Repeat(" ", 201)
				}
				check := func(got context.Context, q domain.CatalogQuery) error {
					calls++
					if got != ctx || q.Page != 1 || q.Limit != 20 {
						t.Fatal("request context/defaults lost")
					}
					if kind == "empty" || kind == "omitted" {
						if q.Search != nil {
							t.Fatal("missing keyword filters list")
						}
					} else {
						want := orm.SearchModeFullTextTrigram
						if kind == "full" {
							want = orm.SearchModeFullText
						}
						if !q.UsesKeywordSearch() || q.Search.Keyword != "Senior.-Reviewer" || q.Search.Mode != want {
							t.Fatal("keyword contract altered", q)
						}
					}
					if kind == "dependency" {
						return cause
					}
					return nil
				}
				catalog, _ := handler.NewCatalogHandler(mocks.Catalog{
					Permissions: func(c context.Context, q domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error) {
						return orm.PageData[domain.PermissionItem]{Page: q.Page, Limit: q.Limit}, check(c, q)
					},
					Applications: func(c context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error) {
						return orm.PageData[domain.ApplicationItem]{Page: q.Page, Limit: q.Limit}, check(c, q)
					},
				})
				roles, _ := handler.NewRoleReadHandler(mocks.RoleRead{List: func(c context.Context, q domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error) {
					return orm.PageData[domain.RoleSummary]{Page: q.Page, Limit: q.Limit}, check(c, q)
				}})
				assignments, _ := handler.NewUserRoleAssignmentHandler(mocks.Assignment{List: func(c context.Context, q domain.CatalogQuery) (domain.UserRoleAssignmentPage, error) {
					if q.TargetUserID != 123 {
						t.Fatal("assignment target lost")
					}
					return domain.UserRoleAssignmentPage{UserID: 123, Version: 1, Page: orm.PageData[domain.UserRoleAssignmentItem]{Page: q.Page, Limit: q.Limit}}, check(c, q)
				}})
				query := &paginatorv1.PageQuery{Search: search}
				var err error
				switch method {
				case "roles":
					r, e := roles.ListRoles(ctx, &rolev1.ListRolesRequest{Query: query})
					err = e
					if e == nil && (r.Meta.Page != 1 || r.Meta.Limit != 20) {
						t.Fatal("metadata")
					}
				case "permissions":
					r, e := catalog.ListPermissions(ctx, &permissionv1.ListPermissionsRequest{Query: query})
					err = e
					if e == nil && (r.Meta.Page != 1 || r.Meta.Limit != 20) {
						t.Fatal("metadata")
					}
				case "applications":
					r, e := catalog.ListApplications(ctx, &applicationv1.ListApplicationsRequest{Query: query})
					err = e
					if e == nil && (r.Meta.Page != 1 || r.Meta.Limit != 20) {
						t.Fatal("metadata")
					}
				case "assignments":
					r, e := assignments.ListUserRoles(ctx, &userv1.ListUserRolesRequest{UserId: 123, Query: query})
					err = e
					if e == nil && (r.UserId != 123 || r.Version != 1 || r.Meta.Page != 1 || r.Meta.Limit != 20) {
						t.Fatal("assignment metadata")
					}
				}
				wantCalls := 1
				var want error
				switch kind {
				case "bad mode", "unknown mode", "mixed", "overflow":
					wantCalls = 0
					want = common.ErrBadRequest
				case "dependency":
					want = cause
				}
				if calls != wantCalls || !errors.Is(err, want) {
					t.Fatalf("calls %d want %d error %v want %v", calls, wantCalls, err, want)
				}
			})
		}
	}
}
