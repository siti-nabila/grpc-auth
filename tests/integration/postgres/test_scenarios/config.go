//go:build postgres

package test_scenarios

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Configuration() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "rejects missing explicit configuration without application fallback", Run: rejectMissingConfiguration},
		{Name: "rejects application system and mismatched database identities", Run: rejectUnsafeDatabase},
		{Name: "validates explicit DSN without exposing credentials", Run: validateDSNSafely},
	}
}

func load(values map[string]string) (fixtures.Config, error) {
	return fixtures.LoadConfig(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
}

func explicitConfig() map[string]string {
	return map[string]string{
		fixtures.EnabledVariable:  "1",
		fixtures.DSNVariable:      "host=invalid.test dbname=grpc_auth_test user=fixture password=fixture-secret sslmode=disable",
		fixtures.DatabaseVariable: "grpc_auth_test",
	}
}

func rejectMissingConfiguration(t *testing.T) {
	cases := []map[string]string{
		{},
		{fixtures.EnabledVariable: "true"},
		{fixtures.EnabledVariable: "1"},
		{fixtures.EnabledVariable: "1", fixtures.DSNVariable: "host=invalid.test"},
		{fixtures.EnabledVariable: "1", fixtures.DatabaseVariable: "grpc_auth_test"},
	}
	for _, values := range cases {
		cfg, err := load(values)
		if err == nil || cfg.DSN != "" || cfg.Database != "" {
			t.Fatal("missing or nonexplicit test configuration must fail without returning a DSN")
		}
	}
}

func rejectUnsafeDatabase(t *testing.T) {
	for _, name := range []string{"user", "postgres", "template0", "template1", " ", " grpc_auth_test"} {
		values := explicitConfig()
		values[fixtures.DatabaseVariable] = name
		if _, err := load(values); err == nil {
			t.Fatalf("unsafe expected database %q accepted", name)
		}
	}
	cfg, err := load(explicitConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := fixtures.ValidateDatabase(cfg, "user"); err == nil {
		t.Fatal("application database identity must be rejected")
	}
	if err := fixtures.ValidateDatabase(cfg, "GRPC_AUTH_TEST"); err == nil {
		t.Fatal("database identity must match exactly")
	}
	if err := fixtures.ValidateDatabase(fixtures.Config{}, ""); err == nil {
		t.Fatal("empty identity must be rejected")
	}
	if err := fixtures.ValidateDatabase(cfg, "grpc_auth_test"); err != nil {
		t.Fatal(err)
	}
	// Invalid application configuration must fail before opening any connection.
	db, err := fixtures.Connect(context.Background(), fixtures.Config{DSN: cfg.DSN, Database: "user"})
	if err == nil || db != nil {
		t.Fatal("Connect must reject application database before network access")
	}
}

func validateDSNSafely(t *testing.T) {
	values := explicitConfig()
	cfg, err := load(values)
	if err != nil || cfg.Database != "grpc_auth_test" || cfg.DSN != values[fixtures.DSNVariable] {
		t.Fatal("explicit valid test configuration rejected")
	}
	for _, dsn := range []string{
		"host=invalid.test dbname=user password=fixture-secret",
		"host=invalid.test password=fixture-secret",
		"dbname=grpc_auth_test dbname=user",
		"postgres://fixture:fixture-secret@invalid.test/user",
	} {
		values[fixtures.DSNVariable] = dsn
		if cfg, err := load(values); err == nil || cfg.DSN != "" {
			t.Fatal("DSN selecting another/default database must fail before network access")
		}
	}
	for _, dsn := range []string{
		"host=invalid.test dbname='grpc_auth_test' password='fixture dbname=user'",
		"postgres://fixture:fixture-secret@invalid.test/grpc_auth_test?sslmode=disable",
		"dbname=user dbname=grpc_auth_test",
		"dbname = grpc_auth_test password='fixture\\'secret'",
	} {
		values[fixtures.DSNVariable] = dsn
		if _, err := load(values); err != nil {
			t.Fatal("valid explicit URI/quoted/duplicate-key test DSN rejected")
		}
	}
	values[fixtures.DSNVariable] = "password='fixture-secret"
	cfg, err = load(values)
	if err == nil || strings.Contains(err.Error(), "fixture-secret") || cfg.DSN != "" {
		t.Fatal("malformed DSN must fail with redacted output")
	}
}

// Environment verifies only readiness; it does not claim transaction correctness.
func Environment() []testutils.Scenario {
	return []testutils.Scenario{{Name: "requires reachable dedicated database and exact identity", Run: func(t *testing.T) {
		cfg, err := fixtures.LoadConfig(os.LookupEnv)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		db, err := fixtures.Connect(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error("PostgreSQL test pool cleanup failed")
			}
		})
	}}}
}
