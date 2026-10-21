package session

import (
	"context"
	"errors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"time"
)

type AdminSingleInput struct {
	UserID         uint64
	SessionID, Key string
}
type AdminGlobalInput struct {
	UserID          uint64
	ExpectedVersion int64
	Key             string
}
type AdminResult struct {
	Changed bool
	Version int64
}
type AdminService interface {
	RevokeUserSession(context.Context, AdminSingleInput) (*AdminResult, error)
	RevokeAllUserSessions(context.Context, AdminGlobalInput) (*AdminResult, error)
}
type AdminRepositories struct {
	State       domain.SessionStateReader
	Users       domain.SessionUserReader
	Permissions domain.PermissionReader
	Targets     domain.SessionLifecycleReader
	Sessions    domain.SessionRevocationWriter
	Refresh     domain.RefreshRevocationWriter
	Counters    domain.CredentialInvalidationWriter
	Idempotency domain.IdempotencyStore
	Audit       domain.AuditWriter
	Outbox      domain.OutboxWriter
}
type AdminDependencies struct {
	Transactions domain.TransactionProvider
	Bind         func(domain.Transaction) AdminRepositories
	Now          func() time.Time
	EventID      func() (string, error)
}
type adminService struct{ deps AdminDependencies }

func NewAdminService(d AdminDependencies) (AdminService, error) {
	if d.Transactions == nil || d.Bind == nil || d.Now == nil || d.EventID == nil {
		return nil, errors.New("incomplete admin session dependencies")
	}
	return &adminService{deps: d}, nil
}
func BindAdminRepositories(tx domain.Transaction) AdminRepositories {
	return AdminRepositories{State: reader.NewSessionStateReader(tx), Users: reader.NewSessionUserReader(tx), Permissions: reader.NewPermissionReader(tx), Targets: reader.NewSessionLifecycleReader(tx), Sessions: writer.NewSessionRevocationWriter(tx), Refresh: writer.NewRefreshRevocationWriter(tx), Counters: writer.NewCredentialInvalidationWriter(tx), Idempotency: writer.NewIdempotencyStore(tx), Audit: writer.NewAuditWriter(tx), Outbox: writer.NewOutboxWriter(tx)}
}
func (r AdminRepositories) complete() bool {
	return r.State != nil && r.Users != nil && r.Permissions != nil && r.Targets != nil && r.Sessions != nil && r.Refresh != nil && r.Counters != nil && r.Idempotency != nil && r.Audit != nil && r.Outbox != nil
}
