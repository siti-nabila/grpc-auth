package reader

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/orm/orm"
)

type (
	roleReader struct {
		Db  *sql.DB
		ctx context.Context
		// DB  *sql.DB
	}
)

func NewRoleReader(ctx context.Context) domain.RoleReader {
	conn := database.DBGetNativePool(database.UserDbSource)

	return &roleReader{
		ctx: ctx,
		Db:  conn,
	}

}

func (a *roleReader) Adapter() *orm.SqlQueryAdapter {
	if a.Db == nil {
		a.Db = database.DBGetNativePool(database.UserDbSource)
	}

	ormCfg := database.GetORMConfig()
	dialect := database.GetDialect(database.UserDbSource)
	return orm.NewSqlQueryAdapter(a.ctx, a.Db, dialect, ormCfg)
}

func (a *roleReader) Model() domain.Role {
	return domain.Role{}
}

func (a *roleReader) GetDefaultRole() (result domain.Role, err error) {
	db := a.Adapter()
	err = db.
		UseModel(a.Model()).
		Where("role_code = ?", domain.MemberRole).
		Limit(1).
		Scan(&result)
	if err != nil {
		return result, err
	}

	return result, nil

}
