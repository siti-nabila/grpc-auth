package test_scenarios

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "grant and revoke validate raw inputs before immutable normalization", Run: inputs},
		{Name: "safe results distinguish access-only role-only absent and large revocations", Run: results},
		{Name: "public scalar catalog uses target-bound application identity and safe projection", Run: queries},
	}
}

func input() domain.ApplicationAccessInput {
	return domain.ApplicationAccessInput{UserID: 7, ApplicationCode: " SALES ", RoleIDs: []uint64{3, 2, 3}, ExpectedVersion: 9, Reason: " Review access ", Key: "key"}
}

func inputs(t *testing.T) {
	r := input()
	if err := r.ValidateGrant(); err != nil {
		t.Fatal(err)
	}
	n := r.Normalized()
	if n.ApplicationCode != "sales" || n.Reason != "Review access" || !reflect.DeepEqual(n.RoleIDs, []uint64{2, 3}) || !reflect.DeepEqual(r.RoleIDs, []uint64{3, 2, 3}) {
		t.Fatal(r, n)
	}
	n.RoleIDs[0] = 1
	if r.RoleIDs[0] != 3 {
		t.Fatal("input aliased")
	}
	for _, c := range []struct {
		name   string
		change func(*domain.ApplicationAccessInput)
	}{
		{"zero target", func(r *domain.ApplicationAccessInput) { r.UserID = 0 }},
		{"target overflow", func(r *domain.ApplicationAccessInput) { r.UserID = math.MaxInt32 + 1 }},
		{"zero version", func(r *domain.ApplicationAccessInput) { r.ExpectedVersion = 0 }},
		{"negative version", func(r *domain.ApplicationAccessInput) { r.ExpectedVersion = -1 }},
		{"empty code", func(r *domain.ApplicationAccessInput) { r.ApplicationCode = " " }},
		{"invalid code", func(r *domain.ApplicationAccessInput) { r.ApplicationCode = "sales.test" }},
		{"long code", func(r *domain.ApplicationAccessInput) { r.ApplicationCode = strings.Repeat("a", 51) }},
		{"short reason", func(r *domain.ApplicationAccessInput) { r.Reason = " x " }},
		{"long reason", func(r *domain.ApplicationAccessInput) { r.Reason = strings.Repeat("x", 501) }},
		{"empty key", func(r *domain.ApplicationAccessInput) { r.Key = " " }},
		{"long key", func(r *domain.ApplicationAccessInput) { r.Key = strings.Repeat("k", 201) }},
		{"empty roles", func(r *domain.ApplicationAccessInput) { r.RoleIDs = nil }},
		{"invalid role", func(r *domain.ApplicationAccessInput) { r.RoleIDs = []uint64{0} }},
		{"role overflow", func(r *domain.ApplicationAccessInput) { r.RoleIDs = []uint64{math.MaxInt32 + 1} }},
		{"twenty one duplicate entries", func(r *domain.ApplicationAccessInput) {
			r.RoleIDs = make([]uint64, 21)
			for i := range r.RoleIDs {
				r.RoleIDs[i] = 1
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := input()
			c.change(&r)
			if !errors.Is(r.ValidateGrant(), common.ErrBadRequest) {
				t.Fatal(r)
			}
		})
	}
	r = input()
	r.RoleIDs = nil
	if err := r.ValidateRevoke(); err != nil {
		t.Fatal(err)
	}
	r.RoleIDs = []uint64{1}
	if !errors.Is(r.ValidateRevoke(), common.ErrBadRequest) {
		t.Fatal("revoke accepts selection")
	}
	r = input()
	r.RoleIDs = make([]uint64, 20)
	for i := range r.RoleIDs {
		r.RoleIDs[i] = uint64(i + 1)
	}
	if err := r.ValidateGrant(); err != nil {
		t.Fatal(err)
	}
}

func result() domain.SafeCommandResult {
	return domain.SafeCommandResult{UserID: 7, ApplicationID: 2, Status: "ACTIVE", Version: 9, Changed: true}
}

func results(t *testing.T) {
	for _, c := range []struct {
		name  string
		value domain.SafeCommandResult
	}{
		{"access only", result()},
		{"absent no-op", domain.SafeCommandResult{UserID: 7, ApplicationID: 2, Version: 9}},
		{"role only absent access", domain.SafeCommandResult{UserID: 7, ApplicationID: 2, Version: 9, Changed: true, ChangedRoleIDs: []uint64{3}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := c.value.ValidateApplicationAccessResult(7, 2); err != nil {
				t.Fatal(err)
			}
		})
	}
	r := result()
	r.Status = "REVOKED"
	r.ChangedRoleIDs = make([]uint64, 25)
	for i := range r.ChangedRoleIDs {
		r.ChangedRoleIDs[i] = uint64(i + 1)
	}
	if err := r.ValidateApplicationAccessResult(7, 2); err != nil {
		t.Fatal(err)
	}
	if r.ValidateAssignmentResult(7) == nil {
		t.Fatal("C batch bound lost")
	}
	for _, c := range []struct {
		name   string
		change func(*domain.SafeCommandResult)
	}{
		{"foreign user", func(r *domain.SafeCommandResult) { r.UserID = 8 }},
		{"foreign app", func(r *domain.SafeCommandResult) { r.ApplicationID = 3 }},
		{"version", func(r *domain.SafeCommandResult) { r.Version = 0 }},
		{"status", func(r *domain.SafeCommandResult) { r.Status = "DISABLED" }},
		{"noop changed IDs", func(r *domain.SafeCommandResult) { r.Changed = false; r.ChangedRoleIDs = []uint64{3} }},
		{"duplicate IDs", func(r *domain.SafeCommandResult) { r.ChangedRoleIDs = []uint64{3, 3} }},
		{"unordered IDs", func(r *domain.SafeCommandResult) { r.ChangedRoleIDs = []uint64{3, 2} }},
		{"invalid IDs", func(r *domain.SafeCommandResult) { r.ChangedRoleIDs = []uint64{math.MaxInt32 + 1} }},
		{"unrelated role", func(r *domain.SafeCommandResult) { r.RoleID = 3 }},
		{"unrelated profile", func(r *domain.SafeCommandResult) { s := "private"; r.Name = &s }},
		{"unrelated login", func(r *domain.SafeCommandResult) { r.LoginRequired = true }},
		{"unrelated session", func(r *domain.SafeCommandResult) { r.SessionID = "00000000-0000-4000-8000-000000000000" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := result()
			c.change(&r)
			if !errors.Is(r.ValidateApplicationAccessResult(7, 2), common.ErrInternalServerError) {
				t.Fatal(r)
			}
		})
	}
}

func queries(t *testing.T) {
	c := domain.UserApplicationAccessCatalog
	q, err := c.Normalize(domain.CatalogQuery{TargetUserID: 7, Fields: []string{"name"}, Statuses: []string{"REVOKED", "ACTIVE", "ACTIVE"}, Search: &orm.SearchQuery{Keyword: " 50%_ "}})
	if err != nil || q.Page != 1 || q.Limit != 20 || !reflect.DeepEqual(q.Fields, []string{"name", "application_id"}) || !reflect.DeepEqual(q.Statuses, []string{"ACTIVE", "REVOKED"}) || !reflect.DeepEqual(q.Search.Fields, []string{"application_code", "name"}) || q.Search.Keyword != "50%_" || q.Sort[0].Field != "application_id" {
		t.Fatal(q, err)
	}
	at := time.Unix(1735689600, 0).UTC()
	row := domain.ApplicationAccessItem{UserID: 7, ApplicationID: 2, ApplicationCode: "sales", Name: "Sales", Status: "REVOKED", ApplicationStatus: "ACTIVE", GrantedAt: &at}
	projected := row.Project(q)
	if projected.ApplicationID != 2 || projected.Name != "Sales" || projected.Status != "" || projected.UserID != 0 || projected.GrantedAt != nil {
		t.Fatal(projected)
	}
	if len(c.Fields()) != 11 {
		t.Fatal(c.Fields())
	}
	for _, f := range c.Fields() {
		for _, desc := range []bool{false, true} {
			t.Run(f.Name+map[bool]string{true: " DESC", false: " ASC"}[desc], func(t *testing.T) {
				q, err := c.Normalize(domain.CatalogQuery{TargetUserID: 7, Sort: []orm.SortField{{Field: f.Name, Desc: desc}}})
				if err != nil {
					t.Fatal(err)
				}
				values := make([]domain.CatalogValue, len(q.Sort))
				for i, s := range q.Sort {
					values[i] = row.CatalogValue(s.Field)
				}
				q.LastID = c.NextCursor(q, values)
				got, err := c.ParseCursor(q)
				if err != nil || !reflect.DeepEqual(got, values) {
					t.Fatal(got, values, err)
				}
				q.TargetUserID = 8
				if _, err = c.ParseCursor(q); err == nil {
					t.Fatal("cross-user cursor accepted")
				}
			})
		}
	}
	for _, q := range []domain.CatalogQuery{
		{}, {TargetUserID: 7, Fields: []string{"id"}}, {TargetUserID: 7, Fields: []string{"user_id"}},
		{TargetUserID: 7, Sort: []orm.SortField{{Field: "name"}, {Field: "name"}}},
		{TargetUserID: 7, Statuses: []string{"DISABLED"}}, {TargetUserID: 7, Limit: 101},
		{TargetUserID: 7, LastID: "0"}, {TargetUserID: 7, LastID: "bad"},
		{TargetUserID: 7, Search: &orm.SearchQuery{Mode: orm.SearchModeFullText, Keyword: "x"}},
		{TargetUserID: 7, Search: &orm.SearchQuery{Fields: []string{"status"}, Keyword: "x"}},
	} {
		if _, err := c.Normalize(q); err == nil {
			t.Fatal("invalid query accepted", q)
		}
	}
	q, err = c.Normalize(domain.CatalogQuery{TargetUserID: 7, LastID: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ParseCursor(q); err != nil {
		t.Fatal(err)
	}
}
