package configs

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	ormconfig "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc"
)

type LoginProducer struct {
	Metrics            *telemetry.Store
	Handler            *handler.SessionLoginHandler
	Limiter            *ratelimit.Store
	SessionInterceptor grpc.UnaryServerInterceptor
	RateInterceptor    grpc.UnaryServerInterceptor
	auditFailures      atomic.Uint64
}

func (p *LoginProducer) AuditFailures() uint64 { return p.auditFailures.Load() }

// PrepareLoginProducer owns no pool, listener or background worker. Call
// LoadLoginSecurity before opening startup resources; pass one reusable pool.
// PrepareFoundation supplies the cleanup loop; NewGRPCServer cancels and joins
// it on shutdown or listener failure, and the loop stops its 1-minute ticker.
func PrepareLoginProducer(ctx context.Context, security *LoginSecurity, db *sql.DB, d dialect.Dialector, cfg ormconfig.Config, now func() time.Time, entropy io.Reader) (*LoginProducer, error) {
	if ctx == nil || security == nil || security.Issuer == nil || security.Verifier == nil || len(security.accountSecret) == 0 || db == nil || d == nil || now == nil || entropy == nil {
		return nil, errors.New("incomplete Login producer assembly")
	}
	cfg.EnableDebug = false // Candidate reads contain password hashes as well.
	provider := database.NewTransactionProvider(db, d, cfg)
	p := &LoginProducer{Metrics: telemetry.NewStore()}
	service, err := authfeature.NewSessionLoginService(authfeature.SessionLoginDependencies{
		Transactions: provider, Candidate: reader.NewLoginAccountReader(orm.NewSqlQueryAdapter(ctx, db, d, cfg)), Bind: func(tx domain.Transaction) authfeature.LoginRepositories {
			r := authfeature.BindLoginRepositories(tx)
			r.Audit = observeRequiredAudit(r.Audit, p.Metrics)
			return r
		},
		VerifyPassword: authfeature.VerifyLoginPassword, Issue: security.Issuer.Issue, Now: now,
		Credentials: func() (authfeature.InitialCredentials, error) { return authfeature.GenerateInitialCredentials(entropy) },
		RecordFailure: func(ctx context.Context, r domain.AuditRequest) error {
			return authfeature.RecordFailedLogin(ctx, provider, r)
		}, ObserveAuditFailure: func() { p.auditFailures.Add(1); p.Metrics.AuditFailure(telemetry.LoginAttempt) },
	})
	if err != nil {
		return nil, err
	}
	p.Handler, err = handler.NewSessionLoginHandler(service)
	if err != nil {
		return nil, err
	}
	p.Limiter, err = ratelimit.NewStore(now)
	if err != nil {
		return nil, err
	}
	encoder := grpcerror.NewEncoder(append(grpcmapping.CommonCodeMappings(), grpcmapping.AuthCodeMappings()...)...)
	p.RateInterceptor, err = interceptors.NewAuthRateInterceptor(p.Limiter, security.AccountSecret(), encoder)
	if err != nil {
		return nil, err
	}
	p.SessionInterceptor, err = interceptors.NewSessionInterceptor(security.Verifier, reader.NewSessionStateReader(orm.NewSqlQueryAdapter(ctx, db, d, cfg)), now)
	if err != nil {
		return nil, err
	}
	return p, nil
}
