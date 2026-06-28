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
	RoleWriter struct {
		Db  *sql.DB
		Tx  *orm.SqlTransactionAdapter
		ctx context.Context
	}
)

func NewRoleWriter(ctx context.Context) *RoleWriter {
	conn := database.DBGetNativePool(database.UserDbSource)
	return &RoleWriter{
		Db:  conn,
		ctx: ctx,
	}
}

func (p *RoleWriter) Model() domain.RoleRequest {
	return domain.RoleRequest{}
}
func (p *RoleWriter) Begin() (*orm.SqlTransactionAdapter, error) {
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

func (p *RoleWriter) UseTransaction(tx *orm.SqlTransactionAdapter) {
	p.Tx = tx
}

func (p *RoleWriter) Create(req *domain.RoleRequest) error {
	err := p.Tx.Create(req)
	if er := dictionary.HandleDBError(err); er != nil {
		return er
	}

	return nil
}
