package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/orm/orm"
	"testing"
)

type Snapshot struct {
	domain.Transaction
	Context     context.Context
	Steps       []string
	At          int
	Fail, Panic string
	Failure     error
	State       domain.SessionState
	Rows        []domain.PermissionContribution
	Empty       bool
}

func New(ctx context.Context) *Snapshot {
	return &Snapshot{Context: ctx, State: fixtures.SessionState(), Rows: []domain.PermissionContribution{{PermissionCode: "iam.audit.read", PermissionApplicationID: 1, RoleID: 3, RoleCode: "iam.auditor", RoleApplicationID: 1, RoleStatus: "ACTIVE", AssignmentStatus: "ACTIVE", ApplicationCode: "iam", ApplicationStatus: "ACTIVE"}}}
}
func (s *Snapshot) Step(ctx context.Context, name string) error {
	if ctx != nil && ctx != s.Context {
		panic("context changed")
	}
	if s.At >= len(s.Steps) || s.Steps[s.At] != name {
		panic("unexpected step " + name)
	}
	s.At++
	if s.Panic == name {
		panic("controlled panic")
	}
	if s.Fail == name {
		return s.Failure
	}
	return nil
}
func (s *Snapshot) Verify(t *testing.T) {
	t.Helper()
	if s.At != len(s.Steps) {
		t.Fatalf("steps %d/%d", s.At, len(s.Steps))
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
		panic("session changed")
	}
	return s.State, s.Step(ctx, "state")
}
func (s *Snapshot) Permissions(ctx context.Context, id uint64) ([]domain.PermissionContribution, error) {
	if id != 42 {
		panic("actor changed")
	}
	return s.Rows, s.Step(ctx, "permissions")
}
func (s *Snapshot) List(ctx context.Context, q domain.AuditLogQuery) (orm.PageData[domain.AuditLogItem], error) {
	if q.Page != 1 || q.Limit != 20 || q.ActorUserID == nil || *q.ActorUserID != 7 {
		panic("historical query changed")
	}
	p := orm.PageData[domain.AuditLogItem]{}
	if !s.Empty {
		p.Items = []domain.AuditLogItem{{ID: 2147483648, OccurredAt: fixtures.ServiceTime}}
	}
	return p, s.Step(ctx, "audit")
}
