package configs

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"google.golang.org/grpc"
	"io"
	"time"
)

func prepareLifecycle(security *LoginSecurity, login *LoginProducer, provider domain.TransactionProvider, snapshots domain.ReadSnapshotProvider, primary domain.QueryReader, now func() time.Time, entropy io.Reader, eventID func() (string, error)) (*sessionAuthServer, *sessionManagementServer, grpc.UnaryServerInterceptor, error) {
	encoder := grpcerror.NewEncoder(append(grpcmapping.CommonCodeMappings(), grpcmapping.AuthCodeMappings()...)...)
	admission, admit, err := interceptors.NewRefreshAdmission(login.Limiter, encoder)
	if err != nil {
		return nil, nil, nil, err
	}
	refresh, err := authfeature.NewRefreshService(authfeature.RefreshDependencies{Transactions: provider, Tokens: reader.NewRefreshReader(primary), State: reader.NewSessionStateReader(primary), Bind: func(tx domain.Transaction) authfeature.RefreshRepositories {
		r := authfeature.BindRefreshRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, login.Metrics)
		return r
	}, Admit: admit, Credentials: func() (authfeature.RotatedCredentials, error) { return authfeature.GenerateRotatedCredentials(entropy) }, Issue: security.Issuer.Issue, EventID: eventID, Now: now})
	if err != nil {
		return nil, nil, nil, err
	}
	refreshHandler, err := handler.NewRefreshHandler(refresh)
	if err != nil {
		return nil, nil, nil, err
	}
	logout, err := authfeature.NewLogoutService(authfeature.LogoutDependencies{Transactions: provider, Tokens: reader.NewRefreshReader(primary), State: reader.NewSessionStateReader(primary), Bind: func(tx domain.Transaction) authfeature.LogoutRepositories {
		r := authfeature.BindLogoutRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, login.Metrics)
		return r
	}, Now: now, EventID: eventID})
	if err != nil {
		return nil, nil, nil, err
	}
	logoutHandler, err := handler.NewLogoutHandler(logout, security.Verifier)
	if err != nil {
		return nil, nil, nil, err
	}
	password, err := authfeature.NewChangePasswordService(authfeature.PasswordDependencies{Transactions: provider, Candidate: reader.NewLoginAccountReader(primary), Bind: func(tx domain.Transaction) authfeature.PasswordRepositories {
		r := authfeature.BindPasswordRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, login.Metrics)
		return r
	}, VerifyPassword: authfeature.VerifyLoginPassword, HashPassword: authfeature.DefaultArgon2Config().HashPassword, Now: now, EventID: eventID})
	if err != nil {
		return nil, nil, nil, err
	}
	passwordHandler, err := handler.NewChangePasswordHandler(password)
	if err != nil {
		return nil, nil, nil, err
	}
	self, err := session.NewSelfService(session.SelfDependencies{Transactions: provider, Bind: func(tx domain.Transaction) session.SelfRepositories {
		r := session.BindSelfRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, login.Metrics)
		return r
	}, Now: now, EventID: eventID})
	if err != nil {
		return nil, nil, nil, err
	}
	selfHandler, err := handler.NewSelfSessionHandler(self)
	if err != nil {
		return nil, nil, nil, err
	}
	lists, err := session.NewListService(session.ListDependencies{Snapshots: snapshots, Bind: session.BindListRepositories, Now: now})
	if err != nil {
		return nil, nil, nil, err
	}
	listHandler, err := handler.NewListSessionsHandler(lists)
	if err != nil {
		return nil, nil, nil, err
	}
	admin, err := session.NewAdminService(session.AdminDependencies{Transactions: provider, Bind: func(tx domain.Transaction) session.AdminRepositories {
		r := session.BindAdminRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, login.Metrics)
		return r
	}, Now: now, EventID: eventID})
	if err != nil {
		return nil, nil, nil, err
	}
	adminHandler, err := handler.NewAdminSessionsHandler(admin)
	if err != nil {
		return nil, nil, nil, err
	}
	rate := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		return login.RateInterceptor(ctx, req, info, func(ctx context.Context, req any) (any, error) { return admission(ctx, req, info, next) })
	}
	return &sessionAuthServer{refresh: refreshHandler, logout: logoutHandler, password: passwordHandler}, &sessionManagementServer{self: selfHandler, lists: listHandler, admin: adminHandler}, rate, nil
}
