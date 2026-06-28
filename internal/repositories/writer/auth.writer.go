package writer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
	"github.com/siti-nabila/orm/orm"
	ormLog "github.com/siti-nabila/orm/pkg/logger"
)

type (
	AuthWriter struct {
		Db  *sql.DB
		Tx  *orm.SqlTransactionAdapter
		ctx context.Context
	}
)

func NewAuthWriter(ctx context.Context) domain.AuthWriter {
	// dbLogger := database.NewDBLogger()
	fmt.Println("---------- auth writer new ----------")

	conn := database.DBGetNativePool(database.UserDbSource)
	// dbLogger.Adapter(conn)
	return &AuthWriter{
		Db:  conn,
		ctx: ctx,
	}
}

func (a *AuthWriter) Begin() (*orm.SqlTransactionAdapter, error) {
	if a.Tx != nil {
		return a.Tx, nil
	}
	tx, err := a.Db.Begin()
	if err != nil {
		return nil, err
	}
	ormCfg := database.GetORMConfig()
	oTx := orm.NewSqlTransactionAdapter(a.ctx, tx, database.GetDialect(database.UserDbSource), ormCfg)
	oTx.SetLogger(ormLog.DefaultLogger{}, ormCfg.EnableDebug)
	a.Tx = oTx

	return a.Tx, nil
}

func (a *AuthWriter) UseTransaction(tx *orm.SqlTransactionAdapter) {
	a.Tx = tx
}

func (a *AuthWriter) Create(req *domain.AuthRequest) error {
	err := a.Tx.Create(req)
	if er := dictionary.HandleDBError(err); er != nil {
		return er
	}

	return nil
}
