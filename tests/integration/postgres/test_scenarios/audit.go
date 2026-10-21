//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	auditv1 "github.com/siti-nabila/api-contracts/pb/audit/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/audit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const auditReadMethod = auditv1.AuditService_ListAuditLogs_FullMethodName

func AuditLogReads() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered audit BIGINT fixed pages exact historical filters and safe snapshots", Run: postgresAuditPages},
		{Name: "audit count page and current authority stay on one state across two physical backends", Run: postgresAuditSnapshots},
		{Name: "audit canceled panicked dependency finalization and independent attempt failures release real pool", Run: postgresAuditCleanup},
	}
}

func newAuditHarness(t *testing.T) *roleHarness {
	t.Helper()
	h := newRoleHarness(t, auditReadMethod)
	_, err := h.f.DB.ExecContext(h.ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.audit.read','Audit history'); INSERT INTO role_permissions(role_id,permission_id) SELECT 2,id FROM permissions WHERE code='iam.audit.read'`)
	if err != nil {
		t.Fatal("owned audit permission fixture", err)
	}
	_, err = h.f.DB.ExecContext(h.ctx, `INSERT INTO audit_logs(id,actor_user_id,actor_service,action,target_type,target_id,safe_after,occurred_at) VALUES
 (2147483648,NULL,'history-service','ALR_FIXTURE','user','2','{}',$1),
 (2147483649,2,'history-service','ALR_FIXTURE','user','2','{"name":""}',$1),
 (9223372036854775807,2,NULL,'ALR_FIXTURE','user','2','{}',$1)`, h.now())
	if err != nil {
		t.Fatal("owned audit fixture", err)
	}
	return h
}
func postgresAuditPages(t *testing.T) {
	h := newAuditHarness(t)
	client := auditv1.NewAuditServiceClient(h.conn)
	actor := shared.RPCContext(h.ctx, h.admin.Token)
	if _, err := h.f.DB.ExecContext(h.ctx, `UPDATE auth SET status='SUSPENDED' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	before := roleBusinessState(t, h)
	req := &auditv1.ListAuditLogsRequest{Query: &paginatorv1.PageQuery{Limit: 1}, Filter: &auditv1.AuditLogFilter{Action: "ALR_FIXTURE"}}
	first, err := client.ListAuditLogs(actor, req)
	if err != nil || first.Meta.Total != 3 || first.Meta.TotalPages != 3 || !first.Meta.HasNext || first.Items[0].Id != math.MaxInt64 || first.Items[0].ActorService != nil || first.Items[0].SafeBefore != nil || first.Items[0].SafeAfter == nil {
		t.Fatal(first, err)
	}
	req.Query.LastId = first.NextCursor
	second, err := client.ListAuditLogs(actor, req)
	if err != nil || second.Items[0].Id != 2147483649 || second.Items[0].ActorService == nil || second.Items[0].ActorUserId == nil || second.Items[0].SafeAfter.Name == nil || *second.Items[0].SafeAfter.Name != "" || !second.Meta.HasPrev {
		t.Fatal(second, err)
	}
	req.Query.LastId = second.NextCursor
	third, err := client.ListAuditLogs(actor, req)
	if err != nil || third.Items[0].Id != 2147483648 || third.Items[0].ActorUserId != nil || third.Meta.HasNext || third.NextCursor != "" {
		t.Fatal(third, err)
	}
	req.Query = &paginatorv1.PageQuery{Page: 2, Limit: 1}
	offset, err := client.ListAuditLogs(actor, req)
	if err != nil || offset.Items[0].Id != 2147483649 || offset.Meta.Total != 3 {
		t.Fatal(offset, err)
	}
	req.Query = &paginatorv1.PageQuery{Limit: 100}
	req.Filter = &auditv1.AuditLogFilter{Action: "ALR_FIXTURE", ActorUserId: proto.Uint64(2), ActorService: " history-service ", TargetType: " user ", TargetId: " 2 ", OccurredFrom: timestamppb.New(h.now()), OccurredBefore: timestamppb.New(h.now().Add(time.Microsecond))}
	both, err := client.ListAuditLogs(actor, req)
	if err != nil || both.Meta.Total != 1 || both.Items[0].Id != 2147483649 {
		t.Fatal("AND/presence/time", both, err)
	}
	for _, kind := range []string{"upper equal", "lower fractional", "upper fractional", "unknown literal"} {
		t.Run(kind, func(t *testing.T) {
			filter := &auditv1.AuditLogFilter{Action: "ALR_FIXTURE"}
			want := int64(0)
			switch kind {
			case "upper equal":
				filter.OccurredBefore = timestamppb.New(h.now())
			case "lower fractional":
				filter.OccurredFrom = timestamppb.New(h.now().Add(1))
			case "upper fractional":
				filter.OccurredBefore = timestamppb.New(h.now().Add(1))
				want = 3
			case "unknown literal":
				filter.ActorService = "%_"
			}
			h.clock.Add(int64(2 * time.Second))
			res, err := client.ListAuditLogs(actor, &auditv1.ListAuditLogsRequest{Filter: filter})
			if err != nil || res.Meta.Total != want {
				t.Fatal(res, err, want)
			}
		})
	}
	if before != roleBusinessState(t, h) {
		t.Fatal("successful audit listing changed persisted state")
	}
	for _, raw := range []string{`null`, `[]`, `{"access_token":"synthetic"}`, `{"version":-1}`} {
		t.Run(raw, func(t *testing.T) {
			if _, err := h.f.DB.ExecContext(h.ctx, `UPDATE audit_logs SET safe_after=$1::jsonb WHERE id=2147483649`, raw); err != nil {
				t.Fatal(err)
			}
			h.clock.Add(int64(2 * time.Second))
			res, err := client.ListAuditLogs(actor, &auditv1.ListAuditLogsRequest{Filter: &auditv1.AuditLogFilter{Action: "ALR_FIXTURE"}})
			if res != nil || status.Code(err) != codes.Unavailable {
				t.Fatal("unsafe snapshot fallback", res, err)
			}
		})
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("audit read leaked primary lease")
	}
}

// Block after count and before page, while retaining the feature's real tx.
type auditPageBarrier struct {
	provider domain.ReadSnapshotProvider
	ready    chan struct{}
	release  <-chan struct{}
	pid      int
}
type auditBarrierTx struct {
	domain.Transaction
	owner *auditPageBarrier
	ctx   context.Context
	reads int
}

func (p *auditPageBarrier) BeginSnapshot(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.provider.BeginSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	var row struct {
		PID int `sql:"column:pid"`
	}
	if err = tx.UseModel(domain.Auth{}).WithContext(ctx).Select("pg_backend_pid() AS pid").Limit(1).Scan(&row); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	p.pid = row.PID
	return &auditBarrierTx{Transaction: tx, owner: p, ctx: ctx}, nil
}
func (t *auditBarrierTx) UseModel(model any) *query.QueryBuilder {
	if _, ok := model.(domain.AuditRecord); ok {
		t.reads++
		if t.reads == 2 {
			close(t.owner.ready)
			select {
			case <-t.owner.release:
			case <-t.ctx.Done():
			}
		}
	}
	return t.Transaction.UseModel(model)
}
func auditServiceFor(t *testing.T, h *roleHarness, p domain.ReadSnapshotProvider) audit.Service {
	t.Helper()
	s, err := audit.NewService(audit.Dependencies{Snapshots: p, Bind: audit.BindRepositories, Now: h.now})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func postgresAuditSnapshots(t *testing.T) {
	for _, kind := range []string{"insert", "mapping", "assignment", "role", "session", "account"} {
		t.Run(kind, func(t *testing.T) {
			h := newAuditHarness(t)
			base := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
			ctx := managementPrincipal(t, h.managementHarness, h.admin.Token, auditReadMethod)
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			ready, release := make(chan struct{}), make(chan struct{})
			barrier := &auditPageBarrier{provider: base, ready: ready, release: release}
			done := make(chan snapshotResult, 1)
			released, finished := false, false
			defer func() {
				if !released {
					close(release)
				}
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("audit worker cleanup")
					}
				}
			}()
			q := domain.AuditLogQuery{Action: "ALR_FIXTURE"}
			s := auditServiceFor(t, h, barrier)
			go func() { r, e := s.ListAuditLogs(ctx, q); done <- snapshotResult{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("before count/page barrier", r.err)
			case <-ctx.Done():
				t.Fatal("audit barrier timeout")
			}
			conn, err := h.f.DB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, h.f.Name)
			if pid == barrier.pid || barrier.pid == 0 {
				t.Fatal("two physical backends required")
			}
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_service,action,target_type,target_id,occurred_at) VALUES('history-service','ALR_FIXTURE','user','2',$1)`, h.now().Add(time.Microsecond)); err != nil {
				t.Fatal(err)
			}
			statement := ""
			switch kind {
			case "mapping":
				statement = `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.audit.read')`
			case "assignment":
				statement = `UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Audit review' WHERE user_id=1 AND role_id=2`
			case "role":
				statement = `UPDATE role SET status='DISABLED' WHERE id=2`
			case "session":
				statement = `UPDATE sessions SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Audit review' WHERE user_id=1`
			case "account":
				statement = `UPDATE auth SET status='SUSPENDED',version=version+1 WHERE id=1`
			}
			if statement != "" {
				if _, err = tx.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			close(release)
			released = true
			var old snapshotResult
			select {
			case old = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("audit completion timeout")
			}
			page, ok := old.res.(orm.PageData[domain.AuditLogItem])
			if old.err != nil || !ok || page.Total != 3 || len(page.Items) != 3 {
				t.Fatal("mixed authority/count/page", old.err, page.Total)
			}
			next, err := auditServiceFor(t, h, base).ListAuditLogs(ctx, q)
			if kind == "insert" {
				if err != nil || next.Total != 4 || len(next.Items) != 4 {
					t.Fatal(next.Total, err)
				}
			} else {
				want := error(common.ErrForbidden)
				if kind == "session" || kind == "account" {
					want = auth.ErrInvalidSession
				}
				if !errors.Is(err, want) || next.Items != nil {
					t.Fatal("next snapshot stale authority", err, want)
				}
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("audit snapshot leaked lease")
			}
			t.Logf("audit RR backend=%d concurrent commit backend=%d", barrier.pid, pid)
		})
	}
}

