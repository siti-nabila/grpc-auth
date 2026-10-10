package reader

import (
	"context"
	"database/sql"
	"errors"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/orm/orm"
	ormdictionary "github.com/siti-nabila/orm/pkg/dictionary"
)

type snapshotUserReader struct {
	ctx    context.Context
	handle domain.QueryReader
}

// NewUserReaderInSnapshot never obtains a pool connection. The feature owns
// the injected read transaction, including authorization and page queries.
func NewUserReaderInSnapshot(ctx context.Context, handle domain.QueryReader) domain.UserReader {
	return &snapshotUserReader{ctx: ctx, handle: handle}
}

func (r *snapshotUserReader) SearchUsers(opts orm.QueryOptions, filter domain.UserListFilter) (orm.PageData[domain.UserSearchRow], error) {
	if err := r.ctx.Err(); err != nil {
		return orm.PageData[domain.UserSearchRow]{}, err
	}
	if r.handle == nil {
		return orm.PageData[domain.UserSearchRow]{}, common.ErrDependencyUnavailable
	}
	rows := make([]domain.UserSearchRow, 0)
	fields := &userReader{}
	res, err := orm.QueryPageWithConfig(r.ctx, buildUserListQuery(r.handle, filter).WithContext(r.ctx), &rows,
		orm.QueryPageConfig{AllowedFields: fields.AllowedFields(), SearchFields: fields.SearchFields()}, opts)
	return res, userPageError(err)
}

func userPageError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// ORM returns both option validation and raw driver failures. Only known
	// frontend option errors are validation; a driver failure cannot become allow.
	for _, invalid := range []error{
		ormdictionary.ErrColumnNotFound, ormdictionary.ErrInvalidWhereOperator,
		ormdictionary.ErrInvalidSearchMode, ormdictionary.ErrUnsupportedSearchModeForDialect,
		ormdictionary.ErrInvalidFullTextLanguage, ormdictionary.ErrInvalidFullTextOperator,
		ormdictionary.ErrFullTextColumnRequired, ormdictionary.ErrSearchFieldNotAllowed,
		ormdictionary.ErrSearchModeNotAllowedForField, ormdictionary.ErrFullTextOperatorRequiresFullTextMode,
		ormdictionary.ErrPaginationCursorRequired, ormdictionary.ErrWhereValueNil,
		ormdictionary.ErrPaginationInvalidLimit, ormdictionary.ErrPaginationOffsetOverflow,
	} {
		if errors.Is(err, invalid) {
			return errors.Join(common.ErrBadRequest, err)
		}
	}
	return errors.Join(common.ErrDependencyUnavailable, err)
}

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

func (u *userReader) SearchUsers(
	opts orm.QueryOptions,
	filter domain.UserListFilter,
) (orm.PageData[domain.UserSearchRow], error) {
	db := u.Adapter()
	rows := make([]domain.UserSearchRow, 0)
	query := BuildUserListQuery(db, filter)

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

func BuildUserListQuery(
	db *orm.SqlQueryAdapter,
	filter domain.UserListFilter,
) *orm.QueryBuilder {
	if db == nil {
		return nil
	}
	return buildUserListQuery(db, filter)
}

func buildUserListQuery(db domain.QueryReader, filter domain.UserListFilter) *orm.QueryBuilder {

	query := db.UseModel(domain.UserSearchRow{}).
		Select(
			"a.id AS auth_id",
			"a.email",
			`p."name"`,
			"p.address",
			"p.phone",
		).
		Join("auth a", "a.id = p.user_id").
		Join("user_profile_search ups", "ups.profile_id = p.id")

	if filter.CreatedFrom != nil {
		query = query.Where("a.created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("a.created_at < ?", *filter.CreatedTo)
	}
	if len(filter.RoleCodes) > 0 {
		query = query.
			Join("user_role ur", "ur.user_id = a.id").
			Join("role r", "r.id = ur.role_id").
			WhereIn("r.role_code", filter.RoleCodes).
			Distinct()
	}

	return query
}
