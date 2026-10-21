package test_scenarios

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "raw batch bounds Unicode IDs version key and immutable canonical input", Run: assignmentInputs},
		{Name: "seventeen scalar sort projection search and filter allowlists", Run: assignmentQuery},
		{Name: "typed nullable cursor target method digest and numeric compatibility", Run: assignmentCursor},
		{Name: "canonical fingerprints safe response shapes and independent scopes", Run: assignmentFingerprint},
	}
}

func validInput() domain.UserRoleAssignmentInput {
	return domain.UserRoleAssignmentInput{UserID: 7, RoleIDs: []uint64{3, 1, 3}, ExpectedVersion: 2, Reason: "  Review access  ", Key: " exact key "}
}

func assignmentInputs(t *testing.T) {
	cases := map[string]func(*domain.UserRoleAssignmentInput){
		"zero target": func(r *domain.UserRoleAssignmentInput) { r.UserID = 0 }, "overflow target": func(r *domain.UserRoleAssignmentInput) { r.UserID = math.MaxInt32 + 1 },
		"nil roles": func(r *domain.UserRoleAssignmentInput) { r.RoleIDs = nil }, "empty roles": func(r *domain.UserRoleAssignmentInput) { r.RoleIDs = []uint64{} }, "zero role": func(r *domain.UserRoleAssignmentInput) { r.RoleIDs = []uint64{0} }, "overflow role": func(r *domain.UserRoleAssignmentInput) { r.RoleIDs = []uint64{math.MaxInt32 + 1} },
		"21 repeated roles": func(r *domain.UserRoleAssignmentInput) {
			r.RoleIDs = make([]uint64, 21)
			for i := range r.RoleIDs {
				r.RoleIDs[i] = 1
			}
		},
		"zero version": func(r *domain.UserRoleAssignmentInput) { r.ExpectedVersion = 0 }, "negative version": func(r *domain.UserRoleAssignmentInput) { r.ExpectedVersion = -1 },
		"blank reason": func(r *domain.UserRoleAssignmentInput) { r.Reason = "   " }, "short reason": func(r *domain.UserRoleAssignmentInput) { r.Reason = " éé " }, "long reason": func(r *domain.UserRoleAssignmentInput) { r.Reason = strings.Repeat("é", 501) }, "invalid UTF8": func(r *domain.UserRoleAssignmentInput) { r.Reason = "ok\xff" }, "NUL": func(r *domain.UserRoleAssignmentInput) { r.Reason = "bad\x00reason" },
		"empty key": func(r *domain.UserRoleAssignmentInput) { r.Key = "" }, "blank key": func(r *domain.UserRoleAssignmentInput) { r.Key = " \n " }, "long key": func(r *domain.UserRoleAssignmentInput) { r.Key = strings.Repeat("é", 201) }, "bad key": func(r *domain.UserRoleAssignmentInput) { r.Key = "bad\xff" }, "NUL key": func(r *domain.UserRoleAssignmentInput) { r.Key = "bad\x00key" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := validInput()
			mutate(&r)
			if !errors.Is(r.Validate(), common.ErrBadRequest) {
				t.Fatal("accepted", name)
			}
		})
	}
	in := validInput()
	original := slices.Clone(in.RoleIDs)
	if e := in.Validate(); e != nil {
		t.Fatal(e)
	}
	out := in.Normalized()
	if !reflect.DeepEqual(out.RoleIDs, []uint64{1, 3}) || out.Reason != "Review access" || out.Key != in.Key || !reflect.DeepEqual(in.RoleIDs, original) {
		t.Fatal("canonical normalization", out)
	}
	in.UserID = math.MaxInt32
	in.ExpectedVersion = math.MaxInt64
	in.Reason = strings.Repeat("界", 500)
	in.RoleIDs = make([]uint64, 20)
	for i := range in.RoleIDs {
		in.RoleIDs[i] = math.MaxInt32
	}
	if e := in.Validate(); e != nil {
		t.Fatal("boundary rejected", e)
	}
}

