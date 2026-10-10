package test_scenarios

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func Sessions() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "authoritative session validates ownership status expiry and auth version", Run: sessionStateValidation},
		{Name: "session dependency and context failures cannot become allow", Run: sessionDependencies},
		{Name: "restricted principal uses exact allowlist and immutable identity", Run: restrictedPrincipal},
	}
}

func callerContext(t *testing.T) context.Context {
	t.Helper()
	v, err := serviceauth.NewVerifier(fixtures.ServiceConfig(), func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, fixtures.ServiceToken(1)))
	ctx, err = v.Authenticate(ctx, "/user.v1.UserService/Me")
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func sessionStateValidation(t *testing.T) {
	claims := fixtures.AccessClaims(t)
	cases := []struct {
		name   string
		change func(*domain.SessionState)
	}{
		{"PENDING", func(s *domain.SessionState) { s.UserStatus = "PENDING" }}, {"SUSPENDED", func(s *domain.SessionState) { s.UserStatus = "SUSPENDED" }}, {"DEACTIVATED", func(s *domain.SessionState) { s.UserStatus = "DEACTIVATED" }},
		{"deleted user", func(s *domain.SessionState) { s.DeletedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true} }},
		{"session revoked", func(s *domain.SessionState) { s.SessionStatus = "REVOKED" }}, {"revoked timestamp", func(s *domain.SessionState) { s.RevokedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true} }},
		{"owner mismatch", func(s *domain.SessionState) { s.SessionUserID = 43 }}, {"user mismatch", func(s *domain.SessionState) { s.UserID = 43 }}, {"session mismatch", func(s *domain.SessionState) { s.SessionID = fixtures.AccessTokenID }},
		{"stale auth version", func(s *domain.SessionState) { s.AuthVersion = 4 }}, {"invalid version", func(s *domain.SessionState) { s.UserVersion = 0 }},
		{"expired exactly", func(s *domain.SessionState) { s.ExpiresAt = fixtures.ServiceTime }}, {"expired before", func(s *domain.SessionState) { s.ExpiresAt = fixtures.ServiceTime.Add(-time.Second) }},
		{"session beyond 7d", func(s *domain.SessionState) { s.ExpiresAt = s.CreatedAt.Add(7*24*time.Hour + time.Nanosecond) }},
		{"token beyond session", func(s *domain.SessionState) { s.ExpiresAt = claims.ExpiresAt().Add(-time.Nanosecond) }},
		{"future creation", func(s *domain.SessionState) { s.CreatedAt = fixtures.ServiceTime.Add(time.Second) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := fixtures.SessionState()
			tc.change(&state)
			if _, err := sessions.PrincipalForState(claims, state, fixtures.ServiceTime); !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("invalid authoritative state accepted")
			}
		})
	}
	state := fixtures.SessionState()
	state.ExpiresAt = claims.ExpiresAt()
	// JWT skew remains valid, but never extends the absolute database expiry.
	if _, err := sessions.PrincipalForState(claims, state, claims.ExpiresAt()); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("JWT skew extended absolute session")
	}
	state = fixtures.SessionState()
	state.UserStatus = "ACTIVE"
	state.SessionStatus = "REVOKED"
	if _, err := sessions.PrincipalForState(claims, state, fixtures.ServiceTime); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("reactivation resurrected revoked session")
	}
}

func sessionDependencies(t *testing.T) {
	claims := fixtures.AccessClaims(t)
	ctx := callerContext(t)
	for _, cause := range []error{nil, auth.ErrNotFound, errors.New("private database detail"), context.Canceled, context.DeadlineExceeded} {
		mock := &mocks.SessionReader{Context: ctx, SessionID: fixtures.SessionID, State: fixtures.SessionState(), Error: cause}
		got, err := authfeature.AuthenticateSession(ctx, claims, mock, fixtures.ServiceTime)
		if mock.Calls != 1 {
			t.Fatal("primary call count")
		}
		switch cause {
		case nil:
			p, er := sessions.PrincipalFromContext(got)
			if err != nil || er != nil || p.UserID() != 42 {
				t.Fatal("valid session not authenticated", err)
			}
		case auth.ErrNotFound:
			if !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("missing session not denied")
			}
		case context.Canceled, context.DeadlineExceeded:
			if !errors.Is(err, cause) {
				t.Fatal("context error lost")
			}
		default:
			if !errors.Is(err, common.ErrDependencyUnavailable) || !errors.Is(err, cause) {
				t.Fatal("dependency failure not preserved", err)
			}
		}
		if err != nil {
			if _, er := sessions.PrincipalFromContext(got); !errors.Is(er, auth.ErrInvalidSession) {
				t.Fatal("failed validation left authenticated principal")
			}
		}
	}
	if _, err := authfeature.AuthenticateSession(ctx, claims, nil, fixtures.ServiceTime); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("missing dependency accepted")
	}
	if _, err := authfeature.AuthenticateSession(context.Background(), claims, nil, fixtures.ServiceTime); !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
		t.Fatal("unverified service reached primary")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := authfeature.AuthenticateSession(cancelled, claims, nil, fixtures.ServiceTime); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled request not stopped")
	}
}

func restrictedPrincipal(t *testing.T) {
	state := fixtures.SessionState()
	state.MustChangePassword = true
	p, err := sessions.PrincipalForState(fixtures.AccessClaims(t), state, fixtures.ServiceTime)
	if err != nil {
		t.Fatal(err)
	}
	state.MustChangePassword = false
	state.UserID = 99
	ctx := p.Context(context.Background())
	got, err := sessions.PrincipalFromContext(ctx)
	if err != nil || got.UserID() != 42 || got.SessionID() != fixtures.SessionID || got.TokenID() != fixtures.AccessTokenID || got.AuthVersion() != 3 || got.UserVersion() != 4 || !got.Restricted() || !got.EvaluatedAt().Equal(fixtures.ServiceTime) {
		t.Fatal("principal mutated")
	}
	for _, method := range []string{"/user.v1.UserService/Me", "/auth.v1.AuthService/ChangePassword", "/auth.v1.AuthService/Logout"} {
		if err := p.AuthorizeMethod(method); err != nil {
			t.Fatal("allowed self method denied")
		}
	}
	for _, method := range []string{"/profile.v1.ProfileService/UpdateProfile", "/user.v1.UserService/ListUsers", "/auth.v1.AuthService/RefreshSession", "/user.v1.UserService/MeExtra", "/other.Service/Me", ""} {
		if !errors.Is(p.AuthorizeMethod(method), common.ErrForbidden) {
			t.Fatal("restricted exact allowlist bypass")
		}
	}
	if _, err := sessions.PrincipalFromContext(context.Background()); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("missing principal accepted")
	}
}
