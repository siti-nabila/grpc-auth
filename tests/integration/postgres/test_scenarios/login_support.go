//go:build postgres

package test_scenarios

import (
	"bytes"
	"context"
	"crypto/tls"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/configs"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func loginFixture(t *testing.T, restricted bool) (context.Context, *fixtures.Schema) {
	t.Helper()
	ctx, _, fixture := dedicatedFixture(t)
	if _, err := fixture.DB.ExecContext(ctx, "UPDATE auth SET password=$1,must_change_password=$2 WHERE id=1", shared.LoginPasswordHash(), restricted); err != nil {
		t.Fatal("synthetic Login account setup failed")
	}
	return loginCaller(t, ctx, userv1.UserService_Login_FullMethodName), fixture
}

func loginCaller(t *testing.T, ctx context.Context, method string) context.Context {
	t.Helper()
	v, err := serviceauth.NewVerifier(shared.ServiceConfig(), func() time.Time { return shared.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, shared.ServiceToken(1)))
	ctx, err = v.Authenticate(ctx, method)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func loginSecurity(t *testing.T, now func() time.Time) *configs.LoginSecurity {
	t.Helper()
	private, public := shared.AccessPEM(t)
	s, err := configs.LoadLoginSecurity(&configs.AppConfig{ApplicationName: shared.AccessIssuer, AccessIssuer: config.AccessIssuerConfig{PrivateKeyFile: "private", PublicKeyFile: "public", KeyVersion: "test-v1"}, AuthLimiter: config.AuthLimiterConfig{AccountHMACSecret: "test-only-limiter-secret"}}, func(path string) ([]byte, error) {
		if path == "private" {
			return private, nil
		}
		return public, nil
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func loginEntropy(offset byte) *bytes.Reader {
	data := make([]byte, 384)
	for i := range data {
		data[i] = byte(i) + offset
	}
	return bytes.NewReader(data)
}

func loginDependencies(t *testing.T, ctx context.Context, fixture *fixtures.Schema, offset byte) authfeature.SessionLoginDependencies {
	t.Helper()
	now := func() time.Time { return shared.ServiceTime }
	security := loginSecurity(t, now)
	provider := productionProvider(fixture)
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	entropy := loginEntropy(offset)
	return authfeature.SessionLoginDependencies{
		Transactions: provider,
		Candidate:    reader.NewLoginAccountReader(orm.NewSqlQueryAdapter(ctx, fixture.DB, dialect.NewPostgres(), cfg)),
		Bind:         authfeature.BindLoginRepositories, VerifyPassword: authfeature.VerifyLoginPassword, Issue: security.Issuer.Issue, Now: now,
		Credentials: func() (authfeature.InitialCredentials, error) { return authfeature.GenerateInitialCredentials(entropy) },
		RecordFailure: func(ctx context.Context, r domain.AuditRequest) error {
			return authfeature.RecordFailedLogin(ctx, provider, r)
		},
		ObserveAuditFailure: func() { t.Error("independent Login audit failed") },
	}
}

func loginService(t *testing.T, deps authfeature.SessionLoginDependencies) authfeature.SessionLoginService {
	t.Helper()
	s, err := authfeature.NewSessionLoginService(deps)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func loginRequest() domain.AuthRequest {
	return domain.AuthRequest{Email: " FIXTURE@example.test ", Password: "secret12"}
}

func assertLoginRows(t *testing.T, ctx context.Context, fixture *fixtures.Schema, issued, denied int) {
	t.Helper()
	var sessions, refresh, audits, failures, outbox, replay int
	err := fixture.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions),(SELECT count(*) FROM refresh_tokens),
	 (SELECT count(*) FROM audit_logs WHERE action='SESSION_CREATED'),(SELECT count(*) FROM audit_logs WHERE action='LOGIN_FAILED'),
	 (SELECT count(*) FROM outbox_events),(SELECT count(*) FROM idempotency_records)`).Scan(&sessions, &refresh, &audits, &failures, &outbox, &replay)
	if err != nil || sessions != issued || refresh != issued || audits != issued || failures != denied || outbox != 0 || replay != 0 {
		t.Fatalf("issuance counts session/refresh/audit/denial/outbox/replay=%d/%d/%d/%d/%d/%d, want=%d/%d/%d/%d/0/0", sessions, refresh, audits, failures, outbox, replay, issued, issued, issued, denied)
	}
}

type loginResult struct {
	response *authfeature.LoginResult
	err      error
}

func waitLogin(t *testing.T, ctx context.Context, done <-chan loginResult) loginResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		t.Fatal("Login worker did not finish within controlled context")
		return loginResult{}
	}
}
