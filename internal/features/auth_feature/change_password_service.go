package authfeature

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

type ChangePasswordResult struct{ Changed, LoginRequired bool }
type ChangePasswordService interface {
	ChangePassword(context.Context, domain.ChangePasswordInput) (*ChangePasswordResult, error)
}
type PasswordRepositories struct {
	Account     domain.LoginAccountReader
	State       domain.SessionStateReader
	Targets     domain.SessionLifecycleReader
	Credentials domain.CredentialInvalidationWriter
	Sessions    domain.SessionRevocationWriter
	Refresh     domain.RefreshRevocationWriter
	Audit       domain.AuditWriter
	Outbox      domain.OutboxWriter
}
type PasswordDependencies struct {
	Transactions   domain.TransactionProvider
	Candidate      domain.LoginAccountReader
	Bind           func(domain.Transaction) PasswordRepositories
	VerifyPassword func(string, string) (bool, error)
	HashPassword   func(string) (string, error)
	Now            func() time.Time
	EventID        func() (string, error)
}
type changePasswordService struct{ deps PasswordDependencies }

func NewChangePasswordService(d PasswordDependencies) (ChangePasswordService, error) {
	if d.Transactions == nil || d.Candidate == nil || d.Bind == nil || d.VerifyPassword == nil || d.HashPassword == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete ChangePassword dependencies")
	}
	return &changePasswordService{deps: d}, nil
}
func BindPasswordRepositories(tx domain.Transaction) PasswordRepositories {
	return PasswordRepositories{Account: reader.NewLoginAccountReader(tx), State: reader.NewSessionStateReader(tx), Targets: reader.NewSessionLifecycleReader(tx), Credentials: writer.NewCredentialInvalidationWriter(tx), Sessions: writer.NewSessionRevocationWriter(tx), Refresh: writer.NewRefreshRevocationWriter(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}
func (r PasswordRepositories) complete() bool {
	return r.Account != nil && r.State != nil && r.Targets != nil && r.Credentials != nil && r.Sessions != nil && r.Refresh != nil && r.Audit != nil && r.Outbox != nil
}
