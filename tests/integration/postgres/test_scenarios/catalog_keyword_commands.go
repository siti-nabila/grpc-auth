//go:build postgres

package test_scenarios

import (
	"testing"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func CatalogKeywordCommands() []testutils.Scenario {
	return []testutils.Scenario{{Name: "registered RPCs search refreshed documents and failed projection rolls back command persistence", Run: catalogKeywordCommands}}
}

func catalogKeywordCommands(t *testing.T) {
	h := newRoleHarness(t, userv1.UserService_ListUserRoles_FullMethodName)
	installCatalogSQL(t, h.ctx, h.f.DB, "01_schema_functions_triggers.sql")
	installCatalogSQL(t, h.ctx, h.f.DB, "02_backfill.sql")
	if _, err := h.f.DB.ExecContext(h.ctx, "SELECT * FROM catalog_search_backfill(0,0,0,0,100)"); err != nil {
		t.Fatal(err)
	}
	query := func(word string) *paginatorv1.PageQuery {
		return &paginatorv1.PageQuery{Search: &paginatorv1.Search{Fields: []string{"keyword"}, Keyword: word, Mode: paginatorv1.SearchMode_SEARCH_MODE_FULL_TEXT_TRIGRAM}}
	}
	roles, err := h.roles.ListRoles(h.actor(""), &rolev1.ListRolesRequest{Query: query("editor")})
	if err != nil || len(roles.Items) != 1 || roles.Items[0].Id != 3 {
		t.Fatal("registered role search", err)
	}
	permissions, err := h.permissions.ListPermissions(h.actor(""), &permissionv1.ListPermissionsRequest{Query: query("order")})
	if err != nil || len(permissions.Items) != 2 {
		t.Fatal("registered permission search", err)
	}
	apps, err := h.applications.ListApplications(h.actor(""), &applicationv1.ListApplicationsRequest{Query: query("sales")})
	if err != nil || len(apps.Items) != 1 {
		t.Fatal("registered application search", err)
	}
	credentials := roleCredentialState(t, h)
	request := roleUpdate(3, 7, "Principal Reviewer")
	result, err := h.roles.UpdateRole(h.actor("keyword-update"), request)
	if err != nil || result.Role.Version != 8 {
		t.Fatal("registered rename", err)
	}
	assignments, err := h.users.ListUserRoles(h.actor(""), &userv1.ListUserRolesRequest{UserId: 2, Query: query("reviewer")})
	if err != nil || len(assignments.Items) != 1 || assignments.Items[0].RoleId != 3 {
		t.Fatal("registered assignment search", err)
	}
	if credentials != roleCredentialState(t, h) {
		t.Fatal("projection changed credentials/assignments")
	}
	var before, after string
	if err = h.f.DB.QueryRowContext(h.ctx, "SELECT xmin::text FROM role_search WHERE role_id=3").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = h.roles.UpdateRole(h.actor("keyword-update"), request); err != nil {
		t.Fatal("replay", err)
	}
	if _, err = h.roles.UpdateRole(h.actor("keyword-no-op"), roleUpdate(3, 8, "Principal Reviewer")); err != nil {
		t.Fatal("no-op", err)
	}
	if err = h.f.DB.QueryRowContext(h.ctx, "SELECT xmin::text FROM role_search WHERE role_id=3").Scan(&after); err != nil || before != after {
		t.Fatal("replay/no-op rebuilt projection", err)
	}
	state := roleBusinessState(t, h)
	if _, err = h.f.DB.ExecContext(h.ctx, `CREATE FUNCTION fail_keyword() RETURNS TRIGGER LANGUAGE PLPGSQL AS $$ BEGIN RAISE EXCEPTION 'controlled keyword failure'; END; $$; CREATE TRIGGER fail_keyword BEFORE UPDATE ON role_search FOR EACH ROW EXECUTE FUNCTION fail_keyword()`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.roles.UpdateRole(h.actor("keyword-failure"), roleUpdate(3, 8, "Must Abort")); err == nil {
		t.Fatal("failed projection returned successful command")
	}
	if state != roleBusinessState(t, h) {
		t.Fatal("source/version/audit/event/result survived failed projection")
	}
	if err = h.f.DB.QueryRowContext(h.ctx, "SELECT xmin::text FROM role_search WHERE role_id=3").Scan(&after); err != nil || before != after {
		t.Fatal("failed command committed projection", err)
	}
}
