package test_scenarios

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
)

func KeywordCatalogs() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "keyword validation is additive isolated and raw Unicode bounded", Run: keywordValidation},
		{Name: "keyword cursors bind scope and revision while preserving legacy cursors", Run: keywordCursors},
	}
}

func keywordValidation(t *testing.T) {
	for _, c := range []domain.Catalog{domain.RoleCatalog, domain.PermissionCatalog, domain.ApplicationCatalog, domain.UserRoleAssignmentCatalog} {
		for _, mode := range []orm.SearchMode{orm.SearchModeFullText, orm.SearchModeFullTextTrigram} {
			for _, word := range []string{"", " \t\n ", "  senior.-reviewer  ", "&!('%_\\)", strings.Repeat("é", 200)} {
				in := domain.CatalogQuery{TargetUserID: 123, Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: word, Mode: mode}}
				q, err := c.Normalize(in)
				if err != nil || in.Search.Keyword != word || in.Search.Fields[0] != "keyword" {
					t.Fatalf("%s normalize or input mutation: %v", c, err)
				}
				want := strings.Join(strings.Fields(word), " ")
				if want == "" {
					if q.Search != nil {
						t.Fatal("empty keyword must not filter")
					}
				} else if !q.UsesKeywordSearch() || q.Search.Keyword != want {
					t.Fatal("keyword lost literal input", q)
				}
			}
		}
		for _, search := range []*orm.SearchQuery{
			{Fields: []string{"keyword"}},
			{Fields: []string{"keyword"}, Mode: orm.SearchModeContains},
			{Fields: []string{"keyword"}, Mode: orm.SearchModePrefix},
			{Fields: []string{"keyword"}, Mode: orm.SearchModeTrigram},
			{Fields: []string{"keyword", "name"}, Mode: orm.SearchModeFullText},
			{Fields: []string{"keyword", "keyword"}, Mode: orm.SearchModeFullText},
			{Mode: orm.SearchModeFullText},
			{Fields: []string{"name"}, Mode: orm.SearchModeFullText},
			{Fields: []string{"keyword"}, Mode: orm.SearchModeFullText, Keyword: strings.Repeat("é", 201)},
			{Fields: []string{"keyword"}, Mode: orm.SearchModeFullText, Keyword: strings.Repeat(" ", 201)},
			{Fields: []string{"keyword"}, Mode: orm.SearchModeFullText, Keyword: "\xff"},
			{Fields: []string{"keyword"}, Mode: orm.SearchModeFullText, Keyword: "a\x00b"},
		} {
			if _, err := c.Normalize(domain.CatalogQuery{TargetUserID: 123, Search: search}); !errors.Is(err, common.ErrBadRequest) {
				t.Fatalf("accepted %+v: %v", search, err)
			}
		}
		for _, q := range []domain.CatalogQuery{{Fields: []string{"keyword"}}, {Sort: []orm.SortField{{Field: "keyword"}}}} {
			q.TargetUserID = 123
			if _, err := c.Normalize(q); !errors.Is(err, common.ErrBadRequest) {
				t.Fatal("keyword exposed as scalar", err)
			}
		}
	}
	if _, err := domain.UserApplicationAccessCatalog.Normalize(domain.CatalogQuery{TargetUserID: 123, Search: &orm.SearchQuery{Fields: []string{"keyword"}, Mode: orm.SearchModeFullText}}); !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("application access changed", err)
	}
}

func keywordCursors(t *testing.T) {
	q, err := domain.UserRoleAssignmentCatalog.Normalize(domain.CatalogQuery{TargetUserID: 123, Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: "reviewer", Mode: orm.SearchModeFullTextTrigram}})
	if err != nil {
		t.Fatal(err)
	}
	id := "7"
	q.LastID = domain.UserRoleAssignmentCatalog.NextCursor(q, []domain.CatalogValue{{Kind: "integer", Value: &id}})
	if _, err = domain.UserRoleAssignmentCatalog.Normalize(q); err != nil {
		t.Fatal("valid cursor", err)
	}
	for _, change := range []func(*domain.CatalogQuery){
		func(q *domain.CatalogQuery) { q.TargetUserID = 456 },
		func(q *domain.CatalogQuery) { q.ApplicationCode = "clm" },
		func(q *domain.CatalogQuery) { q.Statuses = []string{"ACTIVE"} },
		func(q *domain.CatalogQuery) { q.Search.Keyword = "other" },
		func(q *domain.CatalogQuery) { q.Search.Mode = orm.SearchModeFullText },
		func(q *domain.CatalogQuery) { q.Search.Fields = []string{"name"} },
		func(q *domain.CatalogQuery) { q.Sort = []orm.SortField{{Field: "id", Desc: true}} },
		func(q *domain.CatalogQuery) { q.LastID = "7" },
	} {
		copy := q
		s := *q.Search
		copy.Search = &s
		change(&copy)
		if _, err = domain.UserRoleAssignmentCatalog.Normalize(copy); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("accepted changed scope", err)
		}
	}
	b, _ := base64.RawURLEncoding.DecodeString(q.LastID)
	var cursor domain.CatalogCursor
	if json.Unmarshal(b, &cursor) != nil || cursor.Version != 2 {
		t.Fatal("keyword envelope version")
	}
	for _, mutate := range []func(*domain.CatalogCursor){func(c *domain.CatalogCursor) { c.Version = 1 }, func(c *domain.CatalogCursor) { c.Query = "old-search-revision" }, func(c *domain.CatalogCursor) { c.Method = domain.RoleCatalog }} {
		bad := cursor
		mutate(&bad)
		b, _ = json.Marshal(bad)
		copy := q
		copy.LastID = base64.RawURLEncoding.EncodeToString(b)
		if _, err = domain.UserRoleAssignmentCatalog.Normalize(copy); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("accepted envelope/revision mismatch", err)
		}
	}
	q.Page, q.Limit, q.Fields = 9, 1, []string{"name"}
	if _, err = domain.UserRoleAssignmentCatalog.Normalize(q); err != nil {
		t.Fatal("presentation should not change scope", err)
	}
}
