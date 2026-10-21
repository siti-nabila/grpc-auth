//go:build postgres

package test_scenarios

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	"github.com/siti-nabila/orm/dialect"
	"testing"
)

type observedProvider struct {
	inner domain.TransactionProvider
	pid   int
}

func (p *observedProvider) Begin(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.inner.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var row struct {
		PID int `sql:"column:pid"`
	}
	if err = tx.UseModel(domain.Auth{}).Select("pg_backend_pid() AS pid").Limit(1).Scan(&row); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	p.pid = row.PID
	return tx, nil
}

func productionProvider(fixture *fixtures.Schema) domain.TransactionProvider {
	return database.NewTransactionProvider(fixture.DB, dialect.NewPostgres(), database.GetORMConfig())
}

func assertRegistrationCounts(t *testing.T, ctx context.Context, fixture *fixtures.Schema, email string, want int) {
	t.Helper()
	var accounts, profiles, assignments int
	err := fixture.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM auth WHERE email=$1),(SELECT count(*) FROM profile p JOIN auth a ON a.id=p.user_id WHERE a.email=$1),(SELECT count(*) FROM user_role ur JOIN auth a ON a.id=ur.user_id WHERE a.email=$1)`, email).Scan(&accounts, &profiles, &assignments)
	if err != nil || accounts != want || profiles != want || assignments != want {
		t.Fatalf("atomic row counts %d/%d/%d want %d", accounts, profiles, assignments, want)
	}
}

type commitFailureProvider struct {
	inner domain.TransactionProvider
	err   error
}

func (p commitFailureProvider) Begin(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.inner.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return commitFailureTransaction{Transaction: tx, err: p.err}, nil
}

type commitFailureTransaction struct {
	domain.Transaction
	err error
}

func (t commitFailureTransaction) Commit() error { return t.err }
