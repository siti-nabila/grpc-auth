package test_scenarios

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/repositories/domain/role/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func Catalogs() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "pre-refactor serialized cursors remain readable", Run: serializedCursors},
		{Name: "catalog normalization leaves caller slices and search unchanged", Run: catalogInputIsolation},
		{Name: "every public scalar supports both sort directions and projection", Run: catalogRegistry},
		{Name: "query defaults canonical filters literal search and invalid fields", Run: catalogValidation},
		{Name: "typed cursor binds method query and sort while excluding page limit projection", Run: catalogCursors},
		{Name: "cursor rejects malformed unsafe type null and query mismatches", Run: catalogCursorErrors},
	}
}

func catalogRegistry(t *testing.T) {
	for _, x := range []struct {
		c domain.Catalog
		d protoreflect.MessageDescriptor
	}{
		{domain.RoleCatalog, (&rolev1.RoleSummary{}).ProtoReflect().Descriptor()},
		{domain.PermissionCatalog, (&permissionv1.PermissionItem{}).ProtoReflect().Descriptor()},
		{domain.ApplicationCatalog, (&applicationv1.ApplicationItem{}).ProtoReflect().Descriptor()},
	} {
		if len(x.c.Fields()) != x.d.Fields().Len() {
			t.Fatal("registry does not cover public descriptor")
		}
		for _, field := range x.c.Fields() {
			if x.d.Fields().ByName(protoreflect.Name(field.Name)) == nil {
				t.Fatal("internal field exposed", field.Name)
			}
			for _, desc := range []bool{false, true} {
				q, err := x.c.Normalize(domain.CatalogQuery{Fields: []string{field.Name}, Sort: []orm.SortField{{Field: field.Name, Desc: desc}}})
				if err != nil || q.Sort[0].Desc != desc || q.Sort[0].Field != field.Name || !q.Projects("id") || !q.Projects(field.Name) {
					t.Fatal("scalar not supported", field, desc, err)
				}
				if field.Name != "id" && (len(q.Sort) != 2 || q.Sort[1].Field != "id" || q.Sort[1].Desc) {
					t.Fatal("id tie breaker")
				}
			}
		}
	}
}

func catalogValidation(t *testing.T) {
	q, err := domain.RoleCatalog.Normalize(domain.CatalogQuery{Statuses: []string{"DISABLED", "ACTIVE", "ACTIVE"}, Search: &orm.SearchQuery{Keyword: "  50%_\\  "}})
	if err != nil || q.Page != 1 || q.Limit != 20 || q.Sort[0].Field != "id" || q.Search.Keyword != "50%_\\" || q.Search.Mode != orm.SearchModeContains || !reflect.DeepEqual(q.Statuses, []string{"ACTIVE", "DISABLED"}) {
		t.Fatalf("defaults %+v %v", q, err)
	}
	for _, c := range []domain.Catalog{domain.RoleCatalog, domain.PermissionCatalog, domain.ApplicationCatalog} {
		for _, bad := range []domain.CatalogQuery{
			{Page: -1}, {Limit: -1}, {Limit: 101}, {Sort: []orm.SortField{{Field: "id"}, {Field: "id"}}}, {Sort: []orm.SortField{{Field: "r.role_name"}}}, {Fields: []string{"password"}}, {Fields: []string{"id", "id"}}, {Fields: []string{"id; DROP TABLE role"}}, {Statuses: []string{"UNKNOWN"}}, {Search: &orm.SearchQuery{Mode: orm.SearchModeFullText}}, {Search: &orm.SearchQuery{Fields: []string{"id"}, Keyword: "x"}}, {Search: &orm.SearchQuery{Fields: []string{"code", "code"}, Keyword: "x"}}, {Search: &orm.SearchQuery{Keyword: strings.Repeat("é", 201)}},
		} {
			if _, err := c.Normalize(bad); !errors.Is(err, common.ErrBadRequest) {
				t.Fatalf("accepted invalid %s %+v %v", c, bad, err)
			}
		}
	}
	if _, err := domain.PermissionCatalog.Normalize(domain.CatalogQuery{Statuses: []string{"ACTIVE"}}); err == nil {
		t.Fatal("permission has status")
	}
	if _, err := domain.ApplicationCatalog.Normalize(domain.CatalogQuery{ApplicationCode: "sales"}); err == nil {
		t.Fatal("app filter accepted")
	}
	if _, err := domain.RoleCatalog.Normalize(domain.CatalogQuery{ApplicationCode: "Sales"}); err == nil {
		t.Fatal("noncanonical app accepted")
	}
}

