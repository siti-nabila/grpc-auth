package reader

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/orm/orm"
)

type (
	userReader struct {
		Db  *sql.DB
		ctx context.Context
	}
)

func NewUserReader(ctx context.Context) domain.UserReader {
	conn := database.DBGetNativePool(database.UserDbSource)
	return &userReader{
		ctx: ctx,
		Db:  conn,
	}
}

func (u *userReader) Adapter() *orm.SqlQueryAdapter {
	if u.Db == nil {
		u.Db = database.DBGetNativePool(database.UserDbSource)
	}

	ormCfg := database.GetORMConfig()
	dialect := database.GetDialect(database.UserDbSource)
	return orm.NewSqlQueryAdapter(u.ctx, u.Db, dialect, ormCfg)
}

func (u *userReader) Model() domain.UserSearchRow {
	return domain.UserSearchRow{}
}

func (u *userReader) AllowedFields() map[string]string {
	return map[string]string{
		"auth_id": "a.id",
		"email":   "a.email",
		"name":    `p."name"`,
		"phone":   "p.phone",
	}
}

func (u *userReader) SearchFields() map[string]orm.SearchFieldConfig {
	return map[string]orm.SearchFieldConfig{
		"keyword": {
			Column:           `ups.fts_lexeme_text`,
			FullTextColumn:   "ups.fts_keyword",
			FullTextLanguage: orm.FullTextSimple,
			Modes: []orm.SearchMode{
				orm.SearchModeFullText,
				orm.SearchModeFullTextTrigram,
			},
		},
	}
}

func (u *userReader) SearchUsers(opts orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error) {
	db := u.Adapter()
	rows := make([]domain.UserSearchRow, 0)

	query := db.UseModel(u.Model()).
		Select(
			"a.id AS auth_id",
			"a.email",
			`p."name"`,
			"p.address",
			"p.phone",
		).
		Join("auth a", "a.id = p.user_id").
		Join("user_profile_search ups", "ups.profile_id = p.id")

	return orm.QueryPageWithConfig(
		u.ctx,
		query,
		&rows,
		orm.QueryPageConfig{
			AllowedFields: u.AllowedFields(),
			SearchFields:  u.SearchFields(),
		},
		opts,
	)
}
