package database

import (
	"context"
	"database/sql"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	ormconfig "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

type TransactionProvider struct {
	db      *sql.DB
	dialect dialect.Dialector
	config  ormconfig.Config
}

func NewTransactionProvider(db *sql.DB, d dialect.Dialector, cfg ormconfig.Config) *TransactionProvider {
	// A transaction can contain credentials. Never enable query logging here.
	cfg.EnableDebug = false
	return &TransactionProvider{db: db, dialect: d, config: cfg}
}

func (p *TransactionProvider) Begin(ctx context.Context) (domain.Transaction, error) {
	return p.begin(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

func (p *TransactionProvider) BeginSnapshot(ctx context.Context) (domain.Transaction, error) {
	return p.begin(ctx, sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func (p *TransactionProvider) begin(ctx context.Context, options sql.TxOptions) (domain.Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.db == nil {
		return nil, common.ErrDependencyUnavailable
	}
	// Keep a connection lease until Commit/Rollback has finished. On context
	// cancellation database/sql can mark a Tx done before its driver's automatic
	// rollback completes; closing this lease waits for that cleanup.
	conn, err := p.db.Conn(ctx)
	if err != nil {
		return nil, transactionBeginError(err)
	}
	tx, err := conn.BeginTx(ctx, &options)
	if err != nil {
		_ = conn.Close()
		return nil, transactionBeginError(err)
	}
	return &leasedTransaction{SqlTransactionAdapter: orm.NewSqlTransactionAdapter(ctx, tx, p.dialect, p.config), native: tx, conn: conn}, nil
}

func transactionBeginError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.Join(common.ErrDependencyUnavailable, err)
}

// Feature ownership is unchanged: this technical adapter only completes the
// requested finalization and returns its existing connection to the shared pool.
type leasedTransaction struct {
	*orm.SqlTransactionAdapter
	native *sql.Tx
	conn   *sql.Conn
}

func (t *leasedTransaction) Commit() error {
	err := t.SqlTransactionAdapter.Commit()
	if err != nil {
		err = errors.Join(err, completedTransactionError(t.native.Rollback()))
	}
	return errors.Join(err, completedConnectionError(t.conn.Close()))
}

func (t *leasedTransaction) Rollback() error {
	err := t.SqlTransactionAdapter.Rollback()
	// Also guarantee native cleanup if an ORM dialect's lock release failed.
	err = errors.Join(err, completedTransactionError(t.native.Rollback()))
	return errors.Join(err, completedConnectionError(t.conn.Close()))
}

func completedTransactionError(err error) error {
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}

func completedConnectionError(err error) error {
	if errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	return err
}
