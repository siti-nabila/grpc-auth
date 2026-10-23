package reader

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/query"
)

type catalogReader struct{ handle domain.QueryReader }

func NewCatalogReader(handle domain.QueryReader) domain.CatalogReader { return &catalogReader{handle} }

func (r *catalogReader) Roles(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error) {
	page, err := readCatalog[domain.RoleSummary](ctx, r.handle, domain.RoleCatalog, q)
	for i, row := range page.Items {
		page.Items[i] = row.Project(q)
	}
	return page, err
}
func (r *catalogReader) Permissions(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error) {
	page, err := readCatalog[domain.PermissionItem](ctx, r.handle, domain.PermissionCatalog, q)
	for i, row := range page.Items {
		page.Items[i] = row.Project(q)
	}
	return page, err
}
func (r *catalogReader) Applications(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error) {
	page, err := readCatalog[domain.ApplicationItem](ctx, r.handle, domain.ApplicationCatalog, q)
	for i, row := range page.Items {
		page.Items[i] = row.Project(q)
	}
	return page, err
}

func catalogExpressions(c domain.Catalog) map[string]string {
	switch c {
	case domain.UserApplicationAccessCatalog:
		return accessExpressions()
	case domain.UserRoleAssignmentCatalog:
		return assignmentExpressions()
	case domain.RoleCatalog:
		return map[string]string{"id": "r.id", "code": "r.code", "name": "r.role_name", "description": "r.role_description", "application_id": "r.application_id", "application_code": "a.code", "status": "r.status", "is_system": "r.is_system", "version": "r.version", "role_code": "r.role_code", "created_at": "r.created_at", "updated_at": "r.updated_at", "affected_user_count": affectedRoleUsers}
	case domain.PermissionCatalog:
		return map[string]string{"id": "p.id", "application_id": "p.application_id", "application_code": "a.code", "code": "p.code", "description": "p.description"}
	case domain.ApplicationCatalog:
		return map[string]string{"id": "a.id", "code": "a.code", "name": "a.name", "status": "a.status", "is_system": "a.is_system"}
	default:
		return nil
	}
}

func catalogBase(handle domain.QueryReader, c domain.Catalog, q domain.CatalogQuery) *query.QueryBuilder {
	var b *query.QueryBuilder
	switch c {
	case domain.UserApplicationAccessCatalog:
		b = handle.UseModel(domain.ApplicationAccessItem{}).Join("applications a", "a.id = access.application_id").Where("access.user_id = ?", q.TargetUserID)
	case domain.UserRoleAssignmentCatalog:
		b = handle.UseModel(domain.UserRoleAssignmentItem{}).Join("role r", "r.id = ur.role_id").Join("applications a", "a.id = r.application_id").LeftJoin("user_application_access access", "access.user_id = ur.user_id AND access.application_id = a.id").Where("ur.user_id = ?", q.TargetUserID)
	case domain.RoleCatalog:
		b = roleSummaryBase(handle)
	case domain.PermissionCatalog:
		b = handle.UseModel(domain.PermissionItem{}).Join("applications a", "a.id = p.application_id")
	case domain.ApplicationCatalog:
		b = handle.UseModel(domain.ApplicationItem{})
	}
	expressions := catalogExpressions(c)
	if q.ApplicationCode != "" {
		b = b.Where("a.code = ?", q.ApplicationCode)
	}
	if len(q.Statuses) > 0 {
		b = b.WhereIn(expressions["status"], q.Statuses)
	}
	if q.UsesKeywordSearch() {
		return catalogKeywordSearch(b, c, q.Search)
	}
	if q.Search != nil {
		keyword := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.Search.Keyword) + "%"
		if q.Search.Mode == orm.SearchModeContains {
			keyword = "%" + keyword
		}
		terms := make([]string, 0, len(q.Search.Fields))
		args := make([]any, 0, len(q.Search.Fields))
		for _, field := range q.Search.Fields {
			terms = append(terms, expressions[field]+` ILIKE ? ESCAPE '\'`)
			args = append(args, keyword)
		}
		b = b.Where("("+strings.Join(terms, " OR ")+")", args...)
	}
	return b
}

func catalogOrderExpression(c domain.Catalog, field string) string {
	expression := catalogExpressions(c)[field]
	f, _ := c.Field(field)
	if f.Kind == "text" && c != domain.UserRoleAssignmentCatalog {
		return expression + ` COLLATE "C"`
	}
	return expression
}

func catalogArgument(value domain.CatalogValue) any {
	if value.Value == nil {
		return nil
	}
	switch value.Kind {
	case "integer":
		n, _ := strconv.ParseInt(*value.Value, 10, 64)
		return n
	case "bool":
		b, _ := strconv.ParseBool(*value.Value)
		return b
	case "time":
		at, _ := time.Parse(time.RFC3339Nano, *value.Value)
		return at
	default:
		return *value.Value
	}
}

