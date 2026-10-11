//go:build postgres

package test_scenarios

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func SessionGeneration() []testutils.Scenario {
	return []testutils.Scenario{{Name: "prepared migration backfills active generation and leaves revoked legacy unknown in owned schema only", Run: postgresSessionGeneration}}
}

func postgresSessionGeneration(t *testing.T) {
	ctx, f := loginFixture(t, false)
	first, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := loginService(t, loginDependencies(t, ctx, f, 80)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	// Recreate only this owned fixture's legacy shape, never the application schema.
	if _, err = f.DB.ExecContext(ctx, `UPDATE sessions SET status='REVOKED',revoked_at=created_at,revoke_reason='LOGOUT' WHERE id=$1`, first.SessionID); err != nil {
		t.Fatal("legacy fixture setup", err)
	}
	if _, err = f.DB.ExecContext(ctx, `ALTER TABLE sessions DROP COLUMN auth_version`); err != nil {
		t.Fatal("legacy fixture column setup", err)
	}
	script, err := os.ReadFile("../../../docs/sql/session-lifecycle/01_session_generation.sql")
	if err != nil {
		t.Fatal("prepared migration unavailable")
	}
	// All public qualification in the prepared file is remapped to our harness-owned namespace.
	owned := strings.ReplaceAll(string(script), "public.", pq.QuoteIdentifier(f.Name)+".")
	if strings.Contains(owned, "public.") || !strings.HasPrefix(f.Name, "grpc_auth_test_") {
		t.Fatal("unsafe migration fixture target")
	}
	if _, err = f.DB.ExecContext(ctx, owned); err != nil {
		t.Fatal("owned migration failed", err)
	}
	var legacy sql.NullInt64
	var active int64
	if err = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, first.SessionID).Scan(&legacy); err != nil || legacy.Valid {
		t.Fatal("revoked legacy generation was fabricated", err)
	}
	if err = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, second.SessionID).Scan(&active); err != nil || active != 1 {
		t.Fatal("active legacy generation not preserved", err)
	}
	ctx = refreshCaller(t, ctx)
	if res, err := refreshService(t, refreshDependencies(t, ctx, f, 160)).Refresh(ctx, first.RefreshToken); res != nil || !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("unknown generation authorized refresh", err)
	}
	if _, err = refreshService(t, refreshDependencies(t, ctx, f, 160)).Refresh(ctx, second.RefreshToken); err != nil {
		t.Fatal("backfilled active session rejected", err)
	}
	// A known terminal session keeps its issuance generation across later global invalidation.
	if _, err = f.DB.ExecContext(ctx, `UPDATE sessions SET status='REVOKED',revoked_at=created_at,revoke_reason='LOGOUT' WHERE id=$1`, second.SessionID); err != nil {
		t.Fatal("terminal fixture setup", err)
	}
	if _, err = f.DB.ExecContext(ctx, `UPDATE auth SET version=version+1,auth_version=auth_version+1 WHERE id=1`); err != nil {
		t.Fatal("generation invalidation fixture", err)
	}
	var userGeneration, sessionGeneration int64
	if err = f.DB.QueryRowContext(ctx, `SELECT a.auth_version,s.auth_version FROM auth a JOIN sessions s ON s.user_id=a.id WHERE s.id=$1`, second.SessionID).Scan(&userGeneration, &sessionGeneration); err != nil || userGeneration != 2 || sessionGeneration != 1 {
		t.Fatal("terminal generation was overwritten", err)
	}
	// New Login snapshots generation2; migration never supplies a default1.
	fresh, err := loginService(t, loginDependencies(t, ctx, f, 230)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal("postmigration Login", err)
	}
	if err = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, fresh.SessionID).Scan(&active); err != nil || active != 2 {
		t.Fatal("new Login did not persist current generation", err)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("generation fixture pool leak")
	}
}
