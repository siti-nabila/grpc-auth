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
	tx, err := p.db.BeginTx(ctx, &options)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errors.Join(common.ErrDependencyUnavailable, err)
	}
	return orm.NewSqlTransactionAdapter(ctx, tx, p.dialect, p.config), nil
}
