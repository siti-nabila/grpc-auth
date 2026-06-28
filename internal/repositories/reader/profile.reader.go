package reader

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/orm/orm"
)

type (
	profileReader struct {
		Db  *sql.DB
		ctx context.Context
	}
)

func NewProfileReader(ctx context.Context) domain.ProfileReader {
	conn := database.DBGetNativePool(database.UserDbSource)
	return &profileReader{
		ctx: ctx,
		Db:  conn,
	}
}

func (p *profileReader) Adapter() *orm.SqlQueryAdapter {
	if p.Db == nil {
		p.Db = database.DBGetNativePool(database.UserDbSource)
	}

	ormCfg := database.GetORMConfig()
	dialect := database.GetDialect(database.UserDbSource)
	return orm.NewSqlQueryAdapter(p.ctx, p.Db, dialect, ormCfg)
}

func (p *profileReader) Model() domain.Profile {
	return domain.Profile{}
}

func (p *profileReader) GetByUserId(userId uint64) (result domain.Profile, err error) {
	db := p.Adapter()

	err = db.UseModel(p.Model()).
		Where("user_id = ?", userId).
		First(&result)
	if err != nil {
		return result, err
	}

	return result, nil
}
