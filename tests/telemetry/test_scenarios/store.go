package test_scenarios

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
)

func Core() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "fixed histogram boundaries saturation and copied snapshots", Run: histogram},
		{Name: "concurrent observations retain finite indices and no credentials", Run: concurrentMetrics},
	}
}

func histogram(t *testing.T) {
	s := telemetry.NewStore()
	method := telemetry.MethodNames()[0]
	for _, elapsed := range []time.Duration{-1, time.Millisecond, time.Millisecond + 1, 5 * time.Millisecond, 5 * time.Second, 5*time.Second + 1} {
		s.Record(method, codes.OK, elapsed, telemetry.AuthNone)
	}
	r := s.Snapshot(fixtures.ServiceTime).RPC[0]
	if r.Codes[codes.OK] != 6 || r.Histogram != [telemetry.HistogramCount]uint64{2, 4, 4, 4, 4, 4, 4, 5, 6} {
		t.Fatal("histogram boundaries", r)
	}
	copy := s.Snapshot(fixtures.ServiceTime)
	copy.RPC[0].Codes[codes.OK] = 0
	if s.Snapshot(fixtures.ServiceTime).RPC[0].Codes[codes.OK] != 6 {
		t.Fatal("mutable snapshot")
	}
	for range 3 {
		s.Record(method, codes.OK, time.Duration(math.MaxInt64), telemetry.AuthNone)
	}
	if s.Snapshot(fixtures.ServiceTime).RPC[0].DurationNS != math.MaxUint64 {
		t.Fatal("duration saturation")
	}
	if telemetry.MethodIndex("private-input") != 37 || len(telemetry.MethodNames()) != 38 {
		t.Fatal("finite method map")
	}
}

func concurrentMetrics(t *testing.T) {
	s := telemetry.NewStore()
	var wg sync.WaitGroup
	for n := range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Record("credential-sentinel-"+strings.Repeat("x", n%20), codes.Code(999), time.Millisecond, telemetry.AuthFailure)
			s.AuditFailure(telemetry.RequiredWrite)
			_ = s.Snapshot(fixtures.ServiceTime)
		}()
	}
	wg.Wait()
	v := s.Snapshot(fixtures.ServiceTime)
	if v.RPC[37].Codes[codes.Unknown] != 1000 || v.RPC[37].Auth[0] != 1000 || v.Audit[telemetry.RequiredWrite] != 1000 {
		t.Fatal("lost concurrent observation")
	}
	b, err := json.Marshal(v)
	if err != nil || strings.Contains(string(b), "credential-sentinel") || len(b) > telemetry.MaxPayloadBytes {
		t.Fatal("unsafe/unbounded snapshot")
	}
	v = fixtures.MaxTelemetrySnapshot()
	b, err = json.Marshal(v)
	if err != nil || len(b) > telemetry.MaxPayloadBytes {
		t.Fatal("worst-case schema size", len(b), err)
	}
}
