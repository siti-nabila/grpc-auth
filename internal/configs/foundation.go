package configs

import (
	"context"
	"database/sql"
	"errors"
	"github.com/siti-nabila/orm/orm"
	"io"
	"sync"
	"time"

	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	ormconfig "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc"
)

// ServerPipeline contains every required boundary; incomplete assembly fails
// before registration or resource creation. Production uses PrepareFoundation.
type ServerPipeline struct {
	Register               func(*grpc.Server)
	Attempt, Session, Rate grpc.UnaryServerInterceptor
	Cleanup                func(context.Context)
	Now                    func() time.Time
}

func (p ServerPipeline) validate() error {
	if p.Register == nil || p.Attempt == nil || p.Session == nil || p.Rate == nil || p.Cleanup == nil || p.Now == nil {
		return errors.New("complete session server pipeline is required")
	}
	return nil
}

type Foundation struct {
	Pipeline ServerPipeline
	Login    *LoginProducer
}

// PrepareFoundation shares one primary pool between all nineteen RPCs. It opens no
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
	registration, err := authfeature.NewPendingRegistrationService(authfeature.PendingRegistrationDependencies{
		Transactions: provider, Bind: authfeature.BindPendingRegistrationRepositories,
		HashPassword: authfeature.DefaultArgon2Config().HashPassword, Now: now, EventID: eventID,
	})
	if err != nil {
		return nil, err
	}
	register, err := handler.NewPendingRegistrationHandler(registration)
	if err != nil {
		return nil, err
	}
	reads, err := handler.NewSessionReadHandler(provider, now)
	if err != nil {
		return nil, err
	}
	profileService, err := profile.NewSessionProfileService(profile.SessionDependencies{
		Transactions: provider, Bind: profile.BindSessionRepositories, Now: now, EventID: eventID,
	})
	if err != nil {
		return nil, err
	}
	profileHandler, err := handler.NewSessionProfileHandler(profileService)
	if err != nil {
		return nil, err
	}
	attempt, err := interceptors.NewSecurityAttemptInterceptor(func(ctx context.Context, req domain.AuditRequest) error {
		return common.RecordSecurityAttempt(ctx, provider, req)
	}, func() { login.auditFailures.Add(1) }, now)
	if err != nil {
		return nil, err
	}
	authServer, sessionServer, rate, err := prepareLifecycle(security, login, provider, provider, orm.NewSqlQueryAdapter(ctx, db, d, cfg), now, entropy, eventID)
	if err != nil {
		return nil, err
	}
	management, err := user.NewManagementService(user.ManagementDependencies{Transactions: provider, Bind: user.BindManagementRepositories, Fingerprinter: fingerprinter, HashPassword: authfeature.DefaultArgon2Config().HashPassword, Now: now, EventID: eventID})
	if err != nil {
		return nil, err
	}
	managementHandler, err := handler.NewUserManagementHandler(management)
	if err != nil {
		return nil, err
	}
	users := &sessionUserServer{login: login.Handler, register: register, reads: reads, management: managementHandler}
	profiles := &sessionProfileServer{handler: profileHandler}
	return &Foundation{Login: login, Pipeline: ServerPipeline{
		Register: func(s *grpc.Server) {
			registerFoundation(s, users, profiles)
			registerLifecycle(s, authServer, sessionServer)
		},
		Attempt: attempt, Session: login.SessionInterceptor, Rate: rate, Now: now,
		Cleanup: func(ctx context.Context) {
			ticker := time.NewTicker(ratelimit.CleanupInterval)
			defer ticker.Stop()
			login.Limiter.RunCleanup(ctx, ticker.C)
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
