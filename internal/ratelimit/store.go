package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Group string

const (
	LoginIP         Group = "login-ip"
	LoginAccount    Group = "login-account"
	RegisterIP      Group = "register-ip"
	ProtectedRead   Group = "protected-read"
	RefreshIP       Group = "refresh-ip"
	RefreshSession  Group = "refresh-session"
	ChangePassword  Group = "change-password"
	AdminMutation   Group = "admin-mutation"
	IdleTTL               = 15 * time.Minute
	CleanupInterval       = time.Minute
	MaxEntries            = 10000
	// Integer credits retain exact nanosecond refill boundaries. One token costs
	// one minute of credits; refill adds the per-minute rate each nanosecond.
	tokenUnit int64 = int64(time.Minute)
)

type Reason string

const (
	Quota    Reason = "quota"
	Capacity Reason = "capacity"
)

type Key struct {
	Group Group
	Value string
}
type Decision struct {
	Allowed    bool
	Reason     Reason
	RetryAfter time.Duration
}
type bucket struct {
	tokens                int64
	refilledAt, checkedAt time.Time
}
type Counts struct {
	QuotaRejected, CapacityRejected uint64
	Entries                         int
}
type Store struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[Key]bucket
	counts  Counts
}

func NewStore(now func() time.Time) (*Store, error) {
	if now == nil {
		return nil, errors.New("rate limiter requires a clock")
	}
	return &Store{now: now, entries: make(map[Key]bucket)}, nil
}

func policy(group Group) (int64, int64) {
	switch group {
	case LoginIP:
		return 30, 10
	case LoginAccount:
		return 5, 3
	case RegisterIP:
		return 5, 2
	case RefreshIP:
		return 60, 10
	case RefreshSession:
		return 10, 2
	case ChangePassword:
		return 5, 2
	case AdminMutation:
		return 30, 5
	case ProtectedRead:
		return 120, 20
	}
	return 0, 0
}

// Admit checks and consumes the entire request in one critical section. Rejected
// checks touch existing idle timestamps but allocate or consume no other key.
func (s *Store) Admit(keys ...Key) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(keys) == 0 {
		return Decision{}, errors.New("rate limiter requires bucket keys")
	}
	now := s.now()
	pending := make(map[Key]bucket, len(keys))
	newCount := 0
	wait := time.Duration(0)
	for _, key := range keys {
		rate, burst := policy(key.Group)
		burst *= tokenUnit
		if rate == 0 || key.Value == "" {
			return Decision{}, errors.New("invalid rate limiter key")
		}
		if _, ok := pending[key]; ok {
			continue
		}
		b, exists := s.entries[key]
		if !exists {
			newCount++
			b = bucket{tokens: burst, refilledAt: now, checkedAt: now}
		} else {
			if now.After(b.refilledAt) {
				elapsed := now.Sub(b.refilledAt).Nanoseconds()
				needed := (burst - b.tokens + rate - 1) / rate
				if elapsed >= needed {
					b.tokens = burst
				} else {
					b.tokens += elapsed * rate
				}
				b.refilledAt = now
			}
			if now.After(b.checkedAt) {
				b.checkedAt = now
			}
			s.entries[key] = b
		}
		pending[key] = b
		if b.tokens < tokenUnit {
			perSecond := rate * int64(time.Second)
			duration := time.Duration((tokenUnit-b.tokens+perSecond-1)/perSecond) * time.Second
			if duration > wait {
				wait = duration
			}
		}
	}
	if len(s.entries)+newCount > MaxEntries {
		s.counts.CapacityRejected++
		return Decision{Reason: Capacity, RetryAfter: CleanupInterval}, nil
	}
	if wait > 0 {
		s.counts.QuotaRejected++
		return Decision{Reason: Quota, RetryAfter: wait}, nil
	}
	for key, b := range pending {
		b.tokens -= tokenUnit
		s.entries[key] = b
	}
	return Decision{Allowed: true}, nil
}

func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, b := range s.entries {
		if now.Sub(b.checkedAt) >= IdleTTL {
			delete(s.entries, key)
		}
	}
}

// Ticks are owned by the composition lifecycle. Tests supply controlled ticks.
func (s *Store) RunCleanup(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			s.Cleanup()
		}
	}
}

func (s *Store) Counts() Counts {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.counts
	c.Entries = len(s.entries)
	return c
}
