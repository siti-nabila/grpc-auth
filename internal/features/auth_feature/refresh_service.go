package authfeature

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

type RefreshRepositories struct {
	Tokens   domain.RefreshReader
	State    domain.SessionStateReader
	Rotation domain.RefreshRotationWriter
	Family   domain.RefreshRevocationWriter
	Sessions domain.SessionRevocationWriter
	Audit    domain.AuditWriter
	Outbox   domain.OutboxWriter
}

type RefreshDependencies struct {
	Transactions domain.TransactionProvider
	Tokens       domain.RefreshReader
	State        domain.SessionStateReader
	Bind         func(domain.Transaction) RefreshRepositories
	Admit        func(context.Context, string) error
	Credentials  func() (RotatedCredentials, error)
	Issue        func(jwt.AccessIssue) (jwt.IssuedAccess, error)
	EventID      func() (string, error)
	Now          func() time.Time
}

type RefreshService interface {
	Refresh(context.Context, string) (*LoginResult, error)
}
type refreshService struct{ deps RefreshDependencies }

func NewRefreshService(deps RefreshDependencies) (RefreshService, error) {
	if deps.Transactions == nil || deps.Tokens == nil || deps.State == nil || deps.Bind == nil || deps.Admit == nil || deps.Credentials == nil || deps.Issue == nil || deps.EventID == nil || deps.Now == nil {
		return nil, errors.New("incomplete refresh dependencies")
	}
	return &refreshService{deps: deps}, nil
}

func BindRefreshRepositories(tx domain.Transaction) RefreshRepositories {
	return RefreshRepositories{Tokens: reader.NewRefreshReader(tx), State: reader.NewSessionStateReader(tx), Rotation: writer.NewRefreshRotationWriter(tx), Family: writer.NewRefreshRevocationWriter(tx), Sessions: writer.NewSessionRevocationWriter(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}

func (r RefreshRepositories) complete() bool {
	return r.Tokens != nil && r.State != nil && r.Rotation != nil && r.Family != nil && r.Sessions != nil && r.Audit != nil && r.Outbox != nil
}
