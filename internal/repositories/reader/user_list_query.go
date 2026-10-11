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
		domain.UserSortRoles:             "(SELECT MIN(r.role_name) FROM user_role ur JOIN role r ON r.id = ur.role_id WHERE ur.user_id = a.id AND ur.status = '" + string(types.AssignmentActive) + "' AND r.status = '" + string(types.RoleActive) + "')",
		domain.UserSortApplicationAccess: "(SELECT MIN(app.name) FROM user_application_access ua JOIN applications app ON app.id = ua.application_id WHERE ua.user_id = a.id AND ua.status = '" + string(types.ApplicationAccessActive) + "' AND app.status = '" + string(types.ApplicationActive) + "')",
	}
}
func userListProjection() []string {
	e := userSortExpressions()
	return []string{"a.id AS auth_id", "a.email", `COALESCE(p."name", '') AS name`, "COALESCE(p.address, '') AS address", "COALESCE(p.phone, '') AS phone", "a.status", "a.version", "a.created_at", `p."name" AS key_name`, "p.address AS key_address", "p.phone AS key_phone", e[domain.UserSortRoles] + " AS key_roles", e[domain.UserSortApplicationAccess] + " AS key_application_access"}
}
func buildUserListQuery(db domain.QueryReader, f domain.UserListFilter) *orm.QueryBuilder {
	q := db.UseModel(domain.UserSearchRow{}).LeftJoin("profile p", "p.user_id = a.id").LeftJoin("user_profile_search ups", "ups.profile_id = p.id")
	if f.CreatedFrom != nil {
		q = q.Where("a.created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		q = q.Where("a.created_at < ?", *f.CreatedTo)
	}
	if len(f.RoleCodes) > 0 {
		args := []any{types.AssignmentActive, types.RoleActive}
		for _, id := range f.RoleCodes {
			args = append(args, id)
		}
		q = q.Where("EXISTS (SELECT 1 FROM user_role ur JOIN role r ON r.id = ur.role_id WHERE ur.user_id = a.id AND ur.status = ? AND r.status = ? AND r.role_code IN ("+placeholders(len(f.RoleCodes))+"))", args...)
	}
	if len(f.Statuses) > 0 {
		q = q.WhereIn("a.status", f.Statuses)
	}
	if f.ApplicationCode != "" {
		q = q.Where("EXISTS (SELECT 1 FROM user_application_access ua JOIN applications app ON app.id = ua.application_id WHERE ua.user_id = a.id AND ua.status = ? AND app.status = ? AND app.code = ?)", types.ApplicationAccessActive, types.ApplicationActive, f.ApplicationCode)
	}
	return q
}
func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }
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
	var count struct {
		Total int64 `sql:"column:total"`
	}
	err = userSearch(buildUserListQuery(db, f), opts.Search).WithContext(ctx).Select("COUNT(DISTINCT a.id) AS total").Scan(&count)
	if err != nil {
		return page, userPageError(err)
	}
	q := applyUserCursor(userSearch(buildUserListQuery(db, f), opts.Search), opts.Sort, values).WithContext(ctx).Select(userListProjection()...)
	for _, s := range opts.Sort {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		q = q.OrderBy(userSortExpressions()[s.Field] + " " + dir + " NULLS LAST")
	}
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
	page.Items = rows
	page.Total = count.Total
	page.Page = opts.Page
	page.Limit = opts.Limit
	page.HasPrev = opts.Page > 1
	page.TotalPages = int(count.Total / int64(opts.Limit))
	if count.Total%int64(opts.Limit) > 0 {
		page.TotalPages++
	}
	if len(rows) > 0 {
		page.NextCursor = domain.UserListNextCursor(rows[len(rows)-1], opts, f)
	}
	return page, nil
}