func postgresAuditCleanup(t *testing.T) {
	for _, kind := range []string{"cancel count", "cancel page", "deadline", "panic page", "count driver", "page driver", "commit", "audit failure"} {
		t.Run(kind, func(t *testing.T) {
			h := newAuditHarness(t)
			base := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
			if kind == "audit failure" {
				if _, err := h.f.DB.ExecContext(h.ctx, `UPDATE audit_logs SET safe_after='{"unknown":"synthetic"}' WHERE action='ALR_FIXTURE';CREATE FUNCTION audit_read_fault() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'controlled attempt failure';END$$;CREATE TRIGGER audit_read_fault BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION audit_read_fault()`); err != nil {
					t.Fatal(err)
				}
				res, err := auditv1.NewAuditServiceClient(h.conn).ListAuditLogs(shared.RPCContext(h.ctx, h.admin.Token), &auditv1.ListAuditLogsRequest{Filter: &auditv1.AuditLogFilter{Action: "ALR_FIXTURE"}})
				if res != nil || status.Code(err) != codes.Unavailable {
					t.Fatal(res, err)
				}
			} else {
				ctx := managementPrincipal(t, h.managementHarness, h.admin.Token, auditReadMethod)
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				var provider domain.ReadSnapshotProvider = base
				want := error(common.ErrDependencyUnavailable)
				switch kind {
				case "cancel count":
					provider = featuremocks.PermissionSnapshot{Provider: base, CancelAt: 3, Cancel: cancel}
					want = context.Canceled
				case "cancel page":
					provider = featuremocks.PermissionSnapshot{Provider: base, CancelAt: 4, Cancel: cancel}
					want = context.Canceled
				case "deadline":
					var stop context.CancelFunc
					ctx, stop = context.WithDeadline(ctx, shared.ServiceTime)
					defer stop()
					want = context.DeadlineExceeded
				case "panic page":
					provider = featuremocks.PermissionSnapshot{Provider: base, PanicAt: 4}
					want = common.ErrInternalServerError
				case "count driver":
					if _, err := h.f.DB.ExecContext(h.ctx, `DROP TABLE audit_logs`); err != nil {
						t.Fatal(err)
					}
				case "page driver":
					if _, err := h.f.DB.ExecContext(h.ctx, `ALTER TABLE audit_logs DROP COLUMN safe_after`); err != nil {
						t.Fatal(err)
					}
				case "commit":
					provider = permissionCommitFailure{base}
				}
				res, err := auditServiceFor(t, h, provider).ListAuditLogs(ctx, domain.AuditLogQuery{Action: "ALR_FIXTURE"})
				if !errors.Is(err, want) || res.Items != nil {
					t.Fatal("partial result/wrong error", res.Total, err, want)
				}
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("audit primary lease not released")
			}
			var one int
			if err := h.f.DB.QueryRowContext(h.ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
				t.Fatal("pool not reusable", err)
			}
		})
	}
}
