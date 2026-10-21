package user

import (
	"context"
	"errors"
	"time"

	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

type ManagementService interface {
	CreateUser(context.Context, domain.CreateUserInput) (*domain.SafeCommandResult, error)
	UpdateUser(context.Context, domain.UpdateUserInput) (*domain.SafeCommandResult, error)
	ChangeUserStatus(context.Context, domain.ChangeUserStatusInput) (*domain.SafeCommandResult, error)
}

type ManagementRepositories struct {
	State           domain.SessionStateReader
	Permissions     domain.PermissionReader
	Users           domain.SessionUserReader
	Detail          domain.UserDetailReader
	Policy          domain.ManagementPolicyReader
	Targets         domain.SessionLifecycleReader
	Account         domain.AuthReader
	Role            domain.RegistrationRoleReader
	Create          domain.RegistrationWriter
	Profile         domain.ProfileWriter
	Member          domain.RegistrationMemberWriter
	ProfileMutation domain.ManagementProfileWriter
	StatusMutation  domain.ManagementStatusWriter
	Sessions        domain.SessionRevocationWriter
	Refresh         domain.RefreshRevocationWriter
	Idempotency     domain.IdempotencyStore
	Audit           domain.AuditWriter
	Outbox          domain.OutboxWriter
}

type ManagementDependencies struct {
	Transactions  domain.TransactionProvider
	Bind          func(domain.Transaction) ManagementRepositories
	Fingerprinter *featurecommon.Fingerprinter
	HashPassword  func(string) (string, error)
	Now           func() time.Time
	EventID       func() (string, error)
}

type managementService struct{ deps ManagementDependencies }

func NewManagementService(d ManagementDependencies) (ManagementService, error) {
	if d.Transactions == nil || d.Bind == nil || d.Fingerprinter == nil || d.HashPassword == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete user management dependencies")
	}
	return &managementService{deps: d}, nil
}

func BindManagementRepositories(tx domain.Transaction) ManagementRepositories {
	return ManagementRepositories{
		State: reader.NewSessionStateReader(tx), Permissions: reader.NewPermissionReader(tx), Users: reader.NewSessionUserReader(tx), Detail: reader.NewUserDetailReader(tx), Policy: reader.NewManagementPolicyReader(tx), Targets: reader.NewSessionLifecycleReader(tx),
		Account: reader.NewAuthReaderInTransaction(tx), Role: reader.NewRegistrationRoleReader(tx), Create: writer.NewManagementAccountWriter(tx), Profile: writer.NewProfileWriterInTransaction(tx), Member: writer.NewRegistrationMemberWriter(tx),
		ProfileMutation: writer.NewManagementProfileWriter(tx), StatusMutation: writer.NewManagementStatusWriter(tx), Sessions: writer.NewSessionRevocationWriter(tx), Refresh: writer.NewRefreshRevocationWriter(tx),
		Idempotency: writer.NewIdempotencyStore(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx),
	}
}

func (r ManagementRepositories) complete() bool {
	return r.State != nil && r.Permissions != nil && r.Users != nil && r.Detail != nil && r.Policy != nil && r.Targets != nil && r.Account != nil && r.Role != nil && r.Create != nil && r.Profile != nil && r.Member != nil && r.ProfileMutation != nil && r.StatusMutation != nil && r.Sessions != nil && r.Refresh != nil && r.Idempotency != nil && r.Audit != nil && r.Outbox != nil
}
