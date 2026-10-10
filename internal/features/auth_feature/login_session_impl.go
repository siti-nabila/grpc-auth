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

type LoginResult struct {
	AccessToken, RefreshToken, SessionID string
	AccessExpiresAt, SessionExpiresAt    time.Time
	MustChangePassword                   bool
}

type SessionLoginService interface {
	Login(context.Context, domain.AuthRequest) (*LoginResult, error)
}

type LoginRepositories struct {
	Account domain.LoginAccountReader
	Session domain.SessionWriter
	Refresh domain.InitialRefreshWriter
	Audit   domain.AuditWriter
}

type SessionLoginDependencies struct {
	Transactions        domain.TransactionProvider
	Candidate           domain.LoginAccountReader
	Bind                func(domain.Transaction) LoginRepositories
	VerifyPassword      func(string, string) (bool, error)
	Issue               func(jwt.AccessIssue) (jwt.IssuedAccess, error)
	Credentials         func() (InitialCredentials, error)
	Now                 func() time.Time
	RecordFailure       func(context.Context, domain.AuditRequest) error
	ObserveAuditFailure func()
}

type sessionLoginService struct{ deps SessionLoginDependencies }

func NewSessionLoginService(deps SessionLoginDependencies) (SessionLoginService, error) {
	if deps.Transactions == nil || deps.Candidate == nil || deps.Bind == nil || deps.VerifyPassword == nil || deps.Issue == nil || deps.Credentials == nil || deps.Now == nil || deps.RecordFailure == nil || deps.ObserveAuditFailure == nil {
		return nil, errors.New("incomplete session Login dependencies")
	}
	return &sessionLoginService{deps: deps}, nil
}

func BindLoginRepositories(tx domain.Transaction) LoginRepositories {
	return LoginRepositories{Account: reader.NewLoginAccountReader(tx), Session: writer.NewSessionWriter(tx), Refresh: writer.NewInitialRefreshWriter(tx), Audit: writer.NewAuditWriter(tx)}
}
