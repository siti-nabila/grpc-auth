package test_scenarios

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
)

func KeywordSQL() []testutils.Scenario {
	return []testutils.Scenario{{Name: "keyword count and page share projection joins bound literals filters and context", Run: keywordSQL}}
}

func keywordSQL(t *testing.T) {
	for _, c := range []domain.Catalog{domain.RoleCatalog, domain.PermissionCatalog, domain.ApplicationCatalog, domain.UserRoleAssignmentCatalog} {
		for _, mode := range []orm.SearchMode{orm.SearchModeFullText, orm.SearchModeFullTextTrigram} {
			t.Run(string(c)+"/"+string(mode), func(t *testing.T) {
				q := domain.CatalogQuery{TargetUserID: 123, Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: `%_\' & senior.-reviewer`, Mode: mode}}
				var alias, join string
				var args []driver.Value
				if c == domain.UserRoleAssignmentCatalog {
					args = append(args, int64(123))
				}
				if c != domain.ApplicationCatalog {
					q.ApplicationCode = "clm"
					args = append(args, "clm")
				}
				switch c {
				case domain.RoleCatalog:
					alias, join = "rs", "JOIN role_search rs ON rs.role_id = r.id"
				case domain.PermissionCatalog:
					alias, join = "ps", "JOIN permission_search ps ON ps.permission_id = p.id"
				case domain.ApplicationCatalog:
					alias, join = "aps", "JOIN application_search aps ON aps.application_id = a.id"
				case domain.UserRoleAssignmentCatalog:
					alias, join = "urs", "JOIN user_role_search urs ON urs.user_role_id = ur.id"
				}
				n := len(args) + 1
				args = append(args, q.Search.Keyword)
				predicate := fmt.Sprintf("%s.fts_keyword @@ catalog_search_query($%d)", alias, n)
				if mode == orm.SearchModeFullTextTrigram {
					predicate = fmt.Sprintf("(%s.fts_keyword @@ catalog_search_prefix_query($%d) OR %s.fts_lexeme_text ILIKE catalog_search_pattern($%d) ESCAPE '\\')", alias, n, alias, n+1)
					args = append(args, q.Search.Keyword)
				}
				runRoleSQL(t, []mocks.SQLStep{
					{Op: "query", SQL: join, Args: args, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(0)}}},
					{Op: "query", SQL: predicate, Args: args, Columns: []string{"id"}},
				}, func(ctx context.Context, tx domain.Transaction) {
					var err error
					switch c {
					case domain.RoleCatalog:
						_, err = reader.NewCatalogReader(tx).Roles(ctx, q)
					case domain.PermissionCatalog:
						_, err = reader.NewCatalogReader(tx).Permissions(ctx, q)
					case domain.ApplicationCatalog:
						_, err = reader.NewCatalogReader(tx).Applications(ctx, q)
					case domain.UserRoleAssignmentCatalog:
						_, err = reader.NewUserRoleAssignmentReader(tx).ListAssignments(ctx, q)
					}
					if err != nil {
						t.Fatal(err)
					}
				})
			})
		}
	}
}
