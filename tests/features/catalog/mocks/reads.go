package mocks

import (
	"context"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/orm/orm"
)

type Snapshot struct {
	domain.Transaction
	Context context.Context
	Steps   []string
	At      int
	Failure error
	Fail    string
	Panic   string
	State   domain.SessionState
	Rows    []domain.PermissionContribution
	Role    domain.RoleSummary
}

func NewSnapshot(ctx context.Context) *Snapshot {
	return &Snapshot{Context: ctx, State: fixtures.SessionState(), Role: domain.RoleSummary{ID: 7, Code: "sales.editor", Name: "Editor", ApplicationID: 2, ApplicationCode: "sales", Status: "ACTIVE", Version: 3, NumericCode: 81, AffectedUserCount: 4}}
}

func (s *Snapshot) Step(ctx context.Context, name string) error {
	if ctx != nil && ctx != s.Context {
		panic("request context changed")
	}
	if s.At >= len(s.Steps) || s.Steps[s.At] != name {
		panic("unexpected read step: " + name)
	}
	s.At++
	if s.Panic == name {
		panic("controlled read panic")
	}
	if s.Fail == name {
		return s.Failure
	}
	return nil
}
func (s *Snapshot) Verify(t *testing.T) {
	t.Helper()
	if s.At != len(s.Steps) {
		t.Fatalf("read steps consumed %d want %d", s.At, len(s.Steps))
	}
}
func (s *Snapshot) BeginSnapshot(ctx context.Context) (domain.Transaction, error) {
	if err := s.Step(ctx, "begin"); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Snapshot) Commit() error   { return s.Step(nil, "commit") }
func (s *Snapshot) Rollback() error { return s.Step(nil, "rollback") }
func (s *Snapshot) GetSessionState(ctx context.Context, id string) (domain.SessionState, error) {
	if id != fixtures.SessionID {
		panic("session target changed")
	}
	return s.State, s.Step(ctx, "state")
}
func (s *Snapshot) Permissions(ctx context.Context, id uint64) ([]domain.PermissionContribution, error) {
	if id != 42 {
		panic("actor target changed")
	}
	return s.Rows, s.Step(ctx, "permissions")
}

type Catalog struct{ Snapshot *Snapshot }

func (r Catalog) Roles(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error) {
	if q.Page != 1 || q.Limit != 20 {
		panic("query not normalized")
	}
	return orm.PageData[domain.RoleSummary]{Items: []domain.RoleSummary{r.Snapshot.Role}}, r.Snapshot.Step(ctx, "roles")
}
func (r Catalog) Permissions(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error) {
	if q.Page != 1 || q.Limit != 20 {
		panic("query not normalized")
	}
	return orm.PageData[domain.PermissionItem]{Items: []domain.PermissionItem{{ID: 9, Code: "sales.order.read"}}}, r.Snapshot.Step(ctx, "catalog-permissions")
}
func (r Catalog) Applications(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error) {
	if q.Page != 1 || q.Limit != 20 {
		panic("query not normalized")
	}
	return orm.PageData[domain.ApplicationItem]{Items: []domain.ApplicationItem{{ID: 2, Code: "sales"}}}, r.Snapshot.Step(ctx, "applications")
}

type Roles struct{ Snapshot *Snapshot }

func (r Roles) Role(ctx context.Context, id uint64) (domain.RoleSummary, error) {
	if id != 7 {
		panic("role target changed")
	}
	return r.Snapshot.Role, r.Snapshot.Step(ctx, "role")
}
func (r Roles) RolePermissions(ctx context.Context, id uint64) ([]domain.PermissionItem, error) {
	if id != 7 {
		panic("role target changed")
	}
	return []domain.PermissionItem{{ID: 9, Code: "sales.order.read"}}, r.Snapshot.Step(ctx, "role-permissions")
}
func (r Roles) Application(context.Context, string) (domain.ApplicationItem, error) {
	panic("unexpected application read")
}
func (r Roles) PermissionSet(context.Context, uint64, []string) ([]domain.PermissionItem, error) {
	panic("unexpected permission set read")
}

func Contribution(code string) []domain.PermissionContribution {
	return []domain.PermissionContribution{{PermissionCode: code, PermissionApplicationID: 1, RoleID: 3, RoleCode: "iam.reader", RoleApplicationID: 1, RoleStatus: "ACTIVE", AssignmentStatus: "ACTIVE", ApplicationCode: "iam", ApplicationStatus: "ACTIVE"}}
}

func (r Roles) RolePermissionPairs(context.Context, uint64) ([]domain.RolePermissionPair, error) {
	panic("unexpected mapping inventory read")
}
