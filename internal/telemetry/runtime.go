package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"sync"
	"time"
)

const SampleInterval = time.Minute
const QueryTimeout = 2 * time.Second

type RuntimeDependencies struct {
	Pending func(context.Context) (int64, *time.Time, error)
	Pool    func() sql.DBStats
	Limiter func() ratelimit.Counts
	Now     func() time.Time
	Write   func(Snapshot) error
}

type Sampler struct {
	store *Store
	deps  RuntimeDependencies
	mu    sync.Mutex
}

func NewSampler(store *Store, d RuntimeDependencies) (*Sampler, error) {
	if store == nil || d.Pending == nil || d.Pool == nil || d.Limiter == nil || d.Now == nil || d.Write == nil {
		return nil, errors.New("runtime metrics dependencies are incomplete")
	}
	return &Sampler{store: store, deps: d}, nil
}

// Collect is one nonoverlapping cycle. Logging takes no observation-store lock.
func (s *Sampler) Collect(ctx context.Context) {
	if ctx.Err() != nil || !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()
	exporting := false
	defer func() {
		if recover() != nil {
			s.store.mu.Lock()
			defer s.store.mu.Unlock()
			if exporting {
				s.store.data.ExportFailures = add(s.store.data.ExportFailures, 1)
			} else {
				s.store.data.CollectionFailures = add(s.store.data.CollectionFailures, 1)
				if s.store.data.Runtime.Status != Unavailable {
					s.store.data.Runtime.Status = Stale
				}
			}
		}
	}()
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()
	count, oldest, err := s.deps.Pending(queryCtx)
	if err == nil {
		err = queryCtx.Err()
	}
	cancel()
	if err == nil && (count < 0 || count > 0 && oldest == nil || count == 0 && oldest != nil) {
		err = errors.New("invalid pending sample")
	}
	now := s.deps.Now()
	pool := s.deps.Pool()
	limiter := s.deps.Limiter()
	s.store.mu.Lock()
	r := &s.store.data.Runtime
	if err != nil {
		s.store.data.CollectionFailures = add(s.store.data.CollectionFailures, 1)
		if r.Status != Unavailable {
			r.Status = Stale
		}
	} else {
		r.Status = Available
		r.SampledAt = now.Unix()
		r.Pending = count
		s.store.oldest = time.Time{}
		if oldest != nil {
			s.store.oldest = *oldest
		}
	}
	r.Pool = [6]uint64{nonnegative(int64(pool.OpenConnections)), nonnegative(int64(pool.InUse)), nonnegative(int64(pool.Idle)), nonnegative(int64(pool.MaxOpenConnections)), nonnegative(pool.WaitCount), nonnegative(int64(pool.WaitDuration))}
	r.LimiterEntries = limiter.Entries
	r.Rate = [2][2]uint64{{limiter.ServiceQuotaRejected, limiter.ServiceCapacityRejected}, {limiter.IdentityQuotaRejected, limiter.IdentityCapacityRejected}}
	s.store.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	exporting = true
	if s.deps.Write(s.store.Snapshot(now)) != nil {
		s.store.mu.Lock()
		s.store.data.ExportFailures = add(s.store.data.ExportFailures, 1)
		s.store.mu.Unlock()
	}
}

func nonnegative(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

func (s *Sampler) Run(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok || ctx.Err() != nil {
				return
			}
			s.Collect(ctx)
			// Discard delayed ticks rather than queue catch-up samples after slow I/O.
			if !discardTicks(ticks) {
				return
			}
		}
	}
}

func discardTicks(ticks <-chan time.Time) bool {
	for {
		select {
		case _, ok := <-ticks:
			if !ok {
				return false
			}
		default:
			return true
		}
	}
}
