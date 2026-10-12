//go:build postgres

package test_scenarios

import (
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/protobuf/types/known/emptypb"
	"reflect"
	"testing"
)

func RoleEvaluation() []testutils.Scenario {
	return []testutils.Scenario{{Name: "registered role writes immediately change snapshot permissions without changing any credential or assignment row", Run: postgresRoleEvaluation}}
}
func roleEffective(t *testing.T, h *roleHarness) map[string][]uint64 {
	t.Helper()
	if _, e := h.users.Me(shared.RPCContext(h.ctx, h.target.Token), &emptypb.Empty{}); e != nil {
		t.Fatal("existing JWT rejected", e)
	}
	claims, e := loginSecurity(t, h.now).Verifier.Verify(h.target.Token)
	if e != nil {
		t.Fatal(e)
	}
	p := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
	tx, e := p.BeginSnapshot(h.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	ctx := shared.CallerContext(t, h.ctx, userv1.UserService_Me_FullMethodName)
	if _, e = authfeature.AuthenticateSession(ctx, claims, reader.NewSessionStateReader(tx), h.now()); e != nil {
		t.Fatal("snapshot current credential", e)
	}
	rows, e := reader.NewPermissionReader(tx).Permissions(ctx, 2)
	if e != nil {
		t.Fatal(e)
	}
	result := map[string][]uint64{}
	for _, p := range featurecommon.ResolvePermissions(rows) {
		for _, r := range p.Roles {
			result[p.Code] = append(result[p.Code], r.ID)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	return result
}
func postgresRoleEvaluation(t *testing.T) {
	h := newRoleHarness(t)
	before := roleCredentialState(t, h)
	check := func(want map[string][]uint64) {
		t.Helper()
		got := roleEffective(t, h)
		if !reflect.DeepEqual(got, want) {
			t.Fatal("effective contributions", got, want)
		}
		if before != roleCredentialState(t, h) {
			t.Fatal("credentials or assignment history changed")
		}
	}
	both := map[string][]uint64{"sales.order.read": {3, 4}, "sales.order.update": {3}}
	check(both)
	if _, e := h.roles.ChangeRoleStatus(h.actor("disable"), roleStatus(7, "DISABLED")); e != nil {
		t.Fatal(e)
	}
	check(map[string][]uint64{"sales.order.read": {4}})
	if _, e := h.roles.ChangeRoleStatus(h.actor("enable"), roleStatus(8, "ACTIVE")); e != nil {
		t.Fatal(e)
	}
	check(both)
	if _, e := h.roles.ReplaceRolePermissions(h.actor("replace"), roleReplace(9, "sales.order.update")); e != nil {
		t.Fatal(e)
	}
	check(map[string][]uint64{"sales.order.read": {4}, "sales.order.update": {3}})
	if _, e := h.roles.ReplaceRolePermissions(h.actor("clear"), roleReplace(10)); e != nil {
		t.Fatal(e)
	}
	check(map[string][]uint64{"sales.order.read": {4}})
	if _, e := h.f.DB.ExecContext(h.ctx, "UPDATE applications SET status='DISABLED' WHERE id=2"); e != nil {
		t.Fatal(e)
	}
	check(map[string][]uint64{})
	if _, e := h.roles.ReplaceRolePermissions(h.actor("disabled-app"), roleReplace(11, "sales.order.update")); e != nil {
		t.Fatal(e)
	}
	check(map[string][]uint64{})
	if _, e := h.f.DB.ExecContext(h.ctx, "UPDATE applications SET status='ACTIVE' WHERE id=2"); e != nil {
		t.Fatal(e)
	}
	check(map[string][]uint64{"sales.order.read": {4}, "sales.order.update": {3}})
	// Historical revoked role receives mappings but must contribute nothing.
	if _, e := h.roles.ReplaceRolePermissions(h.actor("historical"), &rolev1.ReplaceRolePermissionsRequest{RoleId: 5, ExpectedRoleVersion: managementVersion(1), Reason: "Historical mapping", Permissions: &rolev1.PermissionSet{Codes: []string{"sales.order.read", "sales.order.update"}}}); e != nil {
		t.Fatal(e)
	}
	check(map[string][]uint64{"sales.order.read": {4}, "sales.order.update": {3}})
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("evaluation pool leak")
	}
}
