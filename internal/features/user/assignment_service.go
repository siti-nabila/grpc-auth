package user

import (
	"context"
	"errors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"time"
)

type AssignmentService interface {
	ListUserRoles(context.Context, domain.CatalogQuery) (domain.UserRoleAssignmentPage, error)
	GrantUserRoles(context.Context, domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error)
	RevokeUserRoles(context.Context, domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error)
}

type AssignmentRepositories struct {
	State       domain.SessionStateReader
	Permissions domain.PermissionReader
	Users       domain.SessionUserReader
	Policy      domain.ManagementPolicyReader
	Assignments domain.UserRoleAssignmentReader
	Mutation    domain.UserRoleAssignmentWriter
	Idempotency domain.IdempotencyStore
	Audit       domain.AuditWriter
	Outbox      domain.OutboxWriter
}

type AssignmentDependencies struct {
	Transactions domain.TransactionProvider
	Snapshots    domain.ReadSnapshotProvider
	Bind         func(domain.Transaction) AssignmentRepositories
	Now          func() time.Time
	EventID      func() (string, error)
}

type assignmentService struct{ deps AssignmentDependencies }

func NewAssignmentService(d AssignmentDependencies) (AssignmentService, error) {
	if d.Transactions == nil || d.Snapshots == nil || d.Bind == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete user role assignment dependencies")
	}
	return &assignmentService{d}, nil
}

func BindAssignmentRepositories(tx domain.Transaction) AssignmentRepositories {
	return AssignmentRepositories{State: reader.NewSessionStateReader(tx), Permissions: reader.NewPermissionReader(tx), Users: reader.NewSessionUserReader(tx), Policy: reader.NewManagementPolicyReader(tx), Assignments: reader.NewUserRoleAssignmentReader(tx), Mutation: writer.NewUserRoleAssignmentWriter(tx), Idempotency: writer.NewIdempotencyStore(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}

func (r AssignmentRepositories) complete() bool {
	return r.State != nil && r.Permissions != nil && r.Users != nil && r.Policy != nil && r.Assignments != nil && r.Mutation != nil && r.Idempotency != nil && r.Audit != nil && r.Outbox != nil
}
