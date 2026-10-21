//go:build postgres

package test_scenarios

import (
	"errors"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/application"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"reflect"
	"sync"
	"testing"
)

func ApplicationLists() []testutils.Scenario {
	return []testutils.Scenario{{Name: "every11 scalar directions typed continuation literal filters ties nulls projection numeric offset terminal", Run: postgresApplicationQueries}, {Name: "two physical backends preserve current authority target total and metadata in one snapshot", Run: postgresApplicationSnapshot}}
}
func postgresApplicationQueries(t *testing.T) {
	h := newApplicationHarness(t)
	_, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO applications(code,name,status,is_system) VALUES('clm','Equal','DISABLED',true),('erp','Equal','ACTIVE',false),('literal','literal %_\ value','ACTIVE',false),('ungranted','Absent','ACTIVE',false);INSERT INTO user_application_access(user_id,application_id,granted_by,granted_at) VALUES(2,1,NULL,'2026-01-01'),(2,3,1,'2026-01-01'),(2,4,2,'2026-01-02');INSERT INTO user_application_access(user_id,application_id,status,granted_by,granted_at,revoked_by,revoked_at,revoke_reason) VALUES(2,5,'REVOKED',1,'2026-01-01',2,'2026-01-03','earlier review');UPDATE user_application_access SET granted_at='2026-01-01' WHERE application_id=2;`)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := applicationDeps(h).Snapshots.BeginSnapshot(h.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	r := reader.NewApplicationAccessReader(tx)
	checkCatalogSorts(t, h.ctx, domain.UserApplicationAccessCatalog, r.ListAccess, domain.CatalogQuery{TargetUserID: 2})
	for _, c := range []struct {
		q   domain.CatalogQuery
		ids []uint64
	}{
		{domain.CatalogQuery{ApplicationCode: "sales"}, []uint64{2}},
		{domain.CatalogQuery{Statuses: []string{"REVOKED"}}, []uint64{5}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Keyword: `%_\`}}, []uint64{5}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Keyword: "literal", Fields: []string{"name"}, Mode: orm.SearchModePrefix}}, []uint64{5}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Keyword: "IAM", Fields: []string{"application_code"}}}, []uint64{1}},
		{domain.CatalogQuery{ApplicationCode: "ungranted"}, []uint64{}},
		{domain.CatalogQuery{Page: 2, Limit: 2}, []uint64{3, 4}},
		{domain.CatalogQuery{LastID: "3", Limit: 2}, []uint64{4, 5}},
	} {
		c.q.TargetUserID = 2
		page, e := r.ListAccess(h.ctx, c.q)
		if e != nil {
			t.Fatal(e)
		}
		ids := []uint64{}
		for _, row := range page.Items {
			ids = append(ids, row.ApplicationID)
		}
		if !reflect.DeepEqual(ids, c.ids) {
			t.Fatal("query", c.q, ids, c.ids)
		}
	}
	first, e := r.ListAccess(h.ctx, domain.CatalogQuery{TargetUserID: 2, Limit: 1, Sort: []orm.SortField{{Field: "name"}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.ListAccess(h.ctx, domain.CatalogQuery{TargetUserID: 1, Limit: 1, Sort: []orm.SortField{{Field: "name"}}, LastID: first.NextCursor}); !errors.Is(e, common.ErrBadRequest) {
		t.Fatal("cross target", e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
}

func postgresApplicationSnapshot(t *testing.T) {
	h := newApplicationHarness(t)
	actor := managementPrincipal(t, h.managementHarness, h.admin.Token, userv1.UserService_ListUserApplicationAccess_FullMethodName)
	d := applicationDeps(h)
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
				t.Error("snapshot cleanup")
			}
		}
	}()
	pid := 0
	d.Bind = func(tx domain.Transaction) application.Repositories {
		pid = transactionBackend(t, tx)
		r := application.BindRepositories(tx)
		r.Users = assignmentTargetBarrier{r.Users, ready, release}
		return r
	}
	svc := applicationService(t, d)
	go func() {
		out, e := svc.ListUserApplicationAccess(actor, domain.CatalogQuery{TargetUserID: 2})
		done <- snapshotResult{out, e}
	}()
	select {
	case <-ready:
	case r := <-done:
		finished = true
		t.Fatal("early", r.err)
	case <-actor.Done():
		t.Fatal("deadline")
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
	_, e = tx.ExecContext(actor, `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2;UPDATE auth SET version=version+1,status='SUSPENDED' WHERE id=2;UPDATE applications SET name='After',status='DISABLED' WHERE id=2;UPDATE user_application_access SET status='REVOKED',revoked_by=1,revoked_at=CURRENT_TIMESTAMP,revoke_reason='Concurrent change' WHERE user_id=2;INSERT INTO user_application_access(user_id,application_id) VALUES(2,1)`)
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
		t.Fatal("completion")
	}
	if out.err != nil {
		t.Fatal(out.err)
	}
	res := out.res.(domain.ApplicationAccessPage)
	if res.Version != 1 || res.Page.Total != 1 || len(res.Page.Items) != 1 || res.Page.Items[0].Name != "Sales" || res.Page.Items[0].Status != "ACTIVE" || res.Page.Items[0].ApplicationStatus != "ACTIVE" || res.Page.Items[0].RevokedAt != nil {
		t.Fatal("mixed snapshot", res)
	}
	if _, e = applicationService(t, applicationDeps(h)).ListUserApplicationAccess(actor, domain.CatalogQuery{TargetUserID: 2}); !errors.Is(e, common.ErrForbidden) {
		t.Fatal("fresh stale authority", e)
	}
	if _, e = h.f.DB.ExecContext(actor, `UPDATE role_permissions SET deleted_at=NULL WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.read')`); e != nil {
		t.Fatal(e)
	}
	fresh, e := applicationService(t, applicationDeps(h)).ListUserApplicationAccess(actor, domain.CatalogQuery{TargetUserID: 2})
	if e != nil || fresh.Version != 2 || fresh.Page.Total != 2 {
		t.Fatal("fresh", fresh, e)
	}
	assertAssignmentLockReuse(t, h)
	t.Logf("access RR backend=%d concurrent backend=%d", pid, other)
}
