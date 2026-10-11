package test_scenarios

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	repomocks "github.com/siti-nabila/grpc-auth/tests/repositories/transaction/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func SessionLifecyclePersistence() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "safe session list uses allowed timestamp keys and UUID tie breakers with total before cursor", Run: sessionListPages},
		{Name: "session discovery reads owner binding and canonical sorted IDs on supplied transaction", Run: sessionDiscovery},
		{Name: "single global and refresh revocation preserve conditional row outcomes and context", Run: sessionRevocationWrites},
		{Name: "session lifecycle read validation and dependency failures fail closed", Run: sessionLifecycleReadFailures},
	}
}

var safeSessionColumns = []string{"id", "user_id", "status", "created_at", "last_seen_at", "expires_at", "revoked_at", "device_label", "user_agent", "ip_address"}

func safeSessionRow(id string) []driver.Value {
	return []driver.Value{id, int64(42), "ACTIVE", fixtures.ServiceTime, fixtures.ServiceTime, fixtures.ServiceTime.Add(time.Hour), nil, "Browser", "Bounded agent", "192.0.2.1"}
}

func sessionListPages(t *testing.T) {
	for _, field := range []domain.SessionSortField{domain.SessionSortCreatedAt, domain.SessionSortLastSeenAt, domain.SessionSortExpiresAt} {
		for _, desc := range []bool{false, true} {
			for _, after := range []string{"none", "value", "null"} {
				if after == "null" && field != domain.SessionSortLastSeenAt {
					continue
				}
				t.Run(fmt.Sprintf("%s desc=%v after=%s", field, desc, after), func(t *testing.T) {
					ctx := context.WithValue(context.Background(), struct{}{}, "session-page-context")
					q := domain.SessionListQuery{UserID: 42, Field: field, Desc: desc, Limit: 1}
					op, dir := ">", "ASC"
					if desc {
						op, dir = "<", "DESC"
					}
					args := []driver.Value{int64(42)}
					condition := "user_id = $1"
					if after != "none" {
						q.After = &domain.SessionListCursor{ID: fixtures.SessionID, At: sql.NullTime{Time: fixtures.ServiceTime, Valid: after == "value"}}
						if after == "null" {
							condition += " AND " + string(field) + " IS NULL AND id " + op + " $2"
							args = append(args, fixtures.SessionID)
						} else {
							condition += " AND (" + string(field) + " " + op + " $2 OR (" + string(field) + " = $3 AND id " + op + " $4) OR " + string(field) + " IS NULL)"
							args = append(args, fixtures.ServiceTime, fixtures.ServiceTime, fixtures.SessionID)
						}
					}
					condition += " ORDER BY " + string(field) + " " + dir + " NULLS LAST, id " + dir + " LIMIT 2"
					first := safeSessionRow(fixtures.SessionID)
					second := safeSessionRow(fixtures.AccessTokenID)
					if after == "null" {
						first[4] = nil
						second[4] = nil
					}
					mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{
						{Op: "begin"},
						{Op: "query", SQL: "SELECT COUNT(*) AS total FROM sessions WHERE user_id = $1", Args: []driver.Value{int64(42)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(9)}}},
						{Op: "query", SQL: condition, Args: args, Columns: safeSessionColumns, Rows: [][]driver.Value{first, second}},
						{Op: "rollback"},
					}}
					db := mock.Open()
					defer db.Close()
					tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					page, err := reader.NewSessionLifecycleReader(tx).ListPage(ctx, q)
					if err != nil || page.Total != 9 || !page.HasNext || len(page.Items) != 1 || page.Items[0].UserID != 42 || page.Items[0].ID != fixtures.SessionID || page.Items[0].DeviceLabel.String != "Browser" || page.Items[0].LastSeenAt.Valid != (after != "null") {
						t.Fatal("safe page mapping", page, err)
					}
					if err = tx.Rollback(); err != nil {
						t.Fatal(err)
					}
					if err = mock.Verify(); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func sessionDiscovery(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "session-discovery-context")
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{
		{Op: "begin"},
		{Op: "query", SQL: "SELECT id, user_id, status, created_at, last_seen_at, expires_at, revoked_at, device_label, user_agent, ip_address FROM sessions WHERE id = $1 LIMIT 1", Args: []driver.Value{fixtures.SessionID}, Columns: safeSessionColumns, Rows: [][]driver.Value{safeSessionRow(fixtures.SessionID)}},
		{Op: "query", SQL: "SELECT id FROM sessions WHERE user_id = $1 ORDER BY id ASC", Args: []driver.Value{int64(42)}, Columns: []string{"id"}, Rows: [][]driver.Value{{fixtures.SessionID}, {fixtures.AccessTokenID}}},
		{Op: "rollback"},
	}}
	db := mock.Open()
	defer db.Close()
	tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := reader.NewSessionLifecycleReader(tx)
	got, err := r.GetByID(ctx, fixtures.SessionID)
	if err != nil || got.ID != fixtures.SessionID || got.UserID != 42 {
		t.Fatal("owner missing", err)
	}
	ids, err := r.ListIDs(ctx, 42)
	if err != nil || len(ids) != 2 || ids[0] != fixtures.SessionID || ids[1] != fixtures.AccessTokenID {
		t.Fatal("IDs lost", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = mock.Verify(); err != nil {
		t.Fatal(err)
	}
}

func sessionRevocationWrites(t *testing.T) {
	cause := errors.New("controlled revocation failure")
	for _, method := range []string{"single", "global", "family", "user refresh", "session refresh"} {
		for _, kind := range []string{"one", "zero", "many", "failure", "result failure", "cancelled"} {
			t.Run(method+" "+kind, func(t *testing.T) {
				ctx := context.WithValue(context.Background(), struct{}{}, "revocation-context")
				count := int64(1)
				if kind == "zero" {
					count = 0
				}
				if kind == "many" {
					count = 3
				}
				var result driver.Result = driver.RowsAffected(count)
				var failure, want error
				if kind == "failure" {
					failure = cause
					want = cause
				}
				if kind == "cancelled" {
					failure = context.Canceled
					want = context.Canceled
				}
				if kind == "result failure" {
					result = repomocks.AffectedFailure{Err: cause}
					want = cause
				}
				if kind == "many" && method == "single" {
					want = common.ErrInternalServerError
				}
				args := []driver.Value{fixtures.ServiceTime}
				where := ""
				req := domain.SessionRevocation{UserID: 42, ActorID: 43, At: fixtures.ServiceTime, Reason: types.AuditReason("TEST_REVOKED")}
				switch method {
				case "single", "global":
					args = []driver.Value{"REVOKED", fixtures.ServiceTime, int64(43), "TEST_REVOKED", int64(42), "ACTIVE"}
					where = "UPDATE sessions SET status = $1, revoked_at = $2, revoked_by = $3, revoke_reason = $4 WHERE user_id = $5 AND status = $6"
					if method == "single" {
						req.SessionID = fixtures.SessionID
						args = append(args, fixtures.SessionID)
						where += " AND id = $7"
					}
				case "session refresh":
					where = "WHERE session_id = $2 AND revoked_at IS NULL"
					args = []driver.Value{fixtures.ServiceTime, fixtures.SessionID}
				case "family":
					where = "WHERE session_id = $2 AND family_id = $3 AND revoked_at IS NULL"
					args = append(args, fixtures.SessionID, fixtures.AccessTokenID)
				case "user refresh":
					where = "WHERE session_id IN (SELECT id FROM sessions WHERE user_id = $2) AND revoked_at IS NULL"
					args = append(args, int64(42))
				}
				mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, {Op: "exec", SQL: where, Args: args, Result: result, Error: failure}, {Op: "rollback"}}}
				db := mock.Open()
				defer db.Close()
				tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var got int64
				switch method {
				case "single":
					var changed bool
					changed, err = writer.NewSessionRevocationWriter(tx).RevokeSession(ctx, req)
					if changed {
						got = 1
					}
				case "global":
					got, err = writer.NewSessionRevocationWriter(tx).RevokeUserSessions(ctx, req)
				case "session refresh":
					got, err = writer.NewRefreshRevocationWriter(tx).RevokeSessionRefresh(ctx, fixtures.SessionID, fixtures.ServiceTime)
				case "family":
					got, err = writer.NewRefreshRevocationWriter(tx).RevokeFamily(ctx, domain.RefreshFamilyRevocation{SessionID: fixtures.SessionID, FamilyID: fixtures.AccessTokenID, At: fixtures.ServiceTime})
				case "user refresh":
					got, err = writer.NewRefreshRevocationWriter(tx).RevokeUserRefresh(ctx, 42, fixtures.ServiceTime)
				}
				if want == nil && (err != nil || got != count) || want != nil && !errors.Is(err, want) {
					t.Fatal("row outcome", got, err)
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				if err = mock.Verify(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func sessionLifecycleReadFailures(t *testing.T) {
	for _, kind := range []string{"count failure", "page failure", "empty", "missing target"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			cause := errors.New("controlled list read failure")
			count := mocks.SQLStep{Op: "query", SQL: "COUNT(*)", Args: []driver.Value{int64(42)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(0)}}}
			page := mocks.SQLStep{Op: "query", SQL: "ORDER BY created_at DESC NULLS LAST, id DESC LIMIT 21", Args: []driver.Value{int64(42)}, Columns: safeSessionColumns}
			steps := []mocks.SQLStep{{Op: "begin"}, count, page, {Op: "rollback"}}
			var want error
			if kind == "count failure" {
				steps[1].Error = cause
				steps = append(steps[:2], mocks.SQLStep{Op: "rollback"})
				want = cause
			}
			if kind == "page failure" {
				steps[2].Error = cause
				want = cause
			}
			if kind == "missing target" {
				steps = []mocks.SQLStep{{Op: "begin"}, {Op: "query", SQL: "WHERE id =", Args: []driver.Value{fixtures.SessionID}, Columns: safeSessionColumns}, {Op: "rollback"}}
				want = auth.ErrNotFound
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			defer db.Close()
			tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			r := reader.NewSessionLifecycleReader(tx)
			if kind == "missing target" {
				_, err = r.GetByID(ctx, fixtures.SessionID)
			} else {
				var got domain.SessionListPage
				got, err = r.ListPage(ctx, domain.SessionListQuery{UserID: 42, Field: domain.SessionSortCreatedAt, Desc: true, Limit: 20})
				if kind == "empty" && (got.Total != 0 || len(got.Items) != 0 || got.HasNext) {
					t.Fatal("empty page", got)
				}
			}
			if want == nil && err != nil || want != nil && !errors.Is(err, want) {
				t.Fatal("read failure mapping", err)
			}
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err = mock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := reader.NewSessionLifecycleReader(nil).ListIDs(context.Background(), 42); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing handle fallback")
	}
	ctx := context.Background()
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, {Op: "rollback"}}}
	db := mock.Open()
	defer db.Close()
	tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := writer.NewSessionRevocationWriter(tx)
	valid := domain.SessionRevocation{UserID: 42, ActorID: 43, SessionID: fixtures.SessionID, At: fixtures.ServiceTime, Reason: types.AuditReason("TEST_REVOKED")}
	for _, mutate := range []func(*domain.SessionRevocation){func(r *domain.SessionRevocation) { r.UserID = 0 }, func(r *domain.SessionRevocation) { r.ActorID = 2147483648 }, func(r *domain.SessionRevocation) { r.SessionID = "bad" }, func(r *domain.SessionRevocation) { r.At = time.Time{} }, func(r *domain.SessionRevocation) { r.Reason = "  x  " }} {
		req := valid
		mutate(&req)
		if _, err := w.RevokeSession(ctx, req); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("invalid revocation reached SQL", err)
		}
	}
	missingID := valid
	missingID.SessionID = ""
	if _, err := w.RevokeSession(ctx, missingID); !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("missing target accepted")
	}
	if _, err := w.RevokeUserSessions(ctx, valid); !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("global session target accepted")
	}
	if _, err := writer.NewSessionRevocationWriter(nil).RevokeSession(ctx, valid); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing revoke handle fallback")
	}
	r := reader.NewSessionLifecycleReader(tx)
	if _, err := r.GetByID(ctx, "malformed"); !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("malformed target reached SQL")
	}
	if _, err := r.ListIDs(ctx, 0); !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("invalid owner reached SQL")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.ListIDs(cancelled, 42); !errors.Is(err, context.Canceled) {
		t.Fatal("read cancellation lost")
	}
	if _, err := w.RevokeSession(cancelled, valid); !errors.Is(err, context.Canceled) {
		t.Fatal("revoke cancellation lost")
	}
	rw := writer.NewRefreshRevocationWriter(tx)
	if _, err := rw.RevokeFamily(ctx, domain.RefreshFamilyRevocation{}); !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("invalid family reached SQL")
	}
	if _, err := rw.RevokeUserRefresh(ctx, 0, fixtures.ServiceTime); !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("invalid refresh owner reached SQL")
	}
	if _, err := rw.RevokeUserRefresh(cancelled, 42, fixtures.ServiceTime); !errors.Is(err, context.Canceled) {
		t.Fatal("refresh cancellation lost")
	}
	if _, err := writer.NewRefreshRevocationWriter(nil).RevokeUserRefresh(ctx, 42, fixtures.ServiceTime); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing refresh handle fallback")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = mock.Verify(); err != nil {
		t.Fatal(err)
	}

	base := domain.SessionListQuery{UserID: 42, Field: domain.SessionSortCreatedAt, Limit: 20}
	for _, mutate := range []func(*domain.SessionListQuery){func(q *domain.SessionListQuery) { q.UserID = 0 }, func(q *domain.SessionListQuery) { q.UserID = 2147483648 }, func(q *domain.SessionListQuery) { q.Field = "id;DROP TABLE sessions" }, func(q *domain.SessionListQuery) { q.Limit = 101 }, func(q *domain.SessionListQuery) { q.After = &domain.SessionListCursor{ID: "bad"} }, func(q *domain.SessionListQuery) { q.After = &domain.SessionListCursor{ID: fixtures.SessionID} }} {
		q := base
		mutate(&q)
		if !errors.Is(q.Validate(), common.ErrBadRequest) {
			t.Fatal("invalid list accepted")
		}
	}
}