func catalogAfter(b *query.QueryBuilder, c domain.Catalog, q domain.CatalogQuery, values []domain.CatalogValue) *query.QueryBuilder {
	if len(values) == 0 {
		return b
	}
	alternatives := []string{}
	args := []any{}
	for i, sort := range q.Sort {
		if values[i].Value == nil {
			continue
		}
		terms := []string{}
		for j := 0; j < i; j++ {
			expression := catalogOrderExpression(c, q.Sort[j].Field)
			if values[j].Value == nil {
				terms = append(terms, expression+" IS NULL")
			} else {
				terms = append(terms, expression+" = ?")
				args = append(args, catalogArgument(values[j]))
			}
		}
		expression := catalogOrderExpression(c, sort.Field)
		op := ">"
		if sort.Desc {
			op = "<"
		}
		comparison := expression + " " + op + " ?"
		f, _ := c.Field(sort.Field)
		if f.Nullable {
			comparison = "(" + comparison + " OR " + expression + " IS NULL)"
		}
		terms = append(terms, comparison)
		args = append(args, catalogArgument(values[i]))
		alternatives = append(alternatives, "("+strings.Join(terms, " AND ")+")")
	}
	return b.Where("("+strings.Join(alternatives, " OR ")+")", args...)
}

type catalogRow interface {
	CatalogValue(string) domain.CatalogValue
}

func readCatalog[T catalogRow](ctx context.Context, handle domain.QueryReader, c domain.Catalog, q domain.CatalogQuery) (page orm.PageData[T], err error) {
	if err = ctx.Err(); err != nil {
		return page, err
	}
	if handle == nil {
		return page, common.ErrDependencyUnavailable
	}
	q, err = c.Normalize(q)
	if err != nil {
		return page, err
	}
	values, err := c.ParseCursor(q)
	if err != nil {
		return page, err
	}
	if len(values) == 0 && int64(q.Page-1) > math.MaxInt64/int64(q.Limit) {
		return page, common.ErrBadRequest
	}
	total, err := countCatalog(ctx, handle, c, q)
	if err != nil {
		return page, err
	}
	b := catalogPageQuery(ctx, handle, c, q, values)
	rows := make([]T, 0, q.Limit+1)
	if err = b.Limit(q.Limit + 1).Scan(&rows); err != nil {
		return page, helpers.HandleDBError(err)
	}
	return catalogPage(c, q, values, rows, total), nil
}

func countCatalog(ctx context.Context, handle domain.QueryReader, c domain.Catalog, q domain.CatalogQuery) (int64, error) {
	var count struct {
		Total int64 `sql:"column:total"`
	}
	err := catalogBase(handle, c, q).WithContext(ctx).Select("COUNT(DISTINCT " + catalogExpressions(c)[c.IdentityField()] + ") AS total").Scan(&count)
	if err != nil {
		return 0, helpers.HandleDBError(err)
	}
	if count.Total < 0 {
		return 0, common.ErrDependencyUnavailable
	}
	return count.Total, nil
}

func catalogPageQuery(ctx context.Context, handle domain.QueryReader, c domain.Catalog, q domain.CatalogQuery, values []domain.CatalogValue) *query.QueryBuilder {
	b := catalogAfter(catalogBase(handle, c, q), c, q, values).WithContext(ctx)
	projection := make([]string, 0, len(c.Fields()))
	for _, field := range c.Fields() {
		projection = append(projection, catalogExpressions(c)[field.Name]+" AS "+field.Name)
	}
	b = b.Select(projection...)
	for _, sort := range q.Sort {
		dir := "ASC"
		if sort.Desc {
			dir = "DESC"
		}
		b = b.OrderBy(catalogOrderExpression(c, sort.Field) + " " + dir + " NULLS LAST")
	}
	if len(values) == 0 {
		b = b.Offset((q.Page - 1) * q.Limit)
	}
	return b
}

func catalogPage[T catalogRow](c domain.Catalog, q domain.CatalogQuery, values []domain.CatalogValue, rows []T, total int64) orm.PageData[T] {
	var page orm.PageData[T]
	page.HasNext = len(rows) > q.Limit
	if page.HasNext {
		rows = rows[:q.Limit]
	}
	page.Items = rows
	page.Total = total
	page.Page = q.Page
	page.Limit = q.Limit
	page.HasPrev = len(values) > 0 || q.Page > 1
	page.TotalPages = int(total / int64(q.Limit))
	if total%int64(q.Limit) > 0 {
		page.TotalPages++
	}
	if page.HasNext && len(rows) > 0 {
		last := rows[len(rows)-1]
		keys := make([]domain.CatalogValue, 0, len(q.Sort))
		for _, sort := range q.Sort {
			keys = append(keys, last.CatalogValue(sort.Field))
		}
		page.NextCursor = c.NextCursor(q, keys)
	}
	return page
}
