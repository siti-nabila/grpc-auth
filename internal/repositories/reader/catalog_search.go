package reader

import (
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/query"
)

// Only projection routing belongs here. Database functions own document sources,
// separators and plain-text parsing; request values are always bound arguments.
func catalogKeywordSearch(b *query.QueryBuilder, c domain.Catalog, s *orm.SearchQuery) *query.QueryBuilder {
	var table, join, alias string
	switch c {
	case domain.RoleCatalog:
		table, join, alias = "role_search rs", "rs.role_id = r.id", "rs"
	case domain.PermissionCatalog:
		table, join, alias = "permission_search ps", "ps.permission_id = p.id", "ps"
	case domain.ApplicationCatalog:
		table, join, alias = "application_search aps", "aps.application_id = a.id", "aps"
	case domain.UserRoleAssignmentCatalog:
		table, join, alias = "user_role_search urs", "urs.user_role_id = ur.id", "urs"
	}
	b = b.Join(table, join)
	if s.Mode == orm.SearchModeFullText {
		return b.Where(alias+".fts_keyword @@ catalog_search_query(?)", s.Keyword)
	}
	return b.Where("("+alias+".fts_keyword @@ catalog_search_prefix_query(?) OR "+
		alias+`.fts_lexeme_text ILIKE catalog_search_pattern(?) ESCAPE '\')`, s.Keyword, s.Keyword)
}