func assignmentQuery(t *testing.T) {
	c := domain.UserRoleAssignmentCatalog
	if len(c.Fields()) != 17 {
		t.Fatal("scalar inventory")
	}
	for _, f := range c.Fields() {
		for _, desc := range []bool{false, true} {
			q, e := c.Normalize(domain.CatalogQuery{TargetUserID: 7, Fields: []string{f.Name}, Sort: []orm.SortField{{Field: f.Name, Desc: desc}}})
			if e != nil || !q.Projects("id") || !q.Projects("role_id") || q.Sort[0].Desc != desc || q.Sort[len(q.Sort)-1].Field != "id" {
				t.Fatal("scalar", f, q, e)
			}
		}
	}
	q, e := c.Normalize(domain.CatalogQuery{TargetUserID: 7, Statuses: []string{"REVOKED", "ACTIVE", "ACTIVE"}, Search: &orm.SearchQuery{Keyword: "  50%_safe  "}})
	if e != nil || q.Page != 1 || q.Limit != 20 || !reflect.DeepEqual(q.Statuses, []string{"ACTIVE", "REVOKED"}) || q.Search.Keyword != "50%_safe" || q.Search.Mode != orm.SearchModeContains || !reflect.DeepEqual(q.Search.Fields, []string{"code", "name"}) {
		t.Fatal(q, e)
	}
	cases := []domain.CatalogQuery{
		{Page: -1}, {Limit: -1}, {Limit: 101}, {Statuses: []string{"DISABLED"}}, {Statuses: []string{"active"}},
		{Fields: []string{"password"}}, {Fields: []string{"id", "id"}}, {Sort: []orm.SortField{{Field: "id DESC; DROP TABLE auth"}}}, {Sort: []orm.SortField{{Field: "id"}, {Field: "id"}}},
		{Search: &orm.SearchQuery{Fields: []string{"status"}}}, {Search: &orm.SearchQuery{Fields: []string{"name", "name"}}}, {Search: &orm.SearchQuery{Mode: "FULLTEXT"}}, {Search: &orm.SearchQuery{Keyword: strings.Repeat("界", 201)}}, {ApplicationCode: "sales' OR TRUE"},
	}
	for i, in := range cases {
		in.TargetUserID = 7
		if _, e := c.Normalize(in); !errors.Is(e, common.ErrBadRequest) {
			t.Fatalf("invalid query %d accepted", i)
		}
	}
	if _, e := c.Normalize(domain.CatalogQuery{}); e == nil {
		t.Fatal("absent target")
	}
	for _, mode := range []orm.SearchMode{orm.SearchModeContains, orm.SearchModePrefix} {
		if _, e := c.Normalize(domain.CatalogQuery{TargetUserID: 7, Limit: 100, ApplicationCode: "unknown", Search: &orm.SearchQuery{Fields: []string{"application_code"}, Mode: mode, Keyword: "literal%_"}}); e != nil {
			t.Fatal(e)
		}
	}
}

