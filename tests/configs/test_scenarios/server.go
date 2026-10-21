package test_scenarios

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"crypto/ed25519"
	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/configs"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	jwtPackage "github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/configs/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func serverConfig(t *testing.T) (*configs.AppConfig, []byte) {
	t.Helper()
	files, cert := fixtures.ServiceCertificate(t, 1, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	return &configs.AppConfig{TLS: files, ServiceAuth: fixtures.ServiceConfig(), JWT: configs.JWTConfig{SecretKey: "test-only-legacy-key"}, Timeout: time.Second}, cert
}

func startMemoryServer(t *testing.T, cfg *configs.AppConfig) (*configs.GRPCServerClient, *bufconn.Listener, *mocks.Calls) {
	t.Helper()
	calls := new(mocks.Calls)
	svr, err := configs.NewGRPCServer(cfg, serverPipeline(t, func(s *grpc.Server) {
		userv1.RegisterUserServiceServer(s, &mocks.Users{Calls: calls})
		profilev1.RegisterProfileServiceServer(s, &mocks.Profiles{Calls: calls})
	}))
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	done := make(chan error, 1)
	go func() { done <- svr.Server.Serve(listener) }()
	t.Cleanup(func() {
		svr.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	return svr, listener, calls
}

func memoryClient(t *testing.T, listener *bufconn.Listener, transport credentials.TransportCredentials) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///auth.test", grpc.WithTransportCredentials(transport), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func clientTLS(cert []byte, hostname string) *tls.Config {
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(cert)
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: hostname, Time: func() time.Time { return fixtures.ServiceTime }}
}

func callRPC(conn *grpc.ClientConn, method string, req, resp proto.Message, values ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(values...))
	return conn.Invoke(ctx, method, req, resp)
}

func secureRPCBoundary(t *testing.T) {
	cfg, cert := serverConfig(t)
	svr, listener, calls := startMemoryServer(t, cfg)
	conn := memoryClient(t, listener, credentials.NewTLS(clientTLS(cert, "auth.test")))
	if len(svr.Server.GetServiceInfo()) != 2 {
		t.Fatal("registered service inventory changed")
	}
	methods := []struct {
		name      string
		req, resp proto.Message
		protected bool
	}{
		{userv1.UserService_Register_FullMethodName, &userv1.AuthRequest{}, &userv1.UserTokenResponse{}, false},
		{userv1.UserService_Login_FullMethodName, &userv1.AuthRequest{}, &userv1.UserTokenResponse{}, false},
		{userv1.UserService_TesRPC_FullMethodName, &emptypb.Empty{}, &userv1.TestRPC{}, false},
		{userv1.UserService_Me_FullMethodName, &emptypb.Empty{}, &userv1.UserData{}, true},
		{userv1.UserService_ListUsers_FullMethodName, &userv1.ListUsersRequest{}, &userv1.ListUsersResponse{}, true},
		{profilev1.ProfileService_UpdateProfile_FullMethodName, &profilev1.ProfileRequest{}, &profilev1.ProfileResponse{}, true},
	}
	access := fixtures.SignAccess(t, fixtures.AccessPayload())
	var err error
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			before := calls.Count.Load()
			for _, token := range []string{"", fixtures.ServiceToken(3)} {
				err := callRPC(conn, method.name, method.req, method.resp, serviceauth.MetadataKey, token, "authorization", "Bearer "+access, locale.MetadataKey, "id")
				decoded, ok := grpcerror.Decode(err)
				if !ok || decoded.Code != codes.Unauthenticated || decoded.Reason != "auth.service_authentication_failed" || decoded.Message != auth.ErrServiceAuthenticationFailed.Message(locale.Indonesian) || calls.Count.Load() != before {
					t.Fatal("service denial/locale/order incorrect", err)
				}
			}
			values := []string{serviceauth.MetadataKey, fixtures.ServiceToken(1), "x-service-name", "spoof", "user_id", "99"}
			if method.protected {
				err := callRPC(conn, method.name, method.req, method.resp, values...)
				if status.Code(err) != codes.Unauthenticated || calls.Count.Load() != before {
					t.Fatal("user auth was bypassed")
				}
				values = append(values, "authorization", "Bearer "+access)
			}
			if err := callRPC(conn, method.name, method.req, method.resp, values...); err != nil {
				t.Fatal(err)
			}
			if calls.Count.Load() != before+1 {
				t.Fatal("business call count incorrect")
			}
		})
	}
	before := calls.Count.Load()
	err = callRPC(conn, userv1.UserService_TesRPC_FullMethodName, &emptypb.Empty{}, &userv1.TestRPC{}, serviceauth.MetadataKey, fixtures.ServiceToken(1), serviceauth.MetadataKey, fixtures.ServiceToken(1))
	if status.Code(err) != codes.Unauthenticated || calls.Count.Load() != before {
		t.Fatal("duplicate metadata reached business")
	}
	err = callRPC(conn, "/user.v1.UserService/Unknown", &emptypb.Empty{}, &emptypb.Empty{}, serviceauth.MetadataKey, fixtures.ServiceToken(1))
	if status.Code(err) != codes.Unimplemented || calls.Count.Load() != before {
		t.Fatal("unknown RPC behavior changed")
	}
}

func callerMethodDenied(t *testing.T) {
	cfg, cert := serverConfig(t)
	cfg.ServiceAuth.Callers[0].Methods = []string{userv1.UserService_TesRPC_FullMethodName}
	_, listener, calls := startMemoryServer(t, cfg)
	conn := memoryClient(t, listener, credentials.NewTLS(clientTLS(cert, "auth.test")))
	err := callRPC(conn, userv1.UserService_ListUsers_FullMethodName, &userv1.ListUsersRequest{}, &userv1.ListUsersResponse{}, serviceauth.MetadataKey, fixtures.ServiceToken(1), locale.MetadataKey, "id")
	got, ok := grpcerror.Decode(err)
	if !ok || got.Code != codes.PermissionDenied || got.Message != common.ErrForbidden.Message(locale.Indonesian) || calls.Count.Load() != 0 {
		t.Fatal("method denial failed", err)
	}
}

