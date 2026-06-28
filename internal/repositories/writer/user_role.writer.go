package writer

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
	"github.com/siti-nabila/orm/orm"
	ormLog "github.com/siti-nabila/orm/pkg/logger"
)

type (
	UserRoleWriter struct {
		Db  *sql.DB
		Tx  *orm.SqlTransactionAdapter
		ctx context.Context
	}
)

func NewUserRoleWriter(ctx context.Context) domain.UserRoleWriter {
	conn := database.DBGetNativePool(database.UserDbSource)
	return &UserRoleWriter{
		Db:  conn,
		ctx: ctx,
	}
}

func (p *UserRoleWriter) Model() domain.UserRoleRequest {
	return domain.UserRoleRequest{}
}
func (p *UserRoleWriter) Begin() (*orm.SqlTransactionAdapter, error) {
	if p.Tx != nil {
		return p.Tx, nil
	}
	tx, err := p.Db.Begin()
	if err != nil {
		return nil, err
	}
	ormCfg := database.GetORMConfig()

	oTx := orm.NewSqlTransactionAdapter(p.ctx, tx, database.GetDialect(database.UserDbSource), ormCfg)
	oTx.SetLogger(ormLog.DefaultLogger{}, ormCfg.EnableDebug)

	p.Tx = oTx
	return p.Tx, nil
}

func (p *UserRoleWriter) UseTransaction(tx *orm.SqlTransactionAdapter) {
	p.Tx = tx
}

func (p *UserRoleWriter) Create(req *domain.UserRoleRequest) error {
	err := p.Tx.Create(req)
	if er := dictionary.HandleDBError(err); er != nil {
		return er
	}

	return nil
}
