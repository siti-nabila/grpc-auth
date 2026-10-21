package test_scenarios

import (
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Service() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "service concurrent burst100 and exact100ms refill", Run: serviceBurst},
		{Name: "shared cap preserves active service and cleanup", Run: serviceCap},
	}
}

func serviceBurst(t *testing.T) {
	now := fixtures.ServiceTime
	s, _ := ratelimit.NewStore(func() time.Time { return now })
	key := ratelimit.Key{Group: ratelimit.ServiceIngress, Value: "verified-service"}
	var wg sync.WaitGroup
	var admitted atomic.Int64
	start := make(chan struct{})
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			d, e := s.Admit(key, key)
			if e != nil {
				t.Error(e)
			}
			if d.Allowed {
				admitted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if admitted.Load() != 100 || s.Counts().ServiceQuotaRejected != 100 || s.Counts().Entries != 1 {
		t.Fatal("service overadmission", s.Counts())
	}
	now = now.Add(100*time.Millisecond - time.Nanosecond)
	d, _ := s.Admit(key)
	if d.Allowed || d.RetryAfter != time.Second {
		t.Fatal("early refill/retry", d)
	}
	now = now.Add(time.Nanosecond)
	d, _ = s.Admit(key)
	if !d.Allowed {
		t.Fatal("exact refill")
	}
	d, _ = s.Admit(key)
	if d.Allowed {
		t.Fatal("partial extra token")
	}
	other := ratelimit.Key{Group: ratelimit.ServiceIngress, Value: "other-service"}
	d, _ = s.Admit(other)
	if !d.Allowed {
		t.Fatal("caller quota merged")
	}
}

func serviceCap(t *testing.T) {
	now := fixtures.ServiceTime
	s, _ := ratelimit.NewStore(func() time.Time { return now })
	existing := ratelimit.Key{Group: ratelimit.ServiceIngress, Value: "verified-service"}
	s.Admit(existing)
	for n := 1; n < ratelimit.MaxEntries; n++ {
		s.Admit(ratelimit.Key{Group: ratelimit.RegisterIP, Value: strconv.Itoa(n)})
	}
	d, _ := s.Admit(ratelimit.Key{Group: ratelimit.ServiceIngress, Value: "new-service"})
	if d.Allowed || d.Reason != ratelimit.Capacity || d.RetryAfter != time.Minute || s.Counts().ServiceCapacityRejected != 1 {
		t.Fatal("service capacity", d)
	}
	for range 99 {
		d, _ = s.Admit(existing)
		if !d.Allowed {
			t.Fatal("active caller evicted")
		}
	}
	d, _ = s.Admit(existing)
	if d.Allowed {
		t.Fatal("active quota reset")
	}
	now = now.Add(14 * time.Minute)
	s.Admit(existing)
	now = now.Add(time.Minute)
	s.Cleanup()
	if s.Counts().Entries != 1 {
		t.Fatal("active service cleaned")
	}
	now = now.Add(14 * time.Minute)
	s.Cleanup()
	if s.Counts().Entries != 0 {
		t.Fatal("service idle retention")
	}
}
