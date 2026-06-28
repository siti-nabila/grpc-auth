package reader

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/orm/orm"
)

type (
	userRoleReader struct {
		Db  *sql.DB
		ctx context.Context
		// DB  *sql.DB
	}
)

func NewUserRoleReader(ctx context.Context) domain.UserRoleReader {
	conn := database.DBGetNativePool(database.UserDbSource)

	return &userRoleReader{
		ctx: ctx,
		Db:  conn,
	}

}

func (a *userRoleReader) Adapter() *orm.SqlQueryAdapter {
	if a.Db == nil {
		a.Db = database.DBGetNativePool(database.UserDbSource)
	}

	ormCfg := database.GetORMConfig()
	dialect := database.GetDialect(database.UserDbSource)
	return orm.NewSqlQueryAdapter(a.ctx, a.Db, dialect, ormCfg)
}

func (a *userRoleReader) Model() domain.UserRole {
	return domain.UserRole{}
}

func (a *userRoleReader) GetRolesByUserId(id uint64) (results []domain.Role, err error) {
	db := a.Adapter()
	err = db.
		UseModel(a.Model()).
		Select("r.id", "r.role_name", "r.role_code", "r.role_description").
		Join("role r", "r.id = user_role.role_id").
		Where("user_role.user_id = ?", id).
		Scan(&results)
	if err != nil {
		return results, err
	}

	return results, nil

}
