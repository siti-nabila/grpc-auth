package test_scenarios

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"reflect"
	"strings"
	"testing"
	"time"
)

func UserListQuery() []testutils.Scenario {
	cases := []testutils.Scenario{}
	fields := []string{"id", "name", "email", "address", "phone", "status", "version", "created_at", "roles", "application_access"}
	for _, field := range fields {
		for _, desc := range []bool{false, true} {
			cases = append(cases, testutils.Scenario{Name: field + map[bool]string{true: " DESC", false: " ASC"}[desc], Run: func(t *testing.T) {
				opts := orm.QueryOptions{Sort: []orm.SortField{{Field: field, Desc: desc}}}
				var e error
				opts.Sort, e = domain.NormalizeUserListSort(opts.Sort)
				if e != nil {
					t.Fatal(e)
				}
				if field != "id" && (len(opts.Sort) != 2 || opts.Sort[1].Field != "id" || opts.Sort[1].Desc) {
					t.Fatal("missing ID ASC tie")
				}
				f, _ := domain.NormalizeUserListFilter(domain.UserListFilter{RoleCodes: []uint64{2, 1, 2}, Statuses: []types.UserStatus{types.UserActive, types.UserActive}})
				row := domain.UserSearchRow{AuthID: 7, Email: "a@example.test", Status: types.UserActive, Version: 2, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NameKey: sql.NullString{String: "Alice", Valid: true}, AddressKey: sql.NullString{String: "", Valid: true}, PhoneKey: sql.NullString{String: "123", Valid: true}, RolesKey: sql.NullString{String: "Auditor", Valid: true}}
				raw := domain.UserListNextCursor(row, opts, f)
				cursor, e := domain.ParseUserListCursor(raw, opts, f)
				if e != nil {
					t.Fatal(e)
				}
				if field == "id" {
					if cursor != int64(7) {
						t.Fatal(cursor)
					}
				} else {
					c := cursor.(domain.UserListCursor)
					if !reflect.DeepEqual(c.Values[0], row.SortValue(field)) {
						t.Fatal("value/null roundtrip", c)
					}
				}
				if !reflect.DeepEqual(f.RoleCodes, []uint64{1, 2}) || len(f.Statuses) != 1 {
					t.Fatal("canonical filter", f)
				}
			}})
		}
	}
	for _, kind := range []string{"unknown", "internal", "expression", "duplicate", "alias duplicate", "empty", "whitespace"} {
		cases = append(cases, testutils.Scenario{Name: "reject sort " + kind, Run: func(t *testing.T) {
			sorts := []orm.SortField{{Field: map[string]string{"unknown": "age", "internal": "a.id", "expression": "id DESC; DROP TABLE auth", "empty": "", "whitespace": " id"}[kind]}}
			if kind == "duplicate" {
				sorts = []orm.SortField{{Field: "name"}, {Field: "name", Desc: true}}
			}
			if kind == "alias duplicate" {
				sorts = []orm.SortField{{Field: "id"}, {Field: "auth_id"}}
			}
			if _, e := domain.NormalizeUserListSort(sorts); !errors.Is(e, common.ErrBadRequest) {
				t.Fatal("unsafe sort allowed", e)
			}
		}})
	}
	for _, kind := range []string{"malformed", "oversize", "negative", "numeric dynamic", "sort mismatch", "filter mismatch", "search mismatch", "type", "null ID", "invalid integer", "invalid time", "duplicate json", "unknown json", "trailing json", "missing ID", "version"} {
		cases = append(cases, testutils.Scenario{Name: "reject cursor " + kind, Run: func(t *testing.T) {
			opts := orm.QueryOptions{Sort: []orm.SortField{{Field: "created_at"}, {Field: "id"}}}
			f, _ := domain.NormalizeUserListFilter(domain.UserListFilter{})
			row := domain.UserSearchRow{AuthID: 7, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
			raw := domain.UserListNextCursor(row, opts, f)
			b, _ := base64.RawURLEncoding.DecodeString(raw)
			var c domain.UserListCursor
			_ = json.Unmarshal(b, &c)
			switch kind {
			case "malformed":
				raw = "@bad"
			case "oversize":
				raw = strings.Repeat("a", 16385)
			case "negative":
				raw = "-1"
			case "numeric dynamic":
				raw = "7"
			case "sort mismatch":
				opts.Sort[0].Desc = true
			case "filter mismatch":
				f.ApplicationCode = "clm"
			case "search mismatch":
				opts.Search = &orm.SearchQuery{Keyword: "Alice", Fields: []string{"keyword"}, Mode: orm.SearchModeFullText}
			default:
				switch kind {
				case "type":
					c.Values[0].Kind = "text"
				case "null ID":
					c.Values[1].Value = nil
				case "invalid integer":
					v := "7.1"
					c.Values[1].Value = &v
				case "invalid time":
					v := "yesterday"
					c.Values[0].Value = &v
				case "missing ID":
					c.Values = c.Values[:1]
				case "version":
					c.Version = 2
				}
				b, _ = json.Marshal(c)
				if kind == "duplicate json" {
					b = append([]byte(`{"v":1,`), b[1:]...)
				}
				if kind == "unknown json" {
					b = append([]byte(`{"sql":"a.id",`), b[1:]...)
				}
				if kind == "trailing json" {
					b = append(b, []byte(`{}`)...)
				}
				raw = base64.RawURLEncoding.EncodeToString(b)
			}
			if _, e := domain.ParseUserListCursor(raw, opts, f); !errors.Is(e, common.ErrBadRequest) {
				t.Fatal("invalid cursor allowed", kind, e)
			}
		}})
	}
	cases = append(cases, testutils.Scenario{Name: "multi-sort preserves directions and explicit ID and alias", Run: func(t *testing.T) {
		in := []orm.SortField{{Field: "name", Desc: true}, {Field: "auth_id", Desc: true}, {Field: "email"}}
		out, e := domain.NormalizeUserListSort(in)
		if e != nil || len(out) != 3 || out[1].Field != "id" || !out[0].Desc || !out[1].Desc || out[2].Desc {
			t.Fatal(out, e)
		}
		defaults, e := domain.NormalizeUserListSort(nil)
		if e != nil || len(defaults) != 1 || defaults[0].Field != "id" || defaults[0].Desc {
			t.Fatal(defaults, e)
		}
	}})
	return cases
}