func assignmentCursor(t *testing.T) {
	c := domain.UserRoleAssignmentCatalog
	q, e := c.Normalize(domain.CatalogQuery{TargetUserID: 7, Sort: []orm.SortField{{Field: "revoked_at", Desc: true}, {Field: "name"}}})
	if e != nil {
		t.Fatal(e)
	}
	text, id := "Tied name", "12"
	keys := []domain.CatalogValue{{Kind: "time"}, {Kind: "text", Value: &text}, {Kind: "integer", Value: &id}}
	q.LastID = c.NextCursor(q, keys)
	got, e := c.ParseCursor(q)
	if e != nil || !reflect.DeepEqual(got, keys) {
		t.Fatal(got, e)
	}
	for _, mutate := range []func(*domain.CatalogQuery){func(q *domain.CatalogQuery) { q.TargetUserID++ }, func(q *domain.CatalogQuery) { q.Statuses = []string{"ACTIVE"} }, func(q *domain.CatalogQuery) { q.ApplicationCode = "sales" }, func(q *domain.CatalogQuery) { q.Sort = slices.Clone(q.Sort); q.Sort[0].Desc = false }, func(q *domain.CatalogQuery) { q.Search = &orm.SearchQuery{Keyword: "new"} }} {
		other := q
		mutate(&other)
		if _, e := c.ParseCursor(other); e == nil {
			t.Fatal("cross query accepted")
		}
	}
	if _, e := domain.RoleCatalog.ParseCursor(q); e == nil {
		t.Fatal("cross method accepted")
	}
	other := q
	other.Fields = []string{"id", "role_id"}
	other.Limit = 99
	other.Page = 5
	if _, e := c.ParseCursor(other); e != nil {
		t.Fatal("projection/page/limit digest", e)
	}
	numeric, _ := c.Normalize(domain.CatalogQuery{TargetUserID: 7, LastID: "12"})
	if values, e := c.ParseCursor(numeric); e != nil || len(values) != 1 {
		t.Fatal(e)
	}
	for _, last := range []string{"0", "-1", "01", "2147483648", "not a cursor", q.LastID + "="} {
		bad := numeric
		bad.LastID = last
		if _, e := c.ParseCursor(bad); e == nil {
			t.Fatal("malformed accepted", last)
		}
	}
	bad := q
	bad.LastID = "12"
	if _, e := c.ParseCursor(bad); e == nil {
		t.Fatal("numeric mixed sort")
	}
	bytes, _ := base64.RawURLEncoding.DecodeString(q.LastID)
	var cursor domain.CatalogCursor
	json.Unmarshal(bytes, &cursor)
	for _, mutate := range []func(*domain.CatalogCursor){func(c *domain.CatalogCursor) { c.Version = 2 }, func(c *domain.CatalogCursor) { c.Values[0].Kind = "text" }, func(c *domain.CatalogCursor) { c.Values[2].Value = nil }, func(c *domain.CatalogCursor) { v := "0"; c.Values[2].Value = &v }, func(c *domain.CatalogCursor) { v := "invalid time"; c.Values[0].Value = &v }} {
		var x domain.CatalogCursor
		json.Unmarshal(bytes, &x)
		mutate(&x)
		b, _ := json.Marshal(x)
		bad := q
		bad.LastID = base64.RawURLEncoding.EncodeToString(b)
		if _, e := c.ParseCursor(bad); e == nil {
			t.Fatal("invalid typed cursor accepted")
		}
	}
	row := domain.UserRoleAssignmentItem{ID: 1, RoleID: 2, Code: "iam.member", Name: "Member"}
	p := row.Project(domain.CatalogQuery{Fields: []string{"name"}})
	if p.ID != 1 || p.RoleID != 2 || p.Name != "Member" || p.Code != "" || p.GrantedAt != nil {
		t.Fatal("projection", p)
	}
}

func assignmentFingerprint(t *testing.T) {
	scope := domain.IdempotencyScope{ActorID: 42, Method: "/user.v1.UserService/GrantUserRoles", Target: "user/7", Key: "same"}
	in := validInput().Normalized()
	payload := featurecommon.FingerprintPayload{RoleIDs: in.RoleIDs, Reason: in.Reason, ExpectedVersion: in.ExpectedVersion, Presence: []string{"expected_user_version"}}
	a, e := featurecommon.Fingerprint(scope, payload)
	if e != nil {
		t.Fatal(e)
	}
	payload.RoleIDs = []uint64{3, 1, 3}
	b, _ := featurecommon.Fingerprint(scope, payload)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("set fingerprint")
	}
	payload.Reason = "Different reason"
	b, _ = featurecommon.Fingerprint(scope, payload)
	if reflect.DeepEqual(a, b) {
		t.Fatal("reason omitted")
	}
	r := domain.SafeCommandResult{UserID: 7, Version: 3, Changed: true, ChangedRoleIDs: []uint64{1, 3}}
	if r.ValidateAssignmentResult(7) != nil {
		t.Fatal("valid result")
	}
	for _, ids := range [][]uint64{{3, 1}, {1, 1}, {0}, {math.MaxInt32 + 1}, nil} {
		bad := r
		bad.ChangedRoleIDs = ids
		if bad.ValidateAssignmentResult(7) == nil {
			t.Fatal("corrupt IDs", ids)
		}
	}
	r.Changed = false
	r.ChangedRoleIDs = nil
	if r.ValidateAssignmentResult(7) != nil || r.ValidateAssignmentResult(8) == nil {
		t.Fatal("no-op or target validation")
	}
}
