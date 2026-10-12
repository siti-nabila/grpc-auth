package role

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

type ManagementService interface {
	CreateRole(context.Context, domain.CreateRoleInput) (*domain.SafeCommandResult, error)
	UpdateRole(context.Context, domain.UpdateRoleInput) (*domain.SafeCommandResult, error)
	ChangeRoleStatus(context.Context, domain.ChangeRoleStatusInput) (*domain.SafeCommandResult, error)
	ReplaceRolePermissions(context.Context, domain.ReplaceRolePermissionsInput) (*domain.SafeCommandResult, error)
}

type ManagementRepositories struct {
	State       domain.SessionStateReader
	Permissions domain.PermissionReader
	Roles       domain.RoleManagementReader
	Writer      domain.RoleManagementWriter
	Idempotency domain.IdempotencyStore
	Audit       domain.AuditWriter
	Outbox      domain.OutboxWriter
}

type ManagementDependencies struct {
	Transactions domain.TransactionProvider
	Bind         func(domain.Transaction) ManagementRepositories
	Now          func() time.Time
	EventID      func() (string, error)
}
type managementService struct{ deps ManagementDependencies }

func NewManagementService(d ManagementDependencies) (ManagementService, error) {
	if d.Transactions == nil || d.Bind == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete role management dependencies")
	}
	return &managementService{d}, nil
}
func BindManagementRepositories(tx domain.Transaction) ManagementRepositories {
	return ManagementRepositories{State: reader.NewSessionStateReader(tx), Permissions: reader.NewPermissionReader(tx), Roles: reader.NewRoleManagementReader(tx), Writer: writer.NewRoleManagementWriter(tx), Idempotency: writer.NewIdempotencyStore(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}
func (r ManagementRepositories) complete() bool {
	return r.State != nil && r.Permissions != nil && r.Roles != nil && r.Writer != nil && r.Idempotency != nil && r.Audit != nil && r.Outbox != nil
}
