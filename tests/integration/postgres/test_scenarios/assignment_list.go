//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"reflect"
	"sync"
	"testing"
)

func AssignmentLists() []testutils.Scenario {
	return []testutils.Scenario{{Name: "all17 scalar directions mixed null keys literal filters unique totals and projection", Run: postgresAssignmentQueries}, {Name: "two backends commit authority target provenance and total between authority and data reads", Run: postgresAssignmentSnapshot}}
}

func assignmentDeps(h *roleHarness) user.AssignmentDependencies {
	p := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
	return user.AssignmentDependencies{Transactions: p, Snapshots: p, Bind: user.BindAssignmentRepositories, Now: h.now, EventID: eventIDs()}
}
func assignmentService(t *testing.T, d user.AssignmentDependencies) user.AssignmentService {
	t.Helper()
	s, e := user.NewAssignmentService(d)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func postgresAssignmentQueries(t *testing.T) {
	h := newAssignmentHarness(t)
	_, e := h.f.DB.ExecContext(h.ctx, `UPDATE role SET role_name='equal' WHERE id IN(3,4);
 UPDATE role SET role_name='literal %_\ value' WHERE id IN(5,6);
 UPDATE role SET status='DISABLED' WHERE id=3;UPDATE applications SET status='DISABLED' WHERE id=2;
 UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled fixture revocation' WHERE user_id=2 AND application_id=2;
 INSERT INTO user_role(user_id,role_id,granted_by,granted_at) VALUES(2,6,1,'2026-01-01');
 INSERT INTO user_role(user_id,role_id,status,granted_by,granted_at,revoked_by,revoked_at,revoke_reason) VALUES(2,7,'REVOKED',2,'2026-01-01',1,'2026-01-02','earlier review');`)
	if e != nil {
		t.Fatal(e)
	}
	provider := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
	tx, e := provider.BeginSnapshot(h.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	r := reader.NewUserRoleAssignmentReader(tx)
	checkCatalogSorts(t, h.ctx, domain.UserRoleAssignmentCatalog, r.ListAssignments, domain.CatalogQuery{TargetUserID: 2})
	for _, c := range []struct {
		q     domain.CatalogQuery
		roles []uint64
	}{
		{domain.CatalogQuery{ApplicationCode: "sales", Statuses: []string{"ACTIVE", "REVOKED"}}, []uint64{3, 4, 5, 6}},
		{domain.CatalogQuery{Statuses: []string{"REVOKED"}}, []uint64{5, 7}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Keyword: `%_\`}}, []uint64{5, 6}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Keyword: "literal", Fields: []string{"name"}, Mode: orm.SearchModePrefix}}, []uint64{5, 6}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Keyword: "IAM", Fields: []string{"application_code"}}}, []uint64{1, 7}},
		{domain.CatalogQuery{ApplicationCode: "missing"}, []uint64{}},
		{domain.CatalogQuery{Page: 2, Limit: 2}, []uint64{4, 5}},
	} {
		c.q.TargetUserID = 2
		page, e := r.ListAssignments(h.ctx, c.q)
		if e != nil {
			t.Fatal(e)
		}
		roles := []uint64{}
		for _, row := range page.Items {
			roles = append(roles, row.RoleID)
		}
		if !reflect.DeepEqual(roles, c.roles) {
			t.Fatal("query", c.q, roles, c.roles)
		}
	}
	first, e := r.ListAssignments(h.ctx, domain.CatalogQuery{TargetUserID: 2, Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	q := domain.CatalogQuery{TargetUserID: 1, LastID: first.NextCursor}
	if _, e = r.ListAssignments(h.ctx, q); !errors.Is(e, common.ErrBadRequest) {
		t.Fatal("cross target cursor", e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("list pool leak")
	}
}

type assignmentTargetBarrier struct {
	inner   domain.SessionUserReader
	ready   chan struct{}
	release <-chan struct{}
}

func (b assignmentTargetBarrier) GetByID(c context.Context, id uint64) (domain.SessionUserState, error) {
	close(b.ready)
	select {
	case <-b.release:
		return b.inner.GetByID(c, id)
	case <-c.Done():
		return domain.SessionUserState{}, c.Err()
	}
}

func postgresAssignmentSnapshot(t *testing.T) {
	h := newAssignmentHarness(t)
	actor := managementPrincipal(t, h.managementHarness, h.admin.Token, userv1.UserService_ListUserRoles_FullMethodName)
	d := assignmentDeps(h)
	ready, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	done := make(chan snapshotResult, 1)
	finished := false
	defer func() {
		unblock()
		if !finished {
			select {
			case <-done:
			case <-actor.Done():
				t.Error("snapshot worker cleanup")
			}
		}
	}()
	pid := 0
	d.Bind = func(tx domain.Transaction) user.AssignmentRepositories {
		pid = transactionBackend(t, tx)
		r := user.BindAssignmentRepositories(tx)
		r.Users = assignmentTargetBarrier{r.Users, ready, release}
		return r
	}
	svc := assignmentService(t, d)
	go func() {
		out, e := svc.ListUserRoles(actor, domain.CatalogQuery{TargetUserID: 2})
		done <- snapshotResult{out, e}
	}()
	select {
	case <-ready:
	case out := <-done:
		finished = true
		t.Fatal("snapshot early result", out.err)
	case <-actor.Done():
		t.Fatal("snapshot deadline")
	}
	conn, e := h.f.DB.Conn(actor)
	if e != nil {
		t.Fatal(e)
	}
	other := physicalIdentity(t, actor, conn, h.f.Name)
	if pid == 0 || pid == other {
		t.Fatal("same backend")
	}
	tx, e := conn.BeginTx(actor, nil)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.ExecContext(actor, `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2;UPDATE auth SET version=version+1,status='SUSPENDED' WHERE id=2;UPDATE user_role SET granted_by=1,granted_at='2026-01-01' WHERE user_id=2;UPDATE role SET role_name='after',status='DISABLED' WHERE id=3;UPDATE applications SET status='DISABLED' WHERE id=2;UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled fixture revocation' WHERE user_id=2;INSERT INTO user_role(user_id,role_id) VALUES(2,6)`)
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	conn.Close()
	unblock()
	var out snapshotResult
	select {
	case out = <-done:
		finished = true
	case <-actor.Done():
		t.Fatal("snapshot completion")
	}
	if out.err != nil {
		t.Fatal(out.err)
	}
	res := out.res.(domain.UserRoleAssignmentPage)
	if res.Version != 1 || res.Page.Total != 4 || len(res.Page.Items) != 4 {
		t.Fatal("mixed total/target", res)
	}
	for _, row := range res.Page.Items {
		if row.GrantedAt != nil || row.GrantedBy != nil || row.RoleID == 3 && (row.Name != "Editor" || row.RoleStatus != "ACTIVE" || row.ApplicationStatus != "ACTIVE") {
			t.Fatal("mixed metadata", row)
		}
	}
	if _, e = assignmentService(t, assignmentDeps(h)).ListUserRoles(actor, domain.CatalogQuery{TargetUserID: 2}); !errors.Is(e, common.ErrForbidden) {
		t.Fatal("fresh stale authority", e)
	}
	if _, e = h.f.DB.ExecContext(actor, `UPDATE role_permissions SET deleted_at=NULL WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.read')`); e != nil {
		t.Fatal(e)
	}
	fresh, e := assignmentService(t, assignmentDeps(h)).ListUserRoles(actor, domain.CatalogQuery{TargetUserID: 2})
	if e != nil || fresh.Version != 2 || fresh.Page.Total != 5 {
		t.Fatal("fresh state", fresh, e)
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("snapshot pool leak")
	}
	t.Logf("assignment RR backend=%d concurrent backend=%d", pid, other)
}
