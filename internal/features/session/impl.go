package session

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

type RevokeResult struct{ Changed bool }
type RevokeAllResult struct{ Changed, LoginRequired bool }

type SelfService interface {
	RevokeMySession(context.Context, string) (*RevokeResult, error)
	RevokeAllMySessions(context.Context) (*RevokeAllResult, error)
}

type SelfRepositories struct {
	State    domain.SessionStateReader
	Targets  domain.SessionLifecycleReader
	Sessions domain.SessionRevocationWriter
	Refresh  domain.RefreshRevocationWriter
	Counters domain.CredentialInvalidationWriter
	Audit    domain.AuditWriter
	Outbox   domain.OutboxWriter
}

type SelfDependencies struct {
	Transactions domain.TransactionProvider
	Bind         func(domain.Transaction) SelfRepositories
	Now          func() time.Time
	EventID      func() (string, error)
}

type selfService struct{ deps SelfDependencies }

func NewSelfService(d SelfDependencies) (SelfService, error) {
	if d.Transactions == nil || d.Bind == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete self session dependencies")
	}
	return &selfService{deps: d}, nil
}

func BindSelfRepositories(tx domain.Transaction) SelfRepositories {
	return SelfRepositories{State: reader.NewSessionStateReader(tx), Targets: reader.NewSessionLifecycleReader(tx), Sessions: writer.NewSessionRevocationWriter(tx), Refresh: writer.NewRefreshRevocationWriter(tx), Counters: writer.NewCredentialInvalidationWriter(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}

func (r SelfRepositories) complete() bool {
	return r.State != nil && r.Targets != nil && r.Sessions != nil && r.Refresh != nil && r.Counters != nil && r.Audit != nil && r.Outbox != nil
}
