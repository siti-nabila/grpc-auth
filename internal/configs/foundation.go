package configs

import (
	"context"
	"database/sql"
	"errors"
	auditv1 "github.com/siti-nabila/api-contracts/pb/audit/v1"
	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/orm/orm"
	"io"
	"sync"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/features/audit"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/logger"
	ormconfig "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc"
)

// ServerPipeline contains every required boundary; incomplete assembly fails
// before registration or resource creation. Production uses PrepareFoundation.
type ServerPipeline struct {
	Metrics                *telemetry.Store
	Limiter                *ratelimit.Store
	Register               func(*grpc.Server)
	Attempt, Session, Rate grpc.UnaryServerInterceptor
	Cleanup                func(context.Context)
	Telemetry              func(context.Context)
	Now                    func() time.Time
}

func (p ServerPipeline) validate() error {
	if p.Metrics == nil || p.Limiter == nil || p.Register == nil || p.Attempt == nil || p.Session == nil || p.Rate == nil || p.Cleanup == nil || p.Telemetry == nil || p.Now == nil {
		return errors.New("complete session server pipeline is required")
	}
	return nil
}

type Foundation struct {
	Pipeline ServerPipeline
	Login    *LoginProducer
}

// PrepareFoundation shares one primary pool between all thirty-seven RPCs. It opens no
// resource; NewGRPCServer owns the limiter worker and run owns the pool.
func PrepareFoundation(ctx context.Context, security *LoginSecurity, db *sql.DB, d dialect.Dialector, cfg ormconfig.Config, now func() time.Time, entropy io.Reader, fingerprinter *common.Fingerprinter) (*Foundation, error) {
	if entropy == nil {
		return nil, errors.New("foundation requires entropy")
	}
	entropy = &synchronizedEntropy{source: entropy}
	login, err := PrepareLoginProducer(ctx, security, db, d, cfg, now, entropy)
	if err != nil {
		return nil, err
	}
	cfg.EnableDebug = false
	provider := database.NewTransactionProvider(db, d, cfg)
	eventID := func() (string, error) { return authfeature.GenerateEventID(entropy) }
	register, err := prepareRegistration(provider, login.Metrics, now, eventID)
	if err != nil {
		return nil, err
	}
	reads, err := handler.NewSessionReadHandler(provider, now)
	if err != nil {
		return nil, err
	}
	authorizationHandler, err := handler.NewAuthorizationHandler(provider, now)
	if err != nil {
		return nil, err
	}
	auditService, err := audit.NewService(audit.Dependencies{Snapshots: provider, Bind: audit.BindRepositories, Now: now})
	if err != nil {
		return nil, err
	}
	auditHandler, err := handler.NewAuditHandler(auditService)
	if err != nil {
		return nil, err
	}
	profileHandler, err := prepareProfile(provider, login.Metrics, now, eventID)
	if err != nil {
		return nil, err
	}
	attempt, err := interceptors.NewSecurityAttemptInterceptor(func(ctx context.Context, req domain.AuditRequest) error {
		return common.RecordSecurityAttempt(ctx, provider, req)
	}, func() { login.auditFailures.Add(1); login.Metrics.AuditFailure(telemetry.SecurityAttempt) }, now)
	if err != nil {
		return nil, err
	}
	authServer, sessionServer, rate, err := prepareLifecycle(security, login, provider, provider, orm.NewSqlQueryAdapter(ctx, db, d, cfg), now, entropy, eventID)
	if err != nil {
		return nil, err
	}
	managementHandler, err := prepareUserManagement(provider, login.Metrics, fingerprinter, now, eventID)
	if err != nil {
		return nil, err
	}
	roles, catalogs, err := prepareRoles(provider, login.Metrics, now, eventID)
	if err != nil {
		return nil, err
	}
	assignmentHandler, err := prepareAssignments(provider, login.Metrics, now, eventID)
	if err != nil {
		return nil, err
	}
	accessHandler, err := prepareApplicationAccess(provider, login.Metrics, now, eventID)
	if err != nil {
		return nil, err
	}
	catalogs.access = accessHandler
	users := &sessionUserServer{login: login.Handler, register: register, reads: reads, management: managementHandler, assignments: assignmentHandler, access: accessHandler}
	profiles := &sessionProfileServer{handler: profileHandler}
	write := logger.NewTelemetryWriter(nil, now)
	if logger.Logs != nil {
		write = logger.NewTelemetryWriter(logger.Logs.HTTP, now)
	}
	pending := reader.NewOutboxMetricsReader(orm.NewSqlQueryAdapter(ctx, db, d, cfg))
	sampler, err := telemetry.NewSampler(login.Metrics, telemetry.RuntimeDependencies{
		Pending: func(ctx context.Context) (int64, *time.Time, error) {
			v, err := pending.Pending(ctx)
			if err != nil {
				return 0, nil, err
			}
			if v.Oldest.Valid {
				return v.Count, &v.Oldest.Time, nil
			}
			return v.Count, nil, nil
		}, Pool: db.Stats, Limiter: login.Limiter.Counts, Now: now, Write: write,
	})
	if err != nil {
		return nil, err
	}
	return &Foundation{Login: login, Pipeline: ServerPipeline{
		Metrics: login.Metrics, Limiter: login.Limiter,
		Register: func(s *grpc.Server) {
			registerFoundation(s, users, profiles)
			registerLifecycle(s, authServer, sessionServer)
			registerRoles(s, roles, catalogs)
			authorizationv1.RegisterAuthorizationServiceServer(s, authorizationHandler)
			auditv1.RegisterAuditServiceServer(s, auditHandler)
		},
		Attempt: attempt, Session: login.SessionInterceptor, Rate: rate, Now: now,
		Cleanup: func(ctx context.Context) {
			ticker := time.NewTicker(ratelimit.CleanupInterval)
			defer ticker.Stop()
			login.Limiter.RunCleanup(ctx, ticker.C)
		},
		Telemetry: func(ctx context.Context) {
			ticker := time.NewTicker(telemetry.SampleInterval)
			defer ticker.Stop()
			sampler.Run(ctx, ticker.C)
		},
	}}, nil
}

// One injected entropy reader is shared by concurrent credential/event producers.
type synchronizedEntropy struct {
	mu     sync.Mutex
	source io.Reader
}

func (r *synchronizedEntropy) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.source.Read(p)
}
