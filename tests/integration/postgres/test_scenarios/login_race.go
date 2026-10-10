//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	ormconfig "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func postgresLoginProofRace(t *testing.T) {
	for _, change := range []string{"password", "status"} {
		t.Run(change, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			deps := loginDependencies(t, ctx, fixture, 0)
			observed := &observedProvider{inner: deps.Transactions}
			deps.Transactions = observed
			proofReady, release := make(chan struct{}), make(chan struct{})
			done := make(chan loginResult, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			finished := false
			defer func() {
				unblock()
				if !finished {
					waitLogin(t, ctx, done)
				}
			}()
			deps.VerifyPassword = func(saved, password string) (bool, error) {
				valid, err := authfeature.VerifyLoginPassword(saved, password)
				if err != nil || !valid {
					return valid, err
				}
				close(proofReady)
				select {
				case <-release:
					return true, nil
				case <-ctx.Done():
					return false, ctx.Err()
				}
			}
			service := loginService(t, deps)
			go func() {
				res, err := service.Login(ctx, loginRequest())
				done <- loginResult{res, err}
			}()
			select {
			case <-proofReady:
			case <-done:
				finished = true
				t.Fatal("Login failed before password-proof barrier")
			case <-ctx.Done():
				t.Fatal("password-proof barrier timed out")
			}
			// Keep mutation connection checked out after commit: Login must start
			// its authoritative transaction on the other physical backend.
			conn, err := fixture.DB.Conn(ctx)
			if err != nil {
				t.Fatal("mutation connection checkout failed")
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, fixture.Name)
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal("mutation begin failed")
			}
			defer tx.Rollback()
			adapter := orm.NewSqlTransactionAdapter(ctx, tx, dialect.NewPostgres(), ormconfig.Config{})
			if ok, err := adapter.TryLock(ctx, common.UserLockKey(1)); err != nil || !ok {
				t.Fatal("password/status mutation could not acquire canonical scope")
			}
			if change == "password" {
				_, err = tx.ExecContext(ctx, "UPDATE auth SET password=$1,version=version+1,auth_version=auth_version+1 WHERE id=1", shared.LoginPasswordHashFor("changed12"))
			} else {
				_, err = tx.ExecContext(ctx, "UPDATE auth SET status='SUSPENDED',version=version+1,auth_version=auth_version+1 WHERE id=1")
			}
			if err != nil || tx.Commit() != nil {
				t.Fatal("controlled credential/status mutation did not commit")
			}
			unblock()
			result := waitLogin(t, ctx, done)
			finished = true
			if result.response != nil || !errors.Is(result.err, auth.ErrLoginFailed) || observed.pid == 0 || observed.pid == pid {
				t.Fatal("stale proof issued credentials or lacked distinct backend evidence")
			}
			t.Logf("proof-before-change barrier: mutation backend=%d Login backend=%d; committed change precedes locked recheck", pid, observed.pid)
			assertLoginRows(t, ctx, fixture, 0, 1)
			var safe bool
			if err := fixture.DB.QueryRowContext(ctx, `SELECT actor_user_id IS NULL AND actor_service='rest-orc' AND target_type='rpc'
			 AND target_id='/user.v1.UserService/Login' AND reason='CREDENTIAL_DENIED' AND safe_before IS NULL AND safe_after IS NULL
			 FROM audit_logs WHERE action='LOGIN_FAILED'`).Scan(&safe); err != nil || !safe {
				t.Fatal("stale-proof denial fabricated user authority or recorded private data")
			}
			if err := conn.Close(); err != nil || fixture.DB.Stats().InUse != 0 {
				t.Fatal("stale-proof transaction/pool cleanup failed")
			}
		})
	}
}

func postgresLoginContention(t *testing.T) {
	for _, outcome := range []string{"commit", "failure", "panic", "cancellation"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			requestCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			deps := loginDependencies(t, requestCtx, fixture, 0)
			first := &observedProvider{inner: deps.Transactions}
			deps.Transactions = first
			ready, release := make(chan struct{}), make(chan struct{})
			done := make(chan loginResult, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			finished := false
			defer func() {
				unblock()
				if !finished {
					waitLogin(t, ctx, done)
				}
			}()
			sign := deps.Issue
			deps.Issue = func(req jwt.AccessIssue) (jwt.IssuedAccess, error) {
				close(ready) // All three required writes have succeeded in A.
				select {
				case <-release:
				case <-requestCtx.Done():
					return jwt.IssuedAccess{}, requestCtx.Err()
				}
				switch outcome {
				case "failure":
					return jwt.IssuedAccess{}, errors.New("controlled signing failure")
				case "panic":
					panic("controlled private panic")
				}
				return sign(req)
			}
			service := loginService(t, deps)
			go func() {
				res, err := service.Login(requestCtx, loginRequest())
				done <- loginResult{res, err}
			}()
			select {
			case <-ready:
			case <-done:
				finished = true
				t.Fatal("Login failed before precommit barrier")
			case <-ctx.Done():
				t.Fatal("precommit barrier timed out")
			}
			select {
			case <-done:
				finished = true
				t.Fatal("Login exposed response before commit")
			default:
			}
			assertLoginRows(t, ctx, fixture, 0, 0)
			other := loginDependencies(t, ctx, fixture, 100)
			second := &observedProvider{inner: other.Transactions}
			other.Transactions = second
			res, err := loginService(t, other).Login(ctx, loginRequest())
			if res != nil || !errors.Is(err, auth.ErrResourceBusy) || first.pid == 0 || second.pid == 0 || first.pid == second.pid {
				t.Fatal("Login contention must be busy on a distinct physical backend")
			}
			t.Logf("precommit barrier: holder backend=%d contender backend=%d; no visible rows or credential, contender RESOURCE_BUSY", first.pid, second.pid)
			if outcome == "cancellation" {
				cancel()
			} else {
				unblock()
			}
			result := waitLogin(t, ctx, done)
			finished = true
			want := 0
			switch outcome {
			case "commit":
				want = 1
				if result.err != nil || result.response == nil {
					t.Fatal("winner did not commit")
				}
			case "failure":
				if !errors.Is(result.err, commondictionary.ErrDependencyUnavailable) || result.response != nil {
					t.Fatal("sign failure did not deny credentials")
				}
			case "panic":
				if !errors.Is(result.err, commondictionary.ErrInternalServerError) || result.response != nil {
					t.Fatal("panic did not safely deny credentials")
				}
			case "cancellation":
				if !errors.Is(result.err, context.Canceled) || result.response != nil {
					t.Fatal("cancellation cause lost")
				}
			}
			// A checkout after acknowledgment covers database/sql automatic
			// cancellation cleanup before asserting release and retrying.
			conn, err := fixture.DB.Conn(ctx)
			if err != nil {
				t.Fatal("pool unusable after terminal outcome")
			}
			physicalIdentity(t, ctx, conn, fixture.Name)
			if err := conn.Close(); err != nil {
				t.Fatal("pool checkout cleanup failed")
			}
			assertLoginRows(t, ctx, fixture, want, 0)
			res, err = loginService(t, loginDependencies(t, ctx, fixture, 180)).Login(ctx, loginRequest())
			if err != nil || res == nil {
				t.Fatal("terminal Login leaked canonical lock")
			}
			assertLoginRows(t, ctx, fixture, want+1, 0)
			if fixture.DB.Stats().InUse != 0 {
				t.Fatal("Login leaked a pool slot")
			}
		})
	}
}
