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
	ProfileWriter struct {
		Db  *sql.DB
		Tx  *orm.SqlTransactionAdapter
		ctx context.Context
	}
)

func NewProfileWriter(ctx context.Context) *ProfileWriter {
	conn := database.DBGetNativePool(database.UserDbSource)
	return &ProfileWriter{
		Db:  conn,
		ctx: ctx,
	}
}

func (p *ProfileWriter) Model() domain.ProfileRequest {
	return domain.ProfileRequest{}
}
func (p *ProfileWriter) Begin() (*orm.SqlTransactionAdapter, error) {
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

func (p *ProfileWriter) UseTransaction(tx *orm.SqlTransactionAdapter) {
	p.Tx = tx
}

func (p *ProfileWriter) Create(req *domain.ProfileRequest) error {
	err := p.Tx.Create(req)
	if er := dictionary.HandleDBError(err); er != nil {
		return er
	}

	return nil
}

func (p *ProfileWriter) Update(req *domain.UpdateProfileRequest) error {
	err := p.Tx.Update(req)
	if er := dictionary.HandleDBError(err); er != nil {
		return er
	}

	return nil
}

func (p *ProfileWriter) Patch(req map[string]any) error {
	err := p.Tx.Update(p.Model(), req)
	if er := dictionary.HandleDBError(err); er != nil {
		return er
	}

	return nil
}
