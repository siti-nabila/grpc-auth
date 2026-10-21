//go:build postgres

package test_scenarios

import (
	"database/sql"
	"encoding/json"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"os"
	"strings"
	"testing"
)

func AssignmentSchema() []testutils.Scenario {
	return []testutils.Scenario{{Name: "assignment metadata parity with archived read-only B baseline and owned invariants", Run: assignmentSchema}}
}

func assignmentSchema(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	b, e := os.ReadFile("../../../openspec/changes/archive/2026-10-12-auth-role-management/schema-comparison.json")
	if e != nil {
		t.Fatal(e)
	}
	var baseline struct {
		Baseline struct {
			Columns []struct {
				Table    string `json:"table_name"`
				Column   string `json:"column_name"`
				Type     string `json:"data_type"`
				Nullable string `json:"is_nullable"`
				Length   *int64 `json:"character_maximum_length"`
			}
			Constraints []struct {
				Table      string `json:"table_name"`
				Name       string `json:"name"`
				Definition string `json:"definition"`
			}
		} `json:"baseline_read_only"`
	}
	if e = json.Unmarshal(b, &baseline); e != nil {
		t.Fatal(e)
	}
	columns, constraints := 0, 0
	for _, c := range baseline.Baseline.Columns {
		if c.Table != "user_role" {
			continue
		}
		columns++
		var typ, nullable string
		var length sql.NullInt64
		e = f.DB.QueryRowContext(ctx, `SELECT data_type,is_nullable,character_maximum_length FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND column_name=$3`, f.Name, c.Table, c.Column).Scan(&typ, &nullable, &length)
		if e != nil || typ != c.Type || nullable != c.Nullable || (c.Length != nil) != length.Valid || c.Length != nil && *c.Length != length.Int64 {
			t.Fatal("assignment column parity", c.Column, e)
		}
	}
	for _, c := range baseline.Baseline.Constraints {
		if c.Table != "user_role" {
			continue
		}
		constraints++
		var definition string
		e = f.DB.QueryRowContext(ctx, `SELECT pg_get_constraintdef(k.oid) FROM pg_constraint k JOIN pg_class r ON r.oid=k.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=$1 AND r.relname=$2 AND k.conname=$3 AND k.convalidated`, f.Name, c.Table, c.Name).Scan(&definition)
		equivalent := c.Name == "user_role_status_ck" && definition == "CHECK (((status)::text = ANY (ARRAY[('ACTIVE'::character varying)::text, ('REVOKED'::character varying)::text])))"
		if equivalent {
			t.Log("status constraint has equivalent per-element casts versus archived array cast; other constraints remain exact")
		}
		if e != nil || definition != c.Definition && !equivalent {
			t.Fatal("assignment constraint parity", c.Name, e, "actual", definition, "baseline", c.Definition)
		}
	}
	if columns != 9 || constraints != 8 {
		t.Fatal("incomplete historical baseline", columns, constraints)
	}
	for _, check := range []struct{ table, name, text string }{{"user_role", "user_role_pair_uq", "UNIQUE (user_id, role_id)"}, {"user_role", "user_role_revocation_ck", "revoked_at IS NOT NULL"}, {"auth", "auth_version_ck", "version >= 1"}, {"admin_guard", "admin_guard_id_check", "id = 1"}} {
		var text string
		e = f.DB.QueryRowContext(ctx, `SELECT pg_get_constraintdef(k.oid) FROM pg_constraint k JOIN pg_class r ON r.oid=k.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=$1 AND r.relname=$2 AND k.conname=$3`, f.Name, check.table, check.name).Scan(&text)
		if e != nil || !strings.Contains(text, check.text) {
			t.Fatal("owned invariant", check.name, e)
		}
	}
	var ids []int
	rows, e := f.DB.QueryContext(ctx, "SELECT id FROM admin_guard")
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var id int
		if e = rows.Scan(&id); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil || len(ids) != 1 || ids[0] != 1 {
		t.Fatal("owned admin singleton", ids, e)
	}
}
