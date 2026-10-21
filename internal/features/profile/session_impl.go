package profile

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

type UpdateResult struct {
	Profile domain.Profile
	Version int64
	Changed bool
}

type SessionProfileService interface {
	UpdateProfile(context.Context, domain.ProfileUpdate) (*UpdateResult, error)
}

type SessionRepositories struct {
	State   domain.SessionStateReader
	Reader  domain.ProfileReader
	Writer  domain.ProfileWriter
	Version domain.UserVersionWriter
	Audit   domain.AuditWriter
	Outbox  domain.OutboxWriter
}

type SessionDependencies struct {
	Transactions domain.TransactionProvider
	Bind         func(domain.Transaction) SessionRepositories
	Now          func() time.Time
	EventID      func() (string, error)
}

type sessionProfileService struct{ deps SessionDependencies }

func NewSessionProfileService(deps SessionDependencies) (SessionProfileService, error) {
	if deps.Transactions == nil || deps.Bind == nil || deps.Now == nil || deps.EventID == nil {
		return nil, errors.New("incomplete session profile dependencies")
	}
	return &sessionProfileService{deps: deps}, nil
}

func BindSessionRepositories(tx domain.Transaction) SessionRepositories {
	return SessionRepositories{State: reader.NewSessionStateReader(tx), Reader: reader.NewProfileReaderInTransaction(tx), Writer: writer.NewProfileWriterInTransaction(tx), Version: writer.NewUserVersionWriter(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}