func rejectUntrustedTLS(t *testing.T) {
	cfg, cert := serverConfig(t)
	_, listener, calls := startMemoryServer(t, cfg)
	_, other := fixtures.ServiceCertificate(t, 2, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	transports := []struct {
		name        string
		credentials credentials.TransportCredentials
	}{
		{"plaintext", insecure.NewCredentials()},
		{"wrong CA", credentials.NewTLS(clientTLS(other, "auth.test"))},
		{"wrong hostname", credentials.NewTLS(clientTLS(cert, "other.test"))},
	}
	for _, tc := range transports {
		t.Run(tc.name, func(t *testing.T) {
			conn := memoryClient(t, listener, tc.credentials)
			err := callRPC(conn, userv1.UserService_TesRPC_FullMethodName, &emptypb.Empty{}, &userv1.TestRPC{}, serviceauth.MetadataKey, fixtures.ServiceToken(1))
			if status.Code(err) != codes.Unavailable || calls.Count.Load() != 0 {
				t.Fatal("untrusted transport reached handler", err)
			}
		})
	}
}

func rejectServerConfiguration(t *testing.T) {
	for _, name := range []string{"no TLS", "missing file", "invalid PEM", "mismatched key", "expired", "future", "no caller"} {
		t.Run(name, func(t *testing.T) {
			cfg, _ := serverConfig(t)
			switch name {
			case "no TLS":
				cfg.TLS.KeyFile = ""
			case "missing file":
				cfg.TLS.CertFile = "/fixture/does-not-exist.pem"
			case "invalid PEM":
				if err := os.WriteFile(cfg.TLS.KeyFile, []byte("invalid-test-key"), 0600); err != nil {
					t.Fatal(err)
				}
			case "mismatched key":
				other, _ := fixtures.ServiceCertificate(t, 2, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
				cfg.TLS.KeyFile = other.KeyFile
			case "expired":
				cfg.TLS, _ = fixtures.ServiceCertificate(t, 1, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
			case "future":
				cfg.TLS, _ = fixtures.ServiceCertificate(t, 1, time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2041, 1, 1, 0, 0, 0, 0, time.UTC))
			case "no caller":
				cfg.ServiceAuth.Callers = nil
			}
			registered := false
			svr, err := configs.NewGRPCServer(cfg, serverPipeline(t, func(*grpc.Server) { registered = true }))
			if err == nil || svr != nil || registered {
				t.Fatal("invalid security config created server")
			}
		})
	}
	if s, err := configs.NewGRPCServer(nil, configs.ServerPipeline{}); s != nil || err == nil {
		t.Fatal("nil config accepted")
	}
}

func serverFailureCleanup(t *testing.T) {
	cfg, _ := serverConfig(t)
	cfg.Services = map[string]configs.ServiceConfig{"first": {Host: "127.0.0.1", Port: 1}, "last": {Host: "127.0.0.1", Port: -1}}
	var captured *grpc.Server
	svr, err := configs.NewGRPCServer(cfg, serverPipeline(t, func(s *grpc.Server) { captured = s }))
	if err == nil || svr != nil || captured == nil {
		t.Fatal("outbound configuration failure ignored")
	}
	listener := bufconn.Listen(1024)
	defer listener.Close()
	if err := captured.Serve(listener); !errors.Is(err, grpc.ErrServerStopped) {
		t.Fatal("failed constructor left usable server", err)
	}
	cfg.Services = map[string]configs.ServiceConfig{"first": {Host: "127.0.0.1", Port: 1}}
	svr, err = configs.NewGRPCServer(cfg, serverPipeline(t, func(*grpc.Server) {}))
	if err != nil {
		t.Fatal(err)
	}
	defer svr.GracefulStop()
	cfg.Host = "127.0.0.1"
	cfg.Port = -1
	if err := svr.Start(); err == nil {
		t.Fatal("invalid listen address accepted")
	}
	if svr.Services["first"].GetState() != connectivity.Shutdown {
		t.Fatal("startup error leaked outbound client")
	}
	listener2 := bufconn.Listen(1024)
	defer listener2.Close()
	if err := svr.Server.Serve(listener2); !errors.Is(err, grpc.ErrServerStopped) {
		t.Fatal("startup error left server usable")
	}
}

func serverPipeline(t *testing.T, register func(*grpc.Server)) configs.ServerPipeline {
	t.Helper()
	now := func() time.Time { return fixtures.ServiceTime }
	verifier, err := jwtPackage.NewAccessVerifier(fixtures.AccessIssuer, fixtures.AccessKey().Public().(ed25519.PublicKey), now)
	if err != nil {
		t.Fatal(err)
	}
	session, err := interceptors.NewSessionInterceptor(verifier, mocks.State{}, now)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := interceptors.NewSecurityAttemptInterceptor(func(context.Context, domain.AuditRequest) error { return nil }, func() {}, now)
	if err != nil {
		t.Fatal(err)
	}
	limiter, err := ratelimit.NewStore(now)
	if err != nil {
		t.Fatal(err)
	}
	return configs.ServerPipeline{Metrics: telemetry.NewStore(), Limiter: limiter, Register: register, Attempt: attempt, Session: session,
		Rate: func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			return next(ctx, req)
		},
		Cleanup: func(ctx context.Context) { <-ctx.Done() }, Telemetry: func(ctx context.Context) { <-ctx.Done() }, Now: now}
}
