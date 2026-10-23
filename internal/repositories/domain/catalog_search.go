package domain

import (
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/orm/orm"
)

const CatalogKeywordField = "keyword"

const catalogKeywordRevision = "catalog-keyword-v1"

// UsesKeywordSearch identifies the separate database-document search contract.
func (q CatalogQuery) UsesKeywordSearch() bool {
	return q.Search != nil && len(q.Search.Fields) == 1 &&
		q.Search.Fields[0] == CatalogKeywordField &&
		(q.Search.Mode == orm.SearchModeFullText || q.Search.Mode == orm.SearchModeFullTextTrigram)
}

func (c Catalog) normalizeKeywordSearch(s *orm.SearchQuery) error {
	switch c {
	case RoleCatalog, PermissionCatalog, ApplicationCatalog, UserRoleAssignmentCatalog:
	default:
		return common.ErrBadRequest
	}
	if len(s.Fields) != 1 ||
		(s.Mode != orm.SearchModeFullText && s.Mode != orm.SearchModeFullTextTrigram) ||
		!userText(s.Keyword, 0, 200) {
		return common.ErrBadRequest
	}
	// Whitespace canonicalization is only for request/cursor identity. Catalog
	// separators, lexemes and literal query parsing remain database-owned.
	s.Keyword = strings.Join(strings.Fields(s.Keyword), " ")
	return nil
}

func (q CatalogQuery) cursorVersion() int {
	if q.UsesKeywordSearch() {
		return 2
	}
	return 1
}

func (q CatalogQuery) searchRevision() string {
	if q.UsesKeywordSearch() {
		return catalogKeywordRevision
	}
	return ""
}
