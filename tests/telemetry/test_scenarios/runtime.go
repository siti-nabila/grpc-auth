package test_scenarios

import (
	"context"
	"database/sql"
	"errors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
	"sync/atomic"
	"testing"
	"time"
)

func Runtime() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "runtime unavailable stale recovery aggregate age and pool semantics", Run: runtimeStates},
		{Name: "slow sink isolates requests cleanup cancel join and discards queued ticks", Run: slowRuntime},
		{Name: "late buffered ticks are discarded without catch-up writes", Run: queuedRuntime},
	}
}

func runtimeStates(t *testing.T) {
	s := telemetry.NewStore()
	now := fixtures.ServiceTime
	oldest := now.Add(-time.Minute)
	failed := true
	empty := false
	writes := 0
	deps := telemetry.RuntimeDependencies{
		Pending: func(ctx context.Context) (int64, *time.Time, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("missing query deadline")
			}
			if failed {
				return 0, nil, errors.New("private-DB-sentinel")
			}
			if empty {
				return 0, nil, nil
			}
			return 2, &oldest, nil
		},
		Now: func() time.Time { return now }, Pool: func() sql.DBStats {
			return sql.DBStats{OpenConnections: 3, InUse: 2, Idle: 1, MaxOpenConnections: 25, WaitCount: 4, WaitDuration: time.Second}
		},
		Limiter: func() ratelimit.Counts {
			return ratelimit.Counts{Entries: 7, ServiceQuotaRejected: 1, IdentityCapacityRejected: 2}
		},
		Write: func(telemetry.Snapshot) error { writes++; return nil },
	}
	r, err := telemetry.NewSampler(s, deps)
	if err != nil {
		t.Fatal(err)
	}
	r.Collect(context.Background())
	v := s.Snapshot(now)
	if v.Runtime.Status != telemetry.Unavailable || v.CollectionFailures != 1 || writes != 1 {
		t.Fatal("false healthy zero", v.Runtime)
	}
	failed = false
	r.Collect(context.Background())
	v = s.Snapshot(now)
	if v.Runtime.Status != telemetry.Available || v.Runtime.Pending != 2 || v.Runtime.OldestAgeNS != uint64(time.Minute) || v.Runtime.Pool != [6]uint64{3, 2, 1, 25, 4, uint64(time.Second)} || v.Runtime.Rate != [2][2]uint64{{1, 0}, {0, 2}} || v.Runtime.LimiterEntries != 7 {
		t.Fatal("sample", v.Runtime)
	}
	at := v.Runtime.SampledAt
	now = now.Add(10 * time.Second)
	failed = true
	r.Collect(context.Background())
	v = s.Snapshot(now)
	if v.Runtime.Status != telemetry.Stale || v.Runtime.SampledAt != at || v.Runtime.Pending != 2 || v.Runtime.OldestAgeNS != uint64(70*time.Second) {
		t.Fatal("lost last good sample", v.Runtime)
	}
	failed = false
	empty = true
	r.Collect(context.Background())
	v = s.Snapshot(now)
	if v.Runtime.Pending != 0 || v.Runtime.OldestAgeNS != 0 || v.Runtime.Status != telemetry.Available {
		t.Fatal("empty sample")
	}
	empty = false
	oldest = now.Add(time.Hour)
	r.Collect(context.Background())
	if s.Snapshot(now).Runtime.OldestAgeNS != 0 {
		t.Fatal("future age")
	}
	deps.Write = func(telemetry.Snapshot) error { return errors.New("private-log-sentinel") }
	r, _ = telemetry.NewSampler(s, deps)
	r.Collect(context.Background())
	if s.Snapshot(now).ExportFailures != 1 {
		t.Fatal("export failure missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := s.Snapshot(now)
	r.Collect(ctx)
	if s.Snapshot(now) != before {
		t.Fatal("canceled cycle ran")
	}
	deps.Write = func(telemetry.Snapshot) error { panic("private sink panic") }
	r, _ = telemetry.NewSampler(s, deps)
	r.Collect(context.Background())
	if s.Snapshot(now).ExportFailures != 2 {
		t.Fatal("export panic escaped or was lost")
	}
	deps.Pending = func(context.Context) (int64, *time.Time, error) { panic("private collection panic") }
	r, _ = telemetry.NewSampler(s, deps)
	oldFailures := s.Snapshot(now).CollectionFailures
	r.Collect(context.Background())
	if s.Snapshot(now).CollectionFailures != oldFailures+1 || s.Snapshot(now).Runtime.Status != telemetry.Stale {
		t.Fatal("collection panic state")
	}
	if _, e := telemetry.NewSampler(s, telemetry.RuntimeDependencies{}); e == nil {
		t.Fatal("incomplete sampler")
	}
}

func queuedRuntime(t *testing.T) {
	s := telemetry.NewStore()
	entered, release := make(chan struct{}), make(chan struct{})
	var writes atomic.Int64
	r, _ := telemetry.NewSampler(s, telemetry.RuntimeDependencies{
		Pending: func(context.Context) (int64, *time.Time, error) { return 0, nil, nil }, Pool: func() sql.DBStats { return sql.DBStats{} }, Limiter: func() ratelimit.Counts { return ratelimit.Counts{} }, Now: func() time.Time { return fixtures.ServiceTime },
		Write: func(telemetry.Snapshot) error {
			if writes.Add(1) == 1 {
				close(entered)
				<-release
			}
			return nil
		},
	})
	ticks := make(chan time.Time, 10)
	done := make(chan struct{})
	go func() { r.Run(context.Background(), ticks); close(done) }()
	ticks <- fixtures.ServiceTime
	<-entered
	for range 8 {
		ticks <- fixtures.ServiceTime
	}
	close(ticks)
	close(release)
	<-done
	if writes.Load() != 1 {
		t.Fatal("delayed ticks produced catch-up burst", writes.Load())
	}
}

func slowRuntime(t *testing.T) {
	s := telemetry.NewStore()
	entered, release := make(chan struct{}), make(chan struct{})
	var reads, writes atomic.Int64
	limiter, _ := ratelimit.NewStore(func() time.Time { return fixtures.ServiceTime })
	r, e := telemetry.NewSampler(s, telemetry.RuntimeDependencies{
		Pending: func(context.Context) (int64, *time.Time, error) { reads.Add(1); return 0, nil, nil }, Pool: func() sql.DBStats { return sql.DBStats{} }, Limiter: limiter.Counts, Now: func() time.Time { return fixtures.ServiceTime },
		Write: func(telemetry.Snapshot) error { writes.Add(1); close(entered); <-release; return nil },
	})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time, 10)
	done := make(chan struct{})
	go func() { r.Run(ctx, ticks); close(done) }()
	ticks <- fixtures.ServiceTime
	<-entered
	// These complete while the sink is blocked: no request/limiter lock is held.
	s.Record(telemetry.MethodNames()[0], codes.OK, time.Millisecond, telemetry.AuthNone)
	if d, e := limiter.Admit(ratelimit.Key{Group: ratelimit.ServiceIngress, Value: "service"}); e != nil || !d.Allowed {
		t.Fatal(d, e)
	}
	limiter.Cleanup()
	for range 8 {
		ticks <- fixtures.ServiceTime
	}
	r.Collect(context.Background())
	if reads.Load() != 1 {
		t.Fatal("overlapping sample")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("abandoned active write")
	default:
	}
	close(release)
	<-done
	if reads.Load() != 1 || writes.Load() != 1 {
		t.Fatal("catch-up after cancellation")
	}
	if s.Snapshot(fixtures.ServiceTime).RPC[0].Codes[codes.OK] != 1 {
		t.Fatal("request blocked")
	}
}
