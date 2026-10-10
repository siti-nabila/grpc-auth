//go:build postgres

package fixtures

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"

	"github.com/lib/pq"
)

const (
	EnabledVariable  = "GRPC_AUTH_POSTGRES_TEST_ENABLED"
	DSNVariable      = "GRPC_AUTH_POSTGRES_TEST_DSN"
	DatabaseVariable = "GRPC_AUTH_POSTGRES_TEST_DATABASE"
)

type Config struct {
	DSN      string
	Database string
}

// LoadConfig only accepts explicit test configuration; it never reads env.yaml.
func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	enabled, _ := lookup(EnabledVariable)
	dsn, _ := lookup(DSNVariable)
	name, _ := lookup(DatabaseVariable)
	if enabled != "1" {
		return Config{}, errors.New("PostgreSQL test requires explicit opt-in")
	}
	if strings.TrimSpace(dsn) == "" || strings.TrimSpace(name) == "" {
		return Config{}, errors.New("PostgreSQL test requires a dedicated test DSN and expected database")
	}
	if name != strings.TrimSpace(name) || name == "postgres" || name == "template0" || name == "template1" || name == "user" {
		return Config{}, errors.New("PostgreSQL test database must be dedicated; application and system databases are forbidden")
	}
	if _, err := pq.NewConnector(dsn); err != nil {
		return Config{}, errors.New("PostgreSQL test DSN is invalid")
	}
	parsed, err := dsnDatabase(dsn)
	if err != nil || parsed != name {
		return Config{}, errors.New("PostgreSQL test DSN must explicitly select the expected dedicated database")
	}
	return Config{DSN: dsn, Database: name}, nil
}

func ValidateDatabase(cfg Config, actual string) error {
	if cfg.Database == "" || actual != cfg.Database {
		return errors.New("PostgreSQL test connected to an unexpected database")
	}
	return nil
}

// Connect verifies identity in a read-only transaction before returning a pool.
// Callers must close the pool, and may only mutate their own fixture schema.
func Connect(ctx context.Context, cfg Config) (*sql.DB, error) {
	// Validate even when the caller constructs Config directly.
	checked, err := LoadConfig(func(key string) (string, bool) {
		switch key {
		case EnabledVariable:
			return "1", true
		case DSNVariable:
			return cfg.DSN, true
		case DatabaseVariable:
			return cfg.Database, true
		default:
			return "", false
		}
	})
	if err != nil {
		return nil, err
	}
	connector, err := pq.NewConnector(checked.DSN)
	if err != nil {
		return nil, errors.New("PostgreSQL test DSN is invalid")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		_ = db.Close()
		return nil, errors.New("PostgreSQL test database connection failed")
	}
	var actual string
	err = tx.QueryRowContext(ctx, "SELECT current_database()").Scan(&actual)
	cleanupErr := tx.Rollback()
	if err != nil || cleanupErr != nil {
		_ = db.Close()
		return nil, errors.New("PostgreSQL test database identity check failed")
	}
	if err = ValidateDatabase(checked, actual); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Parse only after pq validates the DSN. Last duplicate keyword wins, as in pq.
// Requiring dbname prevents PGDATABASE/user defaults from selecting an app DB.
func dsnDatabase(dsn string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := pq.ParseURL(dsn)
		if err != nil {
			return "", errors.New("PostgreSQL test DSN is invalid")
		}
		dsn = parsed
	}
	input := []rune(dsn)
	name := ""
	for i := 0; i < len(input); {
		for i < len(input) && unicode.IsSpace(input[i]) {
			i++
		}
		start := i
		for i < len(input) && input[i] != '=' && !unicode.IsSpace(input[i]) {
			i++
		}
		key := string(input[start:i])
		for i < len(input) && unicode.IsSpace(input[i]) {
			i++
		}
		if i >= len(input) || input[i] != '=' {
			return "", errors.New("PostgreSQL test DSN is invalid")
		}
		i++
		for i < len(input) && unicode.IsSpace(input[i]) {
			i++
		}
		quoted := i < len(input) && input[i] == '\''
		if quoted {
			i++
		}
		var value strings.Builder
		for i < len(input) {
			ch := input[i]
			if ch == '\\' && i+1 < len(input) {
				i++
				value.WriteRune(input[i])
				i++
				continue
			}
			if quoted && ch == '\'' {
				i++
				break
			}
			if !quoted && unicode.IsSpace(ch) {
				break
			}
			value.WriteRune(ch)
			i++
		}
		if key == "dbname" {
			name = value.String()
		}
		for i < len(input) && unicode.IsSpace(input[i]) {
			i++
		}
	}
	return name, nil
}
