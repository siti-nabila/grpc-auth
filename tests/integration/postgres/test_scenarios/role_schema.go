//go:build postgres

package test_scenarios

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func RoleSchema() []testutils.Scenario {
	return []testutils.Scenario{{Name: "nullable expansion preserves pairs and rejects incompatible columns", Run: postgresMappingMigration}, {Name: "role catalog metadata matches baseline bounds and constraints", Run: roleSchema}}
}

func roleSchema(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	for _, c := range []struct {
		table, field, typ, nullable string
		length                      int64
	}{
		{"role", "id", "integer", "NO", 0},
		{"role", "role_name", "character varying", "NO", 50},
		{"role", "role_description", "character varying", "YES", 100},
		{"role", "code", "character varying", "NO", 100},
		{"role", "role_code", "integer", "NO", 0},
		{"role", "version", "bigint", "NO", 0},
		{"role", "created_at", "timestamp with time zone", "YES", 0},
		{"role", "updated_at", "timestamp with time zone", "YES", 0},
		{"applications", "code", "character varying", "NO", 50},
		{"permissions", "code", "character varying", "NO", 150},
		{"permissions", "description", "text", "NO", 0},
		{"role_permissions", "role_id", "integer", "NO", 0},
		{"role_permissions", "permission_id", "integer", "NO", 0},
		{"role_permissions", "deleted_at", "timestamp with time zone", "YES", 0},
	} {
		var typ, nullable string
		var length sql.NullInt64
		err := f.DB.QueryRowContext(ctx, `SELECT data_type,is_nullable,character_maximum_length FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND column_name=$3`, f.Name, c.table, c.field).Scan(&typ, &nullable, &length)
		if err != nil || typ != c.typ || nullable != c.nullable || length.Int64 != c.length {
			t.Fatalf("schema mismatch for %s.%s", c.table, c.field)
		}
	}
	var typ, definition string
	var min, max, increment int64
	if err := f.DB.QueryRowContext(ctx, `SELECT data_type,min_value,max_value,increment_by FROM pg_sequences WHERE schemaname=$1 AND sequencename='role_numeric_code_seq'`, f.Name).Scan(&typ, &min, &max, &increment); err != nil || typ != "integer" || min != 1 || max != 2147483647 || increment != 1 {
		t.Fatal("numeric role sequence mismatch")
	}
	if err := f.DB.QueryRowContext(ctx, `SELECT column_default FROM information_schema.columns WHERE table_schema=$1 AND table_name='role' AND column_name='role_code'`, f.Name).Scan(&definition); err != nil || !strings.Contains(definition, "role_numeric_code_seq") {
		t.Fatal("numeric role code does not use its database sequence")
	}
	for _, c := range []struct{ table, name, fragment string }{
		{"role", "role_code_string_uq", "UNIQUE (code)"},
		{"role", "uq_role_code", "UNIQUE (role_code)"},
		{"role", "role_version_ck", "version >= 1"},
		{"role", "role_status_ck", "DISABLED"},
		{"role", "role_application_fk", "REFERENCES applications(id)"},
		{"permissions", "permissions_code_ck", "{2,}"},
		{"role_permissions", "role_permissions_pkey", "PRIMARY KEY (role_id, permission_id)"},
	} {
		err := f.DB.QueryRowContext(ctx, `SELECT pg_get_constraintdef(k.oid) FROM pg_constraint k JOIN pg_class r ON r.oid=k.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=$1 AND r.relname=$2 AND k.conname=$3 AND k.convalidated`, f.Name, c.table, c.name).Scan(&definition)
		if err != nil || !strings.Contains(definition, c.fragment) {
			t.Fatal("catalog constraint mismatch", c.name)
		}
	}
}
