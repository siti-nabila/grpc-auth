//go:build postgres

package test_scenarios

import (
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"
	"time"
)

func RuntimeMetrics() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "primary aggregate excludes published includes delayed retries without mutation", Run: postgresPendingMetrics},
		{Name: "service quota bounds admitted failed attempt audit and preserves recorder failures", Run: postgresServiceMetrics},
	}
}

func postgresPendingMetrics(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	r := reader.NewOutboxMetricsReader(orm.NewSqlQueryAdapter(ctx, f.DB, dialect.NewPostgres(), cfg))
	v, err := r.Pending(ctx)
	if err != nil || v.Count != 0 || v.Oldest.Valid {
		t.Fatal("empty aggregate", v, err)
	}
	now := shared.ServiceTime
	for n := 0; n < 3; n++ {
		id := []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003"}[n]
		var published any
		if n == 2 {
			published = now
		}
		if _, err = f.DB.ExecContext(ctx, `INSERT INTO outbox_events(event_id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,payload,created_at,published_at,next_attempt_at) VALUES($1,'iam.user.created.v1',1,'user','1',1,'{}',$2,$3,$4)`, id, now.Add(time.Duration(n)*time.Hour), published, now.Add(24*time.Hour)); err != nil {
			t.Fatal("owned pending fixture failed")
		}
	}
	v, err = r.Pending(ctx)
	if err != nil || v.Count != 2 || !v.Oldest.Valid || !v.Oldest.Time.Equal(now) {
		t.Fatal("pending aggregate", v, err)
	}
	if _, err = f.DB.ExecContext(ctx, "UPDATE outbox_events SET published_at=$1 WHERE created_at=$2", now, now); err != nil {
		t.Fatal(err)
	}
	v, err = r.Pending(ctx)
	if err != nil || v.Count != 1 || !v.Oldest.Time.Equal(now.Add(time.Hour)) {
		t.Fatal("future oldest", v, err)
	}
	var total, pending int
	if err = f.DB.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE published_at IS NULL) FROM outbox_events`).Scan(&total, &pending); err != nil || total != 3 || pending != 1 || f.DB.Stats().InUse != 0 {
		t.Fatal("reader mutated/leaked")
	}
}

func postgresServiceMetrics(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	names := telemetry.MethodNames()
	config := shared.ServiceConfig()
	config.Callers[0].Methods = names[:37]
	conn, foundation := shared.FoundationServerWithService(t, f.DB, func() time.Time { return shared.ServiceTime }, loginEntropy(0), config)
	users := userv1.NewUserServiceClient(conn)
	for range 3 {
		_, err := users.Me(shared.RPCContext(ctx, ""), &emptypb.Empty{})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatal("admitted denial", err)
		}
	}
	for range 97 {
		d, err := foundation.Login.Limiter.Admit(ratelimit.Key{Group: ratelimit.ServiceIngress, Value: "rest-orc"})
		if err != nil || !d.Allowed {
			t.Fatal("quota precondition")
		}
	}
	for _, method := range names[:37] {
		err := conn.Invoke(shared.RPCContext(ctx, ""), method, &emptypb.Empty{}, &emptypb.Empty{})
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatal("early service gate", method, err)
		}
	}
	var audits int
	if err := f.DB.QueryRowContext(ctx, "SELECT count(*) FROM audit_logs").Scan(&audits); err != nil || audits != 3 {
		t.Fatal("rate rejection persisted extra audit", audits)
	}
	if foundation.Login.Limiter.Counts().ServiceQuotaRejected != 37 || foundation.Login.AuditFailures() != 0 {
		t.Fatal("quota/audit observation")
	}
	// A fresh process-local foundation isolates the next fault scenario without a bypass.
	conn2, second := shared.FoundationServerWithService(t, f.DB, func() time.Time { return shared.ServiceTime }, loginEntropy(0), config)
	users = userv1.NewUserServiceClient(conn2)
	if _, err := f.DB.ExecContext(ctx, "ALTER TABLE audit_logs RENAME TO audit_unavailable"); err != nil {
		t.Fatal(err)
	}
	_, err := users.Me(shared.RPCContext(ctx, ""), &emptypb.Empty{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatal("audit failure changed original denial", err)
	}
	_, err = users.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "missing@example.test", Password: "secret12"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatal("login audit failure changed denial", err)
	}
	_, err = users.Register(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "new@example.test", Password: "secret12"})
	if status.Code(err) != codes.Unavailable {
		t.Fatal("required audit failure did not fail closed", err)
	}
	v := second.Login.Metrics.Snapshot(shared.ServiceTime)
	if v.Audit[telemetry.SecurityAttempt] != 2 || v.Audit[telemetry.LoginAttempt] != 1 || v.Audit[telemetry.RequiredWrite] != 1 || second.Login.AuditFailures() != 3 {
		t.Fatal("failure origin/count", v.Audit, second.Login.AuditFailures())
	}
	if _, err = f.DB.ExecContext(ctx, "ALTER TABLE audit_unavailable RENAME TO audit_logs"); err != nil {
		t.Fatal(err)
	}
	var created, pending int
	if err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM auth WHERE email='new@example.test'),(SELECT count(*) FROM outbox_events)`).Scan(&created, &pending); err != nil || created != 0 || pending != 0 || f.DB.Stats().InUse != 0 {
		t.Fatal("partial required write effects")
	}
}
