package authfeature

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type RegistrationResult struct {
	ID     uint64
	Status types.UserStatus
}
type PendingRegistrationService interface {
	Register(context.Context, domain.AuthRequest) (*RegistrationResult, error)
}
type PendingRegistrationRepositories struct {
	Account domain.AuthReader
	Role    domain.RegistrationRoleReader
	Auth    domain.RegistrationWriter
	Profile domain.ProfileWriter
	Member  domain.RegistrationMemberWriter
	Audit   domain.AuditWriter
	Outbox  domain.OutboxWriter
}
type PendingRegistrationDependencies struct {
	Transactions domain.TransactionProvider
	Bind         func(domain.Transaction) PendingRegistrationRepositories
	HashPassword func(string) (string, error)
	Now          func() time.Time
	EventID      func() (string, error)
}
type pendingRegistrationService struct {
	deps PendingRegistrationDependencies
}

func NewPendingRegistrationService(deps PendingRegistrationDependencies) (PendingRegistrationService, error) {
	if deps.Transactions == nil || deps.Bind == nil || deps.HashPassword == nil || deps.Now == nil || deps.EventID == nil {
		return nil, errors.New("incomplete pending registration dependencies")
	}
	return &pendingRegistrationService{deps: deps}, nil
}

func BindPendingRegistrationRepositories(tx domain.Transaction) PendingRegistrationRepositories {
	return PendingRegistrationRepositories{Account: reader.NewAuthReaderInTransaction(tx), Role: reader.NewRegistrationRoleReader(tx), Auth: writer.NewRegistrationWriter(tx), Profile: writer.NewProfileWriterInTransaction(tx), Member: writer.NewRegistrationMemberWriter(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}
