package reader

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/orm"
)

type (
	authReader struct {
		Db  *sql.DB
		ctx context.Context
	}
)

func NewAuthReader(ctx context.Context) domain.AuthReader {
	conn := database.DBGetNativePool(database.UserDbSource)

	return &authReader{
		ctx: ctx,
		Db:  conn,
	}

}

func (a *authReader) Adapter() *orm.SqlQueryAdapter {
	if a.Db == nil {
		a.Db = database.DBGetNativePool(database.UserDbSource)
	}

	ormCfg := database.GetORMConfig()
	dialect := database.GetDialect(database.UserDbSource)
	return orm.NewSqlQueryAdapter(a.ctx, a.Db, dialect, ormCfg)
}

func (a *authReader) Model() domain.Auth {
	return domain.Auth{}
}

func (a *authReader) GetByEmail(email string) (result domain.Auth, err error) {
	db := a.Adapter()
	err = db.
		UseModel(a.Model()).
		Where("email = ?", email).
		Limit(1).
		Scan(&result)
	if er := helpers.HandleDBError(err); er != nil {
		return result, er
	}

	return result, nil

}

func (a *authReader) GetById(id uint64) (result domain.Auth, err error) {
	db := a.Adapter()

	err = db.
		UseModel(a.Model()).
		Where("id = ?", id).
		Limit(1).
		Scan(&result)
	if er := helpers.HandleDBError(err); er != nil {
		return result, er
	}

	return result, nil
}