func catalogRole() domain.RoleSummary {
	description := ""
	at := time.Date(2026, 10, 12, 0, 0, 0, 123, time.UTC)
	return domain.RoleSummary{ID: 7, Code: "sales.editor", Name: "Editor", Description: &description, ApplicationID: 2, ApplicationCode: "sales", Status: "ACTIVE", Version: 3, NumericCode: 81, CreatedAt: &at}
}

func catalogCursors(t *testing.T) {
	row := catalogRole()
	for _, c := range []domain.Catalog{domain.RoleCatalog, domain.PermissionCatalog, domain.ApplicationCatalog} {
		for _, field := range c.Fields() {
			for _, desc := range []bool{false, true} {
				q, _ := c.Normalize(domain.CatalogQuery{Sort: []orm.SortField{{Field: field.Name, Desc: desc}}})
				values := []domain.CatalogValue{}
				for _, s := range q.Sort {
					switch c {
					case domain.RoleCatalog:
						values = append(values, row.CatalogValue(s.Field))
					case domain.PermissionCatalog:
						values = append(values, (domain.PermissionItem{ID: 9, ApplicationID: 2, ApplicationCode: "sales", Code: "sales.order.read", Description: "Read"}).CatalogValue(s.Field))
					case domain.ApplicationCatalog:
						values = append(values, (domain.ApplicationItem{ID: 2, Code: "sales", Name: "Sales", Status: "ACTIVE"}).CatalogValue(s.Field))
					}
				}
				q.LastID = c.NextCursor(q, values)
				q.Page = 8
				q.Limit = 1
				q.Fields = []string{"id"}
				got, err := c.ParseCursor(q)
				if err != nil || !reflect.DeepEqual(values, got) {
					t.Fatalf("cursor %s %s %v", c, field.Name, err)
				}
			}
		}
	}
	q, _ := domain.RoleCatalog.Normalize(domain.CatalogQuery{LastID: "7"})
	if values, err := domain.RoleCatalog.ParseCursor(q); err != nil || *values[0].Value != "7" {
		t.Fatal("numeric compatibility", err)
	}
}

func catalogCursorErrors(t *testing.T) {
	q, _ := domain.RoleCatalog.Normalize(domain.CatalogQuery{Sort: []orm.SortField{{Field: "description", Desc: true}, {Field: "is_system"}}})
	row := catalogRole()
	values := []domain.CatalogValue{row.CatalogValue("description"), row.CatalogValue("is_system"), row.CatalogValue("id")}
	q.LastID = domain.RoleCatalog.NextCursor(q, values)
	for _, mutate := range []func(*domain.CatalogQuery){func(q *domain.CatalogQuery) { q.ApplicationCode = "other" }, func(q *domain.CatalogQuery) { q.Sort[0].Desc = false }, func(q *domain.CatalogQuery) { q.Statuses = []string{"ACTIVE"} }, func(q *domain.CatalogQuery) {
		q.Search = &orm.SearchQuery{Keyword: "new", Mode: orm.SearchModeContains, Fields: []string{"name"}}
	}, func(q *domain.CatalogQuery) { q.LastID = "7" }, func(q *domain.CatalogQuery) { q.LastID = "bad@cursor" }} {
		changed := q
		changed.Sort = append([]orm.SortField(nil), q.Sort...)
		mutate(&changed)
		if _, err := domain.RoleCatalog.ParseCursor(changed); err == nil {
			t.Fatal("accepted mismatch")
		}
	}
	if _, err := domain.PermissionCatalog.ParseCursor(q); err == nil {
		t.Fatal("cross catalog cursor")
	}
	b, _ := base64.RawURLEncoding.DecodeString(q.LastID)
	var cursor domain.CatalogCursor
	if json.Unmarshal(b, &cursor) != nil {
		t.Fatal("fixture cursor")
	}
	for _, change := range []func(*domain.CatalogCursor){
		func(c *domain.CatalogCursor) { c.Version = 2 }, func(c *domain.CatalogCursor) { c.Values[1].Kind = "text" }, func(c *domain.CatalogCursor) { c.Values[1].Value = nil }, func(c *domain.CatalogCursor) { s := "FALSE"; c.Values[1].Value = &s }, func(c *domain.CatalogCursor) { s := "0"; c.Values[2].Value = &s }, func(c *domain.CatalogCursor) { s := "2147483648"; c.Values[2].Value = &s }, func(c *domain.CatalogCursor) { s := "x\x00"; c.Values[0].Value = &s },
	} {
		copy := cursor
		copy.Values = append([]domain.CatalogValue(nil), cursor.Values...)
		change(&copy)
		encoded, _ := json.Marshal(copy)
		changed := q
		changed.LastID = base64.RawURLEncoding.EncodeToString(encoded)
		if _, err := domain.RoleCatalog.ParseCursor(changed); err == nil {
			t.Fatal("unsafe typed cursor")
		}
	}
	for _, bad := range [][]byte{append(b, []byte(" {}")...), []byte(strings.Replace(string(b), `"v":1`, `"unsafe":"password","v":1`, 1))} {
		changed := q
		changed.LastID = base64.RawURLEncoding.EncodeToString(bad)
		if _, err := domain.RoleCatalog.ParseCursor(changed); err == nil {
			t.Fatal("unknown or trailing cursor")
		}
	}
}

