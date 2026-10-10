//go:build postgres

package fixtures

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

//go:embed schema.sql
var schemaSQL string

var namespacePattern = regexp.MustCompile(`^grpc_auth_test_[a-z0-9_]+$`)

type Schema struct {
	ownedName  string
	DB         *sql.DB
	Name       string
	config     Config
	once       sync.Once
	cleanupErr error
}

func NewSchema(ctx context.Context, cfg Config, name string) (*Schema, error) {
	if len(name) > 63 || !namespacePattern.MatchString(name) {
		return nil, errors.New("PostgreSQL fixture namespace is invalid")
	}
	admin, err := Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer admin.Close()
	tx, err := admin.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.New("PostgreSQL fixture transaction failed")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE SCHEMA "`+name+`"`); err != nil {
		return nil, errors.New("PostgreSQL fixture schema creation failed; existing schema was not adopted")
	}
	if _, err = tx.ExecContext(ctx, "SELECT set_config('search_path', $1, true)", name+",pg_catalog"); err != nil {
		return nil, errors.New("PostgreSQL fixture setup failed")
	}
	if _, err = tx.ExecContext(ctx, schemaSQL); err != nil {
		return nil, errors.New("PostgreSQL fixture DDL failed")
	}
	if err = tx.Commit(); err != nil {
		return nil, errors.New("PostgreSQL fixture commit failed; inspect fixture namespace before retry")
	}
	// Ownership is acquired only after the CREATE transaction commits.
	fixture := &Schema{Name: name, ownedName: name, config: cfg}
	dsn := cfg.DSN
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		dsn, err = pq.ParseURL(dsn)
		if err != nil {
			return nil, errors.Join(errors.New("PostgreSQL fixture DSN is invalid"), fixture.cleanupAfterSetupFailure())
		}
	}
	// Startup option applies to every new/replacement physical connection.
	bound := cfg
	bound.DSN = dsn + " search_path='" + name + ",pg_catalog'"
	fixture.DB, err = Connect(ctx, bound)
	if err != nil {
		return nil, errors.Join(err, fixture.cleanupAfterSetupFailure())
	}
	var current string
	if err = fixture.DB.QueryRowContext(ctx, "SELECT current_schema()").Scan(&current); err != nil || current != name {
		return nil, errors.Join(errors.New("PostgreSQL fixture namespace verification failed"), fixture.cleanupAfterSetupFailure())
	}
	return fixture, nil
}

func (s *Schema) cleanupAfterSetupFailure() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.Close(ctx)
}

// Close drops only this successfully created namespace in the verified test DB.
func (s *Schema) Close(ctx context.Context) error {
	if !namespacePattern.MatchString(s.ownedName) || len(s.ownedName) > 63 {
		return errors.New("PostgreSQL fixture cleanup requires an owned namespace")
	}
	s.once.Do(func() {
		if s.DB != nil {
			if err := s.DB.Close(); err != nil {
				s.cleanupErr = errors.New("PostgreSQL fixture pool close failed")
			}
		}
		db, err := Connect(ctx, s.config)
		if err != nil {
			s.cleanupErr = errors.Join(s.cleanupErr, err)
			return
		}
		defer db.Close()
		if _, err = db.ExecContext(ctx, `DROP SCHEMA "`+s.ownedName+`" CASCADE`); err != nil {
			s.cleanupErr = errors.Join(s.cleanupErr, errors.New("PostgreSQL fixture namespace cleanup failed"))
		}
	})
	return s.cleanupErr
}
