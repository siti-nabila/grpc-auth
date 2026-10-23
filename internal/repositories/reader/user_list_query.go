package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/orm/orm"
	"strconv"
	"strings"
	"time"
)

// Both collection keys use precisely the same ACTIVE predicate as batch reads.
func userSortExpressions() map[string]string {
	return map[string]string{
		domain.UserSortID: "a.id", domain.UserListCursorField: "a.id", domain.UserSortEmail: "a.email", domain.UserSortName: `p."name"`, domain.UserSortAddress: "p.address", domain.UserSortPhone: "p.phone", domain.UserSortStatus: "a.status", domain.UserSortVersion: "a.version", domain.UserSortCreatedAt: "a.created_at",
		domain.UserSortRoles:             "role_keys.first_name",
		domain.UserSortApplicationAccess: "application_keys.first_name",
	}
}
func userListProjection() []string {
	e := userSortExpressions()
	return []string{"a.id AS auth_id", "a.email", `COALESCE(p."name", '') AS name`, "COALESCE(p.address, '') AS address", "COALESCE(p.phone, '') AS phone", "COALESCE(p.country_code, '') AS country_code", "a.status", "a.version", "a.created_at", `p."name" AS key_name`, "p.address AS key_address", "p.phone AS key_phone", e[domain.UserSortRoles] + " AS key_roles", e[domain.UserSortApplicationAccess] + " AS key_application_access"}
}
func buildUserListQuery(db domain.QueryReader, f domain.UserListFilter) *orm.QueryBuilder {
	roles := db.UseModel(domain.UserListRole{}).Select("ur.user_id", "MIN(r.role_name) AS first_name").Join("role r", "r.id = ur.role_id").Where("ur.status = 'ACTIVE' AND r.status = 'ACTIVE'").GroupBy("ur.user_id")
	apps := db.UseModel(domain.UserListApplication{}).Select("ua.user_id", "MIN(app.name) AS first_name").Join("applications app", "app.id = ua.application_id").Where("ua.status = 'ACTIVE' AND app.status = 'ACTIVE'").GroupBy("ua.user_id")
	q := db.UseModel(domain.UserSearchRow{}).LeftJoin("profile p", "p.user_id = a.id").LeftJoin("user_profile_search ups", "ups.profile_id = p.id").LeftJoinQuery(roles, "role_keys", "role_keys.user_id = a.id").LeftJoinQuery(apps, "application_keys", "application_keys.user_id = a.id")
	if f.CreatedFrom != nil {
		q = q.Where("a.created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		q = q.Where("a.created_at < ?", *f.CreatedTo)
	}
	if len(f.RoleCodes) > 0 {
		q = q.WhereExists(db.UseModel(domain.UserListRole{}).Select("ur.user_id").Join("role r", "r.id = ur.role_id").Where("ur.user_id = a.id").Where("ur.status = ? AND r.status = ?", types.AssignmentActive, types.RoleActive).WhereIn("r.role_code", f.RoleCodes))
	}
	if len(f.Statuses) > 0 {
		q = q.WhereIn("a.status", f.Statuses)
	}
	if f.ApplicationCode != "" {
		q = q.WhereExists(db.UseModel(domain.UserListApplication{}).Select("ua.user_id").Join("applications app", "app.id = ua.application_id").Where("ua.user_id = a.id").Where("ua.status = ? AND app.status = ? AND app.code = ?", types.ApplicationAccessActive, types.ApplicationActive, f.ApplicationCode))
	}
	return q
}
func userSearch(q *orm.QueryBuilder, search *orm.SearchQuery) *orm.QueryBuilder {
	if search == nil {
		return q
	}
	if search.Mode == orm.SearchModeFullText {
		return q.Where("ups.fts_keyword @@ websearch_to_tsquery('simple', ?)", search.Keyword)
	}
	tokens := strings.Fields(strings.ToLower(search.Keyword))
	for i := range tokens {
		tokens[i] += ":*"
	}
	return q.Where("(ups.fts_keyword @@ to_tsquery('simple', ?) OR ups.fts_lexeme_text ILIKE '%' || lower(?) || '%')", strings.Join(tokens, " & "), strings.Join(strings.Fields(search.Keyword), " "))
}
func userCursorValues(opts orm.QueryOptions) ([]domain.UserSortValue, error) {
	if opts.InMemoryOffset == nil {
		return nil, nil
	}
	switch c := opts.InMemoryOffset.Cursor.Value.(type) {
	case string:
		if c == "" {
			return nil, nil
		}
	case int64:
		if c > 0 && len(opts.Sort) == 1 && opts.Sort[0].Field == domain.UserSortID {
			text := strconv.FormatInt(c, 10)
			return []domain.UserSortValue{{Kind: "integer", Value: &text}}, nil
		}
	case domain.UserListCursor:
		if len(c.Values) == len(opts.Sort) {
			return c.Values, nil
		}
	}
	return nil, common.ErrBadRequest
}
func userCursorArgument(v domain.UserSortValue) any {
	if v.Value == nil {
		return nil
	}
	switch v.Kind {
	case "integer":
		n, _ := strconv.ParseInt(*v.Value, 10, 64)
		return n
	case "time":
		t, _ := time.Parse(time.RFC3339Nano, *v.Value)
		return t
	default:
		return *v.Value
	}
}
func applyUserCursor(q *orm.QueryBuilder, sorts []orm.SortField, values []domain.UserSortValue) *orm.QueryBuilder {
	if len(values) == 0 {
		return q
	}
	expressions := userSortExpressions()
	alternatives := []string{}
	args := []any{}
	for i, s := range sorts {
		if values[i].Value == nil {
			continue
		}
		terms := []string{}
		for j := 0; j < i; j++ {
			field := expressions[sorts[j].Field]
			if values[j].Value == nil {
				terms = append(terms, field+" IS NULL")
			} else {
				terms = append(terms, field+" = ?")
				args = append(args, userCursorArgument(values[j]))
			}
		}
		field := expressions[s.Field]
		op := ">"
		if s.Desc {
			op = "<"
		}
		terms = append(terms, "("+field+" "+op+" ? OR "+field+" IS NULL)")
		args = append(args, userCursorArgument(values[i]))
		alternatives = append(alternatives, "("+strings.Join(terms, " AND ")+")")
	}
	return q.Where("("+strings.Join(alternatives, " OR ")+")", args...)
}
func readUserPage(ctx context.Context, db domain.QueryReader, opts orm.QueryOptions, f domain.UserListFilter) (page orm.PageData[domain.UserSearchRow], err error) {
	if err = ctx.Err(); err != nil {
		return page, err
	}
	if db == nil {
		return page, common.ErrDependencyUnavailable
	}
	if opts.Limit < 1 || opts.Limit > 1000 {
		return page, common.ErrBadRequest
	}
	// Revalidate at the adapter boundary before resolving any SQL expressions.
	normalized, e := domain.NormalizeUserListSort(opts.Sort)
	if e != nil {
		return page, e
	}
	opts.Sort = normalized
	values, e := userCursorValues(opts)
	if e != nil {
		return page, e
	}
	total, err := countUsers(ctx, db, opts, f)
	if err != nil {
		return page, err
	}
	q := userPageQuery(ctx, db, opts, f, values)
	rows := make([]domain.UserSearchRow, 0, opts.Limit+1)
	err = q.Limit(opts.Limit + 1).Scan(&rows)
	if err != nil {
		return page, userPageError(err)
	}
	page.HasNext = len(rows) > opts.Limit
	if page.HasNext {
		rows = rows[:opts.Limit]
	}
	if err = readUserCollections(ctx, db, rows); err != nil {
		return page, err
	}
	return userPageMetadata(page, rows, total, opts, f), nil
}

func countUsers(ctx context.Context, db domain.QueryReader, opts orm.QueryOptions, f domain.UserListFilter) (int64, error) {
	var count struct {
		Total int64 `sql:"column:total"`
	}
	err := userSearch(buildUserListQuery(db, f), opts.Search).WithContext(ctx).Select("COUNT(DISTINCT a.id) AS total").Scan(&count)
	if err != nil {
		return 0, userPageError(err)
	}
	return count.Total, nil
}

func userPageQuery(ctx context.Context, db domain.QueryReader, opts orm.QueryOptions, f domain.UserListFilter, values []domain.UserSortValue) *orm.QueryBuilder {
	q := applyUserCursor(userSearch(buildUserListQuery(db, f), opts.Search), opts.Sort, values).WithContext(ctx).Select(userListProjection()...)
	for _, s := range opts.Sort {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		q = q.OrderBy(userSortExpressions()[s.Field] + " " + dir + " NULLS LAST")
	}
	return q
}

func userPageMetadata(page orm.PageData[domain.UserSearchRow], rows []domain.UserSearchRow, total int64, opts orm.QueryOptions, f domain.UserListFilter) orm.PageData[domain.UserSearchRow] {
	page.Items = rows
	page.Total = total
	page.Page = opts.Page
	page.Limit = opts.Limit
	page.HasPrev = opts.Page > 1
	page.TotalPages = int(total / int64(opts.Limit))
	if total%int64(opts.Limit) > 0 {
		page.TotalPages++
	}
	if len(rows) > 0 {
		page.NextCursor = domain.UserListNextCursor(rows[len(rows)-1], opts, f)
	}
	return page
}
