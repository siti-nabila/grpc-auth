//go:build postgres

package test_scenarios

import (
	"database/sql"
	"errors"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"sort"
	"strings"
	"testing"
	"time"
)

func SessionListLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "real session pagination supports every direction equal timestamps and null-last", Run: postgresSessionLists}, {Name: "session read authority and data use one RR snapshot and next request sees permission loss", Run: postgresSessionListSnapshot}}
}

func sessionListService(t *testing.T, f *fixtures.Schema) session.ListService {
	t.Helper()
	s, e := session.NewListService(session.ListDependencies{Snapshots: database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig()), Bind: session.BindListRepositories, Now: func() time.Time { return shared.ServiceTime }})
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func postgresSessionLists(t *testing.T) {
	ctx, f := loginFixture(t, false)
	// Current application metadata makes last_seen_at NOT NULL. Relax it only
	// in this owned test schema to exercise the required defensive null keyset;
	// this is not an application schema prerequisite or migration.
	if _, e := f.DB.ExecContext(ctx, `ALTER TABLE sessions ALTER COLUMN last_seen_at DROP NOT NULL`); e != nil {
		t.Fatal("owned nullable boundary setup", e)
	}
	var token string
	for n := 0; n < 5; n++ {
		r, e := loginService(t, loginDependencies(t, ctx, f, byte(n*40))).Login(ctx, loginRequest())
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 {
			token = r.AccessToken
		}
		created := shared.ServiceTime.Add(-time.Duration(n/2) * time.Minute)
		expiry := shared.ServiceTime.Add(time.Duration(5+n/2) * time.Hour)
		var seen any
		if n < 3 {
			seen = shared.ServiceTime.Add(-time.Duration(n/2) * time.Minute)
		}
		if _, e = f.DB.ExecContext(ctx, `UPDATE sessions SET created_at=$2,expires_at=$3,last_seen_at=$4,device_label=$5 WHERE id=$1`, r.SessionID, created, expiry, seen, "display-device"); e != nil {
			t.Fatal("session display setup", e)
		}
	}
	actor := selfActor(t, ctx, f, token, sessionv1.SessionService_ListMySessions_FullMethodName)
	svc := sessionListService(t, f)
	type row struct {
		id              string
		created, expiry time.Time
		seen            sql.NullTime
	}
	rows, e := f.DB.QueryContext(ctx, `SELECT id,created_at,expires_at,last_seen_at FROM sessions`)
	if e != nil {
		t.Fatal(e)
	}
	all := []row{}
	for rows.Next() {
		var r row
		if e = rows.Scan(&r.id, &r.created, &r.expiry, &r.seen); e != nil {
			t.Fatal(e)
		}
		all = append(all, r)
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	_ = rows.Close()
	for _, field := range []domain.SessionSortField{domain.SessionSortCreatedAt, domain.SessionSortLastSeenAt, domain.SessionSortExpiresAt} {
		for _, desc := range []bool{false, true} {
			t.Run(string(field)+map[bool]string{true: " DESC", false: " ASC"}[desc], func(t *testing.T) {
				expected := append([]row{}, all...)
				value := func(r row) (time.Time, bool) {
					switch field {
					case domain.SessionSortCreatedAt:
						return r.created, true
					case domain.SessionSortExpiresAt:
						return r.expiry, true
					default:
						return r.seen.Time, r.seen.Valid
					}
				}
				sort.Slice(expected, func(i, j int) bool {
					a, av := value(expected[i])
					b, bv := value(expected[j])
					if av != bv {
						return av
					}
					cmp := 0
					if av {
						cmp = a.Compare(b)
					}
					if cmp == 0 {
						cmp = strings.Compare(expected[i].id, expected[j].id)
					}
					if desc {
						return cmp > 0
					}
					return cmp < 0
				})
				q := session.ListInput{Field: field, Desc: desc, Limit: 2}
				got := []string{}
				current := 0
				for n := 0; n < 4; n++ {
					q.Page = n + 1
					p, e := svc.ListMySessions(actor, q)
					if e != nil {
						t.Fatal("session page", e)
					}
					if p.Total != 5 || p.HasPrev != (n > 0) || len(p.Items) > 2 {
						t.Fatal("unique total/page metadata")
					}
					for _, item := range p.Items {
						if item.Current {
							current++
						}
						if item.Session.UserID != 1 {
							t.Fatal("owner leak")
						}
						got = append(got, item.Session.ID)
					}
					q.Cursor = p.NextCursor
					if !p.HasNext {
						break
					}
				}
				if len(got) != 5 || current != 1 {
					t.Fatal("pagination missing/duplicating session", got, current)
				}
				for n, id := range got {
					if id != expected[n].id {
						t.Fatal("wrong keyset order", got)
					}
				}
				empty, e := svc.ListMySessions(actor, q)
				if e != nil || empty.Total != 5 || len(empty.Items) != 0 || empty.HasNext || empty.NextCursor != "" {
					t.Fatal("empty later-page count", e)
				}
			})
		}
	}
	// Default ordering is created_at DESC, id DESC and limit20/page1.
	p, e := svc.ListMySessions(actor, session.ListInput{})
	if e != nil || p.Page != 1 || p.Limit != 20 || len(p.Items) != 5 {
		t.Fatal("default list", e)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("session list pool leak")
	}
}

func postgresSessionListSnapshot(t *testing.T) {
	ctx, f := loginFixture(t, false)
	initial, e := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.DB.ExecContext(ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.session.read','fixture'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Session Reader',2,1,'iam.session_reader'); INSERT INTO role_permissions(role_id,permission_id) VALUES(2,1); INSERT INTO user_role(user_id,role_id) VALUES(1,2)`); e != nil {
		t.Fatal("permission setup", e)
	}
	actor := selfActor(t, ctx, f, initial.AccessToken, sessionv1.SessionService_ListUserSessions_FullMethodName)
	ready, release := make(chan struct{}), make(chan struct{})
	provider := database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig())
	barrier := &snapshotBarrier{provider: provider, ready: ready, release: release}
	svc, e := session.NewListService(session.ListDependencies{Snapshots: barrier, Bind: session.BindListRepositories, Now: func() time.Time { return shared.ServiceTime }})
	if e != nil {
		t.Fatal(e)
	}
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
				t.Error("list snapshot worker cleanup")
			}
		}
	}()
	go func() { r, e := svc.ListUserSessions(actor, 1, session.ListInput{}); done <- snapshotResult{r, e} }()
	select {
	case <-ready:
	case r := <-done:
		finished = true
		t.Fatal("snapshot failed before barrier", r.err)
	case <-ctx.Done():
		t.Fatal("snapshot timeout")
	}
	conn, e := f.DB.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	pid := physicalIdentity(t, ctx, conn, f.Name)
	if pid == barrier.pid || barrier.pid == 0 {
		t.Fatal("missing distinct snapshot backends")
	}
	tx, e := conn.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2`); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.ExecContext(ctx, `UPDATE sessions SET device_label='committed-later' WHERE id=$1`, initial.SessionID); e != nil {
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
		t.Fatal("snapshot result timeout")
	}
	if result.err != nil {
		t.Fatal("prior snapshot lost permission", result.err)
	}
	old := result.res.(*session.ListResult)
	if len(old.Items) != 1 || old.Items[0].Session.DeviceLabel.Valid || old.Total != 1 {
		t.Fatal("authority/data mixed snapshots")
	}
	fresh := sessionListService(t, f)
	for _, target := range []uint64{1, 42} {
		if r, e := fresh.ListUserSessions(actor, target, session.ListInput{}); r != nil || !errors.Is(e, common.ErrForbidden) {
			t.Fatal("next snapshot retained revoked permission/empty bypass", e)
		}
	}
	t.Logf("session list RR backend=%d policy/data commit backend=%d", barrier.pid, pid)
	if e = conn.Close(); e != nil || f.DB.Stats().InUse != 0 {
		t.Fatal("list snapshot pool leak")
	}
}
