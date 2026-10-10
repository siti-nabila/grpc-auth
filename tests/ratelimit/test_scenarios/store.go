package test_scenarios

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "numeric burst and refill boundaries are deterministic", Run: rates}, {Name: "layered concurrent rejection does not consume other quota", Run: layered}, {Name: "capacity idle cleanup and shutdown preserve active entries", Run: retention}}
}

func rates(t *testing.T) {
	for _, tc := range []struct {
		group  ratelimit.Group
		burst  int
		refill time.Duration
	}{{ratelimit.LoginIP, 10, 2 * time.Second}, {ratelimit.LoginAccount, 3, 12 * time.Second}, {ratelimit.RegisterIP, 2, 12 * time.Second}, {ratelimit.ProtectedRead, 20, time.Second / 2}} {
		now := fixtures.ServiceTime
		s, err := ratelimit.NewStore(func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		key := ratelimit.Key{Group: tc.group, Value: "test-key"}
		for n := 0; n < tc.burst; n++ {
			d, err := s.Admit(key)
			if err != nil || !d.Allowed {
				t.Fatal("burst rejected", tc.group, n)
			}
		}
		d, err := s.Admit(key)
		if err != nil || d.Allowed || d.Reason != ratelimit.Quota || d.RetryAfter <= 0 {
			t.Fatal("burst exceeded")
		}
		now = now.Add(tc.refill - time.Nanosecond)
		d, _ = s.Admit(key)
		if d.Allowed {
			t.Fatal("early refill")
		}
		now = now.Add(time.Nanosecond)
		d, _ = s.Admit(key)
		if !d.Allowed {
			t.Fatal("refill boundary rejected", tc.group)
		}
	}
	if _, err := ratelimit.NewStore(nil); err == nil {
		t.Fatal("missing clock")
	}
}

func layered(t *testing.T) {
	s, _ := ratelimit.NewStore(func() time.Time { return fixtures.ServiceTime })
	ip := ratelimit.Key{Group: ratelimit.LoginIP, Value: "ip"}
	account := ratelimit.Key{Group: ratelimit.LoginAccount, Value: "account"}
	var admitted atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for n := 0; n < 100; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			d, err := s.Admit(ip, account)
			if err != nil {
				t.Error(err)
			}
			if d.Allowed {
				admitted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if admitted.Load() != 3 {
		t.Fatal("layered concurrent over-admission")
	}
	for n := 0; n < 7; n++ {
		d, _ := s.Admit(ip)
		if !d.Allowed {
			t.Fatal("rejection consumed IP bucket")
		}
	}
	if d, _ := s.Admit(ip); d.Allowed {
		t.Fatal("IP burst exceeded")
	}
	if s.Counts().Entries != 2 || s.Counts().QuotaRejected != 98 {
		t.Fatal("bounded rejection counts")
	}
}

func retention(t *testing.T) {
	now := fixtures.ServiceTime
	s, _ := ratelimit.NewStore(func() time.Time { return now })
	for n := 0; n < ratelimit.MaxEntries; n++ {
		d, err := s.Admit(ratelimit.Key{Group: ratelimit.RegisterIP, Value: strconv.Itoa(n)})
		if err != nil || !d.Allowed {
			t.Fatal("capacity filled early")
		}
	}
	existing := ratelimit.Key{Group: ratelimit.RegisterIP, Value: "0"}
	newKey := ratelimit.Key{Group: ratelimit.RegisterIP, Value: "new"}
	d, _ := s.Admit(existing, newKey)
	if d.Allowed || d.Reason != ratelimit.Capacity || d.RetryAfter != time.Minute || s.Counts().Entries != 10000 {
		t.Fatal("capacity eviction/allocation")
	}
	d, _ = s.Admit(existing)
	if !d.Allowed {
		t.Fatal("capacity rejection consumed existing quota")
	}
	d, _ = s.Admit(existing)
	if d.Allowed {
		t.Fatal("exhausted active key reset")
	}
	now = now.Add(14 * time.Minute)
	s.Admit(existing)
	s.Admit(existing)
	d, _ = s.Admit(existing)
	if d.Allowed {
		t.Fatal("refilled burst exceeded")
	}
	now = now.Add(time.Minute)
	s.Cleanup()
	if s.Counts().Entries != 1 {
		t.Fatal("cleanup removed active or retained idle")
	}
	now = now.Add(14 * time.Minute)
	s.Cleanup()
	if s.Counts().Entries != 0 {
		t.Fatal("idle TTL boundary")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	ticks := make(chan time.Time)
	go func() { s.RunCleanup(ctx, ticks); close(done) }()
	cancel()
	<-done
	if _, err := s.Admit(); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := s.Admit(ratelimit.Key{Group: "unknown", Value: "key"}); err == nil {
		t.Fatal("unknown group accepted")
	}
}