func serializedCursors(t *testing.T) {
	for _, c := range []domain.Catalog{domain.RoleCatalog, domain.PermissionCatalog, domain.ApplicationCatalog, domain.UserRoleAssignmentCatalog} {
		field := "name"
		target := uint64(7)
		if c == domain.PermissionCatalog {
			field = "description"
			target = 0
		}
		q, err := c.Normalize(domain.CatalogQuery{TargetUserID: target, Sort: []orm.SortField{{Field: field, Desc: true}}, LastID: fixtures.SerializedCursors[string(c)]})
		if err != nil {
			t.Fatal(c, err)
		}
		values, err := c.ParseCursor(q)
		if err != nil || len(values) != 2 || values[0].Value == nil || *values[0].Value != "Tied name" || values[1].Value == nil || *values[1].Value != "12" {
			t.Fatal(c, values, err)
		}
		if c.NextCursor(q, values) != q.LastID {
			t.Fatal("cursor encoding changed", c)
		}
	}
	sorts, _ := domain.NormalizeUserListSort([]orm.SortField{{Field: "name", Desc: true}})
	filter, _ := domain.NormalizeUserListFilter(domain.UserListFilter{})
	opts := orm.QueryOptions{Sort: sorts}
	value, err := domain.ParseUserListCursor(fixtures.SerializedUserCursor, opts, filter)
	if err != nil {
		t.Fatal("old user cursor", err)
	}
	cursor, ok := value.(domain.UserListCursor)
	if !ok || len(cursor.Values) != 2 || cursor.Values[0].Value == nil || *cursor.Values[0].Value != "Tied name" || cursor.Values[1].Value == nil || *cursor.Values[1].Value != "12" {
		t.Fatal("old user cursor values", value)
	}
}

func catalogInputIsolation(t *testing.T) {
	in := domain.CatalogQuery{Fields: []string{"name"}, Sort: []orm.SortField{{Field: "name", Desc: true}}, Statuses: []string{"DISABLED", "ACTIVE", "ACTIVE"}, Search: &orm.SearchQuery{Fields: []string{"name", "code"}, Keyword: "  Editor  "}}
	original := domain.CatalogQuery{Fields: []string{"name"}, Sort: []orm.SortField{{Field: "name", Desc: true}}, Statuses: []string{"DISABLED", "ACTIVE", "ACTIVE"}, Search: &orm.SearchQuery{Fields: []string{"name", "code"}, Keyword: "  Editor  "}}
	out, err := domain.RoleCatalog.Normalize(in)
	if err != nil {
		t.Fatal(err)
	}
	out.Fields[0] = "code"
	out.Sort[0].Desc = false
	out.Statuses[0] = "DISABLED"
	out.Search.Fields[0] = "description"
	out.Search.Keyword = "Changed"
	if !reflect.DeepEqual(in, original) {
		t.Fatal("caller input mutated", in)
	}
	in.Sort = []orm.SortField{{Field: "invalid"}}
	if _, err := domain.RoleCatalog.Normalize(in); err == nil || !reflect.DeepEqual(in.Fields, original.Fields) || !reflect.DeepEqual(in.Statuses, original.Statuses) || !reflect.DeepEqual(in.Search, original.Search) {
		t.Fatal("failed normalization mutated input", in, err)
	}
}
