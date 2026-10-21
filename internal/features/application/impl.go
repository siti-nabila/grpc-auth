package application

import (
	"context"
	"errors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"time"
)

type Service interface {
	ListUserApplicationAccess(context.Context, domain.CatalogQuery) (domain.ApplicationAccessPage, error)
	GrantApplicationAccess(context.Context, domain.ApplicationAccessInput) (*domain.SafeCommandResult, error)
	RevokeApplicationAccess(context.Context, domain.ApplicationAccessInput) (*domain.SafeCommandResult, error)
}

type Repositories struct {
	State          domain.SessionStateReader
	Permissions    domain.PermissionReader
	Users          domain.SessionUserReader
	Applications   domain.ApplicationLookup
	Access         domain.ApplicationAccessReader
	Assignments    domain.UserRoleAssignmentReader
	Mutation       domain.UserRoleAssignmentWriter
	AccessMutation domain.ApplicationAccessWriter
	Idempotency    domain.IdempotencyStore
	Audit          domain.AuditWriter
	Outbox         domain.OutboxWriter
}

type Dependencies struct {
	Transactions domain.TransactionProvider
	Snapshots    domain.ReadSnapshotProvider
	Bind         func(domain.Transaction) Repositories
	Now          func() time.Time
	EventID      func() (string, error)
}

type applicationService struct{ deps Dependencies }

func NewService(d Dependencies) (Service, error) {
	if d.Transactions == nil || d.Snapshots == nil || d.Bind == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete application access dependencies")
	}
	return &applicationService{d}, nil
}

func BindRepositories(tx domain.Transaction) Repositories {
	return Repositories{State: reader.NewSessionStateReader(tx), Permissions: reader.NewPermissionReader(tx), Users: reader.NewSessionUserReader(tx), Applications: reader.NewRoleManagementReader(tx), Access: reader.NewApplicationAccessReader(tx), Assignments: reader.NewUserRoleAssignmentReader(tx), Mutation: writer.NewUserRoleAssignmentWriter(tx), AccessMutation: writer.NewApplicationAccessWriter(tx), Idempotency: writer.NewIdempotencyStore(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}

func (r Repositories) complete() bool {
	return r.State != nil && r.Permissions != nil && r.Users != nil && r.Applications != nil && r.Access != nil && r.Assignments != nil && r.Mutation != nil && r.AccessMutation != nil && r.Idempotency != nil && r.Audit != nil && r.Outbox != nil
}
