package authfeature

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
)

type LogoutResult struct {
	SessionID string
	Changed   bool
}
type LogoutRepositories struct {
	Tokens   domain.RefreshReader
	State    domain.SessionStateReader
	Sessions domain.SessionRevocationWriter
	Refresh  domain.RefreshRevocationWriter
	Audit    domain.AuditWriter
	Outbox   domain.OutboxWriter
}
type LogoutDependencies struct {
	Transactions domain.TransactionProvider
	Tokens       domain.RefreshReader
	State        domain.SessionStateReader
	Bind         func(domain.Transaction) LogoutRepositories
	Now          func() time.Time
	EventID      func() (string, error)
}
type LogoutService interface {
	Logout(context.Context, sessions.LogoutCredential) (*LogoutResult, error)
}
type logoutService struct{ deps LogoutDependencies }

func NewLogoutService(d LogoutDependencies) (LogoutService, error) {
	if d.Transactions == nil || d.Tokens == nil || d.State == nil || d.Bind == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete Logout dependencies")
	}
	return &logoutService{deps: d}, nil
}
func BindLogoutRepositories(tx domain.Transaction) LogoutRepositories {
	return LogoutRepositories{Tokens: reader.NewRefreshReader(tx), State: reader.NewSessionStateReader(tx), Sessions: writer.NewSessionRevocationWriter(tx), Refresh: writer.NewRefreshRevocationWriter(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}
