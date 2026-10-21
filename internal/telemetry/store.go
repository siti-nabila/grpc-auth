package telemetry

import (
	"math"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
)

const (
	MethodCount     = 38
	CodeCount       = 17
	HistogramCount  = 9
	MaxPayloadBytes = 64 * 1024
)

type AuthOutcome uint8

const (
	AuthNone AuthOutcome = iota
	AuthFailure
	AuthDenied
	AuthDependency
)

type AuditOrigin uint8

const (
	RequiredWrite AuditOrigin = iota
	LoginAttempt
	SecurityAttempt
	FinalizationUnknown
	auditOrigins
)

type RPC struct {
	Codes      [CodeCount]uint64      `json:"codes"`
	Auth       [3]uint64              `json:"auth"`
	DurationNS uint64                 `json:"duration_ns"`
	Histogram  [HistogramCount]uint64 `json:"histogram"`
}

type SampleStatus uint8

const (
	Unavailable SampleStatus = iota
	Available
	Stale
)

type Runtime struct {
	Status         SampleStatus `json:"status"`
	SampledAt      int64        `json:"sampled_at"`
	Pending        int64        `json:"pending"`
	OldestAgeNS    uint64       `json:"oldest_age_ns"`
	Pool           [6]uint64    `json:"pool"`
	LimiterEntries int          `json:"limiter_entries"`
	Rate           [2][2]uint64 `json:"rate"`
}

type Snapshot struct {
	RPC                [MethodCount]RPC     `json:"rpc"`
	Audit              [auditOrigins]uint64 `json:"audit"`
	Runtime            Runtime              `json:"runtime"`
	CollectionFailures uint64               `json:"collection_failures"`
	ExportFailures     uint64               `json:"export_failures"`
}

type Store struct {
	mu     sync.Mutex
	data   Snapshot
	oldest time.Time
}

func NewStore() *Store { return new(Store) }

func add(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}

// Record only retains finite indices and numeric values, never caller input.
func (s *Store) Record(method string, code codes.Code, elapsed time.Duration, outcome AuthOutcome) {
	if elapsed < 0 {
		elapsed = 0
	}
	if code >= CodeCount {
		code = codes.Unknown
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &s.data.RPC[MethodIndex(method)]
	r.Codes[code] = add(r.Codes[code], 1)
	if outcome > AuthNone && outcome <= AuthDependency {
		r.Auth[outcome-1] = add(r.Auth[outcome-1], 1)
	}
	r.DurationNS = add(r.DurationNS, uint64(elapsed))
	for i, bound := range [...]time.Duration{time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond, 500 * time.Millisecond, time.Second, 5 * time.Second, time.Duration(math.MaxInt64)} {
		if elapsed <= bound {
			r.Histogram[i] = add(r.Histogram[i], 1)
		}
	}
}

func (s *Store) AuditFailure(origin AuditOrigin) {
	if origin >= auditOrigins {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Audit[origin] = add(s.data.Audit[origin], 1)
}

func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data
	if v.Runtime.Status != Unavailable && !s.oldest.IsZero() && now.After(s.oldest) {
		v.Runtime.OldestAgeNS = uint64(now.Sub(s.oldest))
	}
	return v
}
