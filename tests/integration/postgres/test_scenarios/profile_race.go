//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"sync"
	"testing"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func SessionProfileConcurrency() []testutils.Scenario {
	return []testutils.Scenario{{Name: "committed actor or session revocation precedes locked profile recheck", Run: postgresProfileRecheck}, {Name: "profile retains ordered locks until commit rollback panic or cancellation", Run: postgresProfileContention}}
}

type profileBeginBarrier struct {
	provider domain.TransactionProvider
	ready    chan struct{}
	release  <-chan struct{}
}

func (p profileBeginBarrier) Begin(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.provider.Begin(ctx)
	if err != nil {
		return nil, err
	}
	close(p.ready)
	select {
	case <-p.release:
		return tx, nil
	case <-ctx.Done():
		_ = tx.Rollback()
		return nil, ctx.Err()
	}
}

type sessionProfileResult struct {
	response *profile.UpdateResult
	err      error
}

func waitProfile(t *testing.T, ctx context.Context, done <-chan sessionProfileResult) sessionProfileResult {
	t.Helper()
	select {
	case res := <-done:
		return res
	case <-ctx.Done():
		t.Fatal("profile worker timed out")
		return sessionProfileResult{}
	}
}

func postgresProfileRecheck(t *testing.T) {
	for _, change := range []string{"session", "status"} {
		t.Run(change, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			ctx = foundationActor(t, ctx, fixture, profilev1.ProfileService_UpdateProfile_FullMethodName)
			principal, err := sessions.PrincipalFromContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			deps := foundationProfileDeps(fixture)
			observed := &observedProvider{inner: deps.Transactions}
			ready, release := make(chan struct{}), make(chan struct{})
			deps.Transactions = profileBeginBarrier{provider: observed, ready: ready, release: release}
			done := make(chan sessionProfileResult, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			finished := false
			defer func() {
				unblock()
				if !finished {
					waitProfile(t, ctx, done)
				}
			}()
			s, err := profile.NewSessionProfileService(deps)
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				res, err := s.UpdateProfile(ctx, domain.ProfileUpdate{Name: "Must deny"})
				done <- sessionProfileResult{res, err}
			}()
			select {
			case <-ready:
			case got := <-done:
				finished = true
				t.Fatal("profile failed before pre-lock barrier", got.err)
			case <-ctx.Done():
				t.Fatal("pre-lock barrier timed out")
			}
			conn, err := fixture.DB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, fixture.Name)
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			adapter := orm.NewSqlTransactionAdapter(ctx, tx, dialect.NewPostgres(), config.Config{})
			for _, key := range []string{featurecommon.UserLockKey(principal.UserID()), "auth:session:" + principal.SessionID()} {
				if ok, err := adapter.TryLock(ctx, key); err != nil || !ok {
					t.Fatal("revocation canonical lock failed")
				}
			}
			if change == "session" {
				_, err = tx.ExecContext(ctx, `UPDATE sessions SET status='REVOKED',revoked_at=$1,revoke_reason='fixture revocation' WHERE id=$2`, shared.ServiceTime, principal.SessionID())
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE auth SET status='SUSPENDED',auth_version=auth_version+1 WHERE id=1`)
			}
			if err != nil || tx.Commit() != nil {
				t.Fatal("revocation commit failed")
			}
			unblock()
			res := waitProfile(t, ctx, done)
			finished = true
			if res.response != nil || !errors.Is(res.err, auth.ErrInvalidSession) || observed.pid == pid || observed.pid == 0 {
				t.Fatal("stale actor authorized after committed revocation", res.err)
			}
			var name string
			var version, audit, event int
			if err = conn.QueryRowContext(ctx, `SELECT p.name,a.version,(SELECT count(*) FROM audit_logs WHERE action IN ('USER_PROFILE_UPDATED','NO_OP')),(SELECT count(*) FROM outbox_events) FROM profile p JOIN auth a ON a.id=p.user_id WHERE a.id=1`).Scan(&name, &version, &audit, &event); err != nil || name != "Fixture user" || version != 1 || audit != 0 || event != 0 {
				t.Fatal("unauthorized profile effects", err)
			}
			t.Logf("pre-lock barrier: revocation backend=%d profile backend=%d; committed %s revocation denied before write", pid, observed.pid, change)
			if err = conn.Close(); err != nil || fixture.DB.Stats().InUse != 0 {
				t.Fatal("revocation recheck leaked pool slot")
			}
		})
	}
}

func postgresProfileContention(t *testing.T) {
	for _, outcome := range []string{"commit", "failure", "panic", "cancellation"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			ctx = foundationActor(t, ctx, fixture, profilev1.ProfileService_UpdateProfile_FullMethodName)
			requestCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			deps := foundationProfileDeps(fixture)
			first := &observedProvider{inner: deps.Transactions}
			deps.Transactions = first
			ready, release := make(chan struct{}), make(chan struct{})
			done := make(chan sessionProfileResult, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			finished := false
			defer func() {
				unblock()
				if !finished {
					waitProfile(t, ctx, done)
				}
			}()
			eventID := deps.EventID
			deps.EventID = func() (string, error) {
				close(ready)
				select {
				case <-release:
				case <-requestCtx.Done():
					return "", requestCtx.Err()
				}
				switch outcome {
				case "failure":
					return "", errors.New("controlled event failure")
				case "panic":
					panic("controlled private panic")
				}
				return eventID()
			}
			s, err := profile.NewSessionProfileService(deps)
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				res, err := s.UpdateProfile(requestCtx, domain.ProfileUpdate{Name: "Winner"})
				done <- sessionProfileResult{res, err}
			}()
			select {
			case <-ready:
			case got := <-done:
				finished = true
				t.Fatal("profile failed before precommit barrier", got.err)
			case <-ctx.Done():
				t.Fatal("profile precommit barrier timed out")
			}
			assertFoundationProfile(t, ctx, fixture, "Fixture user", 1, 0, 0)
			other := foundationProfileDeps(fixture)
			second := &observedProvider{inner: other.Transactions}
			other.Transactions = second
			contender, err := profile.NewSessionProfileService(other)
			if err != nil {
				t.Fatal(err)
			}
			res, err := contender.UpdateProfile(ctx, domain.ProfileUpdate{Name: "Contender"})
			if res != nil || !errors.Is(err, auth.ErrResourceBusy) || first.pid == 0 || second.pid == 0 || first.pid == second.pid {
				t.Fatal("profile contention lacked distinct backend or busy denial", err)
			}
			t.Logf("profile precommit barrier: holder=%d contender=%d; tentative profile/version/audit invisible, locks retained", first.pid, second.pid)
			if outcome == "cancellation" {
				cancel()
			} else {
				unblock()
			}
			got := waitProfile(t, ctx, done)
			finished = true
			version, changes := 1, 0
			name := "Fixture user"
			if outcome == "commit" {
				version = 2
				changes = 1
				name = "Winner"
				if got.err != nil || got.response == nil {
					t.Fatal("profile winner failed", got.err)
				}
			} else if got.err == nil || got.response != nil {
				t.Fatal("profile failure returned success", got.err)
			}
			if outcome == "failure" && !errors.Is(got.err, common.ErrDependencyUnavailable) || outcome == "panic" && !errors.Is(got.err, common.ErrInternalServerError) || outcome == "cancellation" && !errors.Is(got.err, context.Canceled) {
				t.Fatal("profile terminal outcome lost", got.err)
			}
			conn, err := fixture.DB.Conn(ctx)
			if err != nil {
				t.Fatal("profile failure poisoned pool")
			}
			physicalIdentity(t, ctx, conn, fixture.Name)
			if err = conn.Close(); err != nil {
				t.Fatal(err)
			}
			assertFoundationProfile(t, ctx, fixture, name, version, changes, 0)
			other = foundationProfileDeps(fixture)
			other.EventID = func() (string, error) { return "ffffffff-ffff-4fff-8fff-ffffffffffff", nil }
			retry, _ := profile.NewSessionProfileService(other)
			res, err = retry.UpdateProfile(ctx, domain.ProfileUpdate{Name: "Retry"})
			if err != nil || res == nil || res.Version != int64(version+1) {
				t.Fatal("profile terminal outcome leaked canonical locks", err)
			}
			assertFoundationProfile(t, ctx, fixture, "Retry", version+1, changes+1, 0)
			if fixture.DB.Stats().InUse != 0 {
				t.Fatal("profile terminal outcome leaked pool slot")
			}
		})
	}
}
