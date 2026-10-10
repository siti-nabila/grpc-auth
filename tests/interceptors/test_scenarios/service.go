package test_scenarios

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func serviceContext(tokens ...string) context.Context {
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	return metadata.NewIncomingContext(ctx, metadata.MD{serviceauth.MetadataKey: tokens, "x-service-name": {"spoof"}, "user_id": {"999"}, "authorization": {"Bearer ignored-user-token"}})
}

func serviceCredentialValidation(t *testing.T) {
	v, err := serviceauth.NewVerifier(fixtures.ServiceConfig(), func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]string{nil, {""}, {"Bearer " + fixtures.ServiceToken(1)}, {"short"}, {fixtures.ServiceToken(1), fixtures.ServiceToken(1)}, {fixtures.ServiceToken(3)}, {fixtures.ServiceToken(1) + "\n"}, {strings.Repeat("a", 343)}}
	for _, tokens := range cases {
		called := false
		_, err := interceptors.ServiceInterceptor(v)(serviceContext(tokens...), nil, &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Register_FullMethodName}, func(context.Context, any) (any, error) { called = true; return nil, nil })
		if !errors.Is(err, auth.ErrServiceAuthenticationFailed) || called {
			t.Fatal("bad token accepted or business handler called")
		}
	}
	for _, ctx := range []context.Context{context.Background(), metadata.NewIncomingContext(context.Background(), metadata.Pairs(serviceauth.MetadataKey, fixtures.ServiceToken(1)))} {
		if _, err := v.Authenticate(ctx, userv1.UserService_Me_FullMethodName); !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
			t.Fatal("plaintext peer accepted")
		}
	}
	ctx, cancel := context.WithCancel(serviceContext(fixtures.ServiceToken(1)))
	cancel()
	if _, err := v.Authenticate(ctx, userv1.UserService_Me_FullMethodName); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	if _, err := serviceauth.CallerFromContext(context.Background()); !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
		t.Fatal("unverified caller exposed")
	}
}

func serviceRotationPolicy(t *testing.T) {
	now := fixtures.ServiceTime
	cfg := fixtures.ServiceConfig()
	v, err := serviceauth.NewVerifier(cfg, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	// Mutating source config cannot mutate the verifier's authority.
	cfg.Callers[0].Methods[0] = "/spoof.Service/Allow"
	cfg.Callers[0].Namespaces[0] = "spoof"
	cfg.Callers[0].Current.SHA256 = fixtures.ServiceDigest(fixtures.ServiceToken(3))
	request := new(userv1.AuthRequest)
	ctx := serviceContext(fixtures.ServiceToken(1))
	key := struct{}{}
	ctx = context.WithValue(ctx, key, "request-marker")
	var got serviceauth.Caller
	resp, err := interceptors.ServiceInterceptor(v)(ctx, request, &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Register_FullMethodName}, func(inner context.Context, req any) (any, error) {
		if req != request || inner.Value(key) != "request-marker" || inner.Done() != ctx.Done() {
			t.Fatal("request context or argument lost")
		}
		var err error
		got, err = serviceauth.CallerFromContext(inner)
		return request, err
	})
	if err != nil || resp != request || got.ID() != "rest-orc" || got.AuthorizeNamespace("iam") != nil {
		t.Fatal("verified identity changed or spoofed")
	}
	for _, namespace := range []string{"spoof", "iam.user.read", "IAM", "iam-extra", ""} {
		if !errors.Is(got.AuthorizeNamespace(namespace), common.ErrForbidden) {
			t.Fatal("namespace prefix/wildcard bypass")
		}
	}
	for _, method := range []string{"/spoof.Service/Allow", "/user.v1.UserService/MeExtra", "/user.v1.UserService/Unknown"} {
		if _, err := v.Authenticate(serviceContext(fixtures.ServiceToken(1)), method); !errors.Is(err, common.ErrForbidden) {
			t.Fatal("unknown method inherited authority")
		}
	}
	boundary := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, point := range []time.Time{boundary.Add(-time.Nanosecond), boundary, boundary.Add(time.Nanosecond)} {
		now = point
		_, err := v.Authenticate(serviceContext(fixtures.ServiceToken(2)), userv1.UserService_Me_FullMethodName)
		if point.Before(boundary) && err != nil || !point.Before(boundary) && !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
			t.Fatal("overlap expiry boundary incorrect")
		}
		if _, err := v.Authenticate(serviceContext(fixtures.ServiceToken(1)), userv1.UserService_Me_FullMethodName); err != nil {
			t.Fatal("current credential expired with previous")
		}
	}
	now = time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := v.Authenticate(serviceContext(fixtures.ServiceToken(1)), userv1.UserService_Me_FullMethodName); !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
		t.Fatal("current expiry ignored")
	}
}

func concurrentServiceCallers(t *testing.T) {
	v, err := serviceauth.NewVerifier(fixtures.ServiceConfig(), func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, err := v.Authenticate(serviceContext(fixtures.ServiceToken(1)), userv1.UserService_Me_FullMethodName)
			if err != nil {
				t.Error(err)
				return
			}
			caller, err := serviceauth.CallerFromContext(ctx)
			if err != nil || caller.ID() != "rest-orc" || caller.AuthorizeNamespace("clm") != nil {
				t.Error("caller isolation failed")
			}
		}()
	}
	wg.Wait()
}
