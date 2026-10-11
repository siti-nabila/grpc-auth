//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"fmt"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	userfeature "github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/protobuf/types/known/timestamppb"
	"reflect"
	"testing"
	"time"
)

func UserListLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "all public user sort fields keyset filters full active collections and unique totals", Run: postgresUserLists}, {Name: "ListUsers authority total page and collections share primary RR snapshot", Run: postgresUserListSnapshot}}
}
func userListFixture(t *testing.T) (context.Context, *fixtures.Schema, jwt.VerifiedClaims) {
	t.Helper()
	ctx, f := loginFixture(t, false)
	initial, e := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.DB.ExecContext(ctx, `
 INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.read','fixture');
 INSERT INTO role(role_name,role_code,application_id,code) VALUES('Reader',2,1,'iam.reader'),('Auditor',3,1,'iam.auditor'),('Writer',4,1,'iam.writer'),('Hidden',5,1,'iam.hidden');
 UPDATE role SET status='DISABLED' WHERE id=5;
 INSERT INTO role_permissions VALUES(2,1);
 INSERT INTO user_role(user_id,role_id) VALUES(1,2);
 INSERT INTO auth(email,password,status,version,created_at) VALUES
 ('alice@example.test','fixture-not-a-login-hash','ACTIVE',2,'2026-01-02'),
 ('alicia@example.test','fixture-not-a-login-hash','ACTIVE',2,'2026-01-02'),
 ('bob@example.test','fixture-not-a-login-hash','SUSPENDED',3,'2026-01-03'),
 ('carol@example.test','fixture-not-a-login-hash','PENDING',1,'2026-01-01'),
 ('missing-profile@example.test','fixture-not-a-login-hash','DEACTIVATED',1,'2026-01-04');
 UPDATE auth SET created_at='2026-01-01' WHERE id=1;
 INSERT INTO profile(user_id,name,address,phone) VALUES(2,'Alice','A','123'),(3,'Alice','A',NULL),(4,'Bob','', '123'),(5,NULL,NULL,NULL);
 INSERT INTO user_profile_search SELECT p.id,to_tsvector('simple',coalesce(name,'') || ' ' || a.email),lower(coalesce(name,'') || ' ' || a.email) FROM profile p JOIN auth a ON a.id=p.user_id WHERE p.user_id IN(2,3,4);
 INSERT INTO user_role(user_id,role_id) VALUES(2,3),(2,4),(3,3),(4,5);
 INSERT INTO user_role(user_id,role_id,status,revoked_at,revoke_reason) VALUES(5,3,'REVOKED',CURRENT_TIMESTAMP,'fixture revoked');
 INSERT INTO applications(code,name) VALUES('clm','Contracts'),('hidden','Hidden application');
 UPDATE applications SET status='DISABLED' WHERE id=3;
 INSERT INTO user_application_access(user_id,application_id) VALUES(1,1),(2,1),(2,2),(3,2),(4,3);
 INSERT INTO user_application_access(user_id,application_id,status,revoked_at,revoke_reason) VALUES(5,2,'REVOKED',CURRENT_TIMESTAMP,'fixture revoked');`)
	if e != nil {
		t.Fatal("user list fixture", e)
	}
	ctx = shared.CallerContext(t, ctx, userv1.UserService_ListUsers_FullMethodName)
	claims, e := loginSecurity(t, func() time.Time { return shared.ServiceTime }).Verifier.Verify(initial.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	return ctx, f, claims
}
func postgresUserLists(t *testing.T) {
	ctx, f, claims := userListFixture(t)
	provider := database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig())
	read := func(t *testing.T, req *userv1.ListUsersRequest) *userv1.ListUsersResponse {
		t.Helper()
		q, e := handler.UserListRequestFromProto(req)
		if e != nil {
			t.Fatal(e)
		}
		page, e := userfeature.ReadUsersSnapshot(ctx, claims, q, provider, shared.ServiceTime)
		if e != nil {
			t.Fatal("user page", e)
		}
		return handler.ListUsersResponseFromPage(page)
	}
	expressions := map[string]string{"id": "a.id", "email": "a.email", "name": "p.name", "address": "p.address", "phone": "p.phone", "status": "a.status", "version": "a.version", "created_at": "a.created_at", "roles": "(SELECT MIN(r.role_name) FROM user_role ur JOIN role r ON r.id=ur.role_id WHERE ur.user_id=a.id AND ur.status='ACTIVE' AND r.status='ACTIVE')", "application_access": "(SELECT MIN(app.name) FROM user_application_access ua JOIN applications app ON app.id=ua.application_id WHERE ua.user_id=a.id AND ua.status='ACTIVE' AND app.status='ACTIVE')"}
	sorts := [][]*paginatorv1.Sort{}
	for _, field := range []string{"id", "name", "email", "address", "phone", "status", "version", "created_at", "roles", "application_access"} {
		for _, desc := range []bool{false, true} {
			sorts = append(sorts, []*paginatorv1.Sort{{Field: field, Desc: desc}})
		}
	}
	sorts = append(sorts, []*paginatorv1.Sort{{Field: "name", Desc: true}, {Field: "version"}, {Field: "id", Desc: true}}, []*paginatorv1.Sort{{Field: "roles", Desc: true}, {Field: "application_access"}, {Field: "phone", Desc: true}}, []*paginatorv1.Sort{{Field: "status"}, {Field: "created_at", Desc: true}}, nil, []*paginatorv1.Sort{{Field: "auth_id", Desc: true}})
	for n, sorts := range sorts {
		t.Run(fmt.Sprintf("sort %d", n), func(t *testing.T) {
			order := ""
			hasID := false
			for _, s := range sorts {
				field := s.Field
				if field == "auth_id" {
					field = "id"
				}
				if field == "id" {
					hasID = true
				}
				dir := "ASC"
				if s.Desc {
					dir = "DESC"
				}
				if order != "" {
					order += ", "
				}
				order += expressions[field] + " " + dir + " NULLS LAST"
			}
			if !hasID {
				if order != "" {
					order += ", "
				}
				order += "a.id ASC"
			}
			rs, e := f.DB.QueryContext(ctx, "SELECT a.id FROM auth a LEFT JOIN profile p ON p.user_id=a.id ORDER BY "+order)
			if e != nil {
				t.Fatal(e)
			}
			expected := []int32{}
			for rs.Next() {
				var id int32
				if e = rs.Scan(&id); e != nil {
					t.Fatal(e)
				}
				expected = append(expected, id)
			}
			if e = rs.Err(); e != nil {
				t.Fatal(e)
			}
			rs.Close()
			q := &paginatorv1.PageQuery{Page: 1, Limit: 2, Sort: sorts}
			got := []int32{}
			for i := 0; i < 5; i++ {
				res := read(t, &userv1.ListUsersRequest{Query: q})
				if res.Total != 6 || res.TotalPages != 3 {
					t.Fatal("total includes all unique users before cursor", res.Total, res.TotalPages)
				}
				for _, r := range res.Items {
					got = append(got, r.Id)
					if r.CreatedAt == nil || r.Version < 1 || r.Status == "" {
						t.Fatal("unmapped scalar", r.Id)
					}
					for _, role := range r.Roles {
						if role.Status != "ACTIVE" || role.RoleStatus != "ACTIVE" {
							t.Fatal("inactive role leaked")
						}
					}
					for _, app := range r.ApplicationAccess {
						if app.Status != "ACTIVE" || app.ApplicationStatus != "ACTIVE" {
							t.Fatal("inactive app leaked")
						}
					}
					if r.Id == 2 && (len(r.Roles) != 2 || len(r.ApplicationAccess) != 2 || r.Roles[0].Name != "Auditor" || r.ApplicationAccess[0].Name != "Contracts") {
						t.Fatal("full independent collections", r)
					}
					if r.Id >= 4 && (len(r.Roles) != 0 || len(r.ApplicationAccess) != 0) {
						t.Fatal("hidden/revoked collection leaked", r.Id)
					}
				}
				if res.NextCursor == "" {
					t.Fatal("missing next cursor")
				}
				q.LastId = res.NextCursor
				q.Page++
				if !res.HasNext {
					break
				}
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatal("keyset order/duplicate/missing", got, expected)
			}
			later := read(t, &userv1.ListUsersRequest{Query: q})
			if later.Total != 6 || len(later.Items) != 0 || !later.HasPrev {
				t.Fatal("empty later total", later)
			}
		})
	}
	for _, c := range []struct {
		name    string
		filter  *userv1.UserFilter
		keyword string
		ids     []int32
	}{
		{"role OR", &userv1.UserFilter{RoleCodes: []uint64{1, 3}}, "", []int32{1, 2, 3}},
		{"created range", &userv1.UserFilter{CreatedFrom: timestamppb.New(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)), CreatedTo: timestamppb.New(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))}, "", []int32{2, 3}},
		{"role search AND", &userv1.UserFilter{RoleCodes: []uint64{1, 3}}, "Alice", []int32{2, 3}},
		{"role email search AND", &userv1.UserFilter{RoleCodes: []uint64{1, 3}}, "alicia@example.test", []int32{3}},
		{"application status and role", &userv1.UserFilter{RoleCodes: []uint64{1, 3}, ApplicationCode: "clm", Statuses: []string{"ACTIVE"}}, "", []int32{2, 3}},
		{"status no search index required", &userv1.UserFilter{Statuses: []string{"PENDING", "DEACTIVATED"}}, "", []int32{5, 6}},
		{"inactive role", &userv1.UserFilter{RoleCodes: []uint64{5}}, "", []int32{}},
		{"inactive app", &userv1.UserFilter{ApplicationCode: "hidden"}, "", []int32{}},
		{"filter does not trim collections", &userv1.UserFilter{RoleCodes: []uint64{4}, ApplicationCode: "clm"}, "", []int32{2}},
	} {
		t.Run(c.name, func(t *testing.T) {
			q := &paginatorv1.PageQuery{Limit: 100}
			if c.keyword != "" {
				q.Search = &paginatorv1.Search{Keyword: c.keyword}
			}
			res := read(t, &userv1.ListUsersRequest{Query: q, Filter: c.filter})
			ids := []int32{}
			for _, r := range res.Items {
				ids = append(ids, r.Id)
				if r.Id == 2 && (len(r.Roles) != 2 || len(r.ApplicationAccess) != 2) {
					t.Fatal("filter trimmed collection")
				}
			}
			if !reflect.DeepEqual(ids, c.ids) || int(res.Total) != len(c.ids) {
				t.Fatal("unique filter/search total", ids, res.Total, c.ids)
			}
		})
	}
	projected := read(t, &userv1.ListUsersRequest{Query: &paginatorv1.PageQuery{Limit: 1, Fields: []string{"email"}, Sort: []*paginatorv1.Sort{{Field: "name"}}}})
	if projected.Items[0].Email == "" || projected.Items[0].Id != 0 || projected.Items[0].Name != "" || projected.Items[0].CreatedAt != nil || projected.NextCursor == "" {
		t.Fatal("projection lost cursor/leaked unselected fields", projected)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("user list pool leak")
	}
}
func postgresUserListSnapshot(t *testing.T) {
	ctx, f, claims := userListFixture(t)
	provider := database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig())
	ready, release := make(chan struct{}), make(chan struct{})
	barrier := &snapshotBarrier{provider: provider, ready: ready, release: release}
	done := make(chan snapshotResult, 1)
	finished, released := false, false
	defer func() {
		if !released {
			close(release)
		}
		if !finished {
			select {
			case <-done:
			case <-ctx.Done():
				t.Error("snapshot worker cleanup")
			}
		}
	}()
	go func() {
		r, e := userfeature.ReadUsersSnapshot(ctx, claims, domain.UserListRequest{}, barrier, shared.ServiceTime)
		done <- snapshotResult{r, e}
	}()
	select {
	case <-ready:
	case r := <-done:
		finished = true
		t.Fatal("snapshot before barrier", r.err)
	case <-ctx.Done():
		t.Fatal("snapshot timeout")
	}
	conn, e := f.DB.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	pid := physicalIdentity(t, ctx, conn, f.Name)
	if barrier.pid == 0 || barrier.pid == pid {
		t.Fatal("distinct primary backends missing")
	}
	tx, e := conn.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id=2; UPDATE role SET role_name='Changed after snapshot' WHERE id=3; UPDATE profile SET name='Changed after snapshot' WHERE user_id=2; INSERT INTO auth(email,password) VALUES('new-after-snapshot@example.test','fixture-not-a-login-hash')`); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	close(release)
	released = true
	var result snapshotResult
	select {
	case result = <-done:
		finished = true
	case <-ctx.Done():
		t.Fatal("snapshot completion timeout")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	// JSON-free domain assertion checks total, page and separately loaded roles.
	page := result.res.(orm.PageData[domain.UserSearchRow])
	if page.Total != 6 || len(page.Items) != 6 || page.Items[1].Name != "Alice" || len(page.Items[1].Roles) != 2 || page.Items[1].Roles[0].Name != "Auditor" {
		t.Fatal("mixed authority/page/count/collection snapshots", page.Total)
	}
	fresh, e := userfeature.ReadUsersSnapshot(ctx, claims, domain.UserListRequest{}, provider, shared.ServiceTime)
	if !errors.Is(e, common.ErrForbidden) || len(fresh.Items) != 0 {
		t.Fatal("stale authorization", e)
	}
	t.Logf("ListUsers RR backend=%d policy/data backend=%d", barrier.pid, pid)
	conn.Close()
	if f.DB.Stats().InUse != 0 {
		t.Fatal("snapshot pool leak")
	}
}
