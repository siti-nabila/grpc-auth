package test_scenarios

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func Rate() []testutils.Scenario {
	return []testutils.Scenario{{Name: "rate rejection preserves locale catalog RetryInfo through outer encoder", Run: rateDetails}, {Name: "forwarding trusted only for verified rest-orc and fallback cannot be spoofed", Run: rateTrust}, {Name: "protected reads share verified-user quota and unverified calls allocate nothing", Run: ratePrincipal}}
}

func rateEncoder() *grpcerror.Encoder {
	return grpcerror.NewEncoder(append(grpcmapping.CommonCodeMappings(), grpcmapping.AuthCodeMappings()...)...)
}
func rateCaller(t *testing.T, id, method string, ip ...string) context.Context {
	t.Helper()
	cfg := shared.ServiceConfig()
	cfg.Callers[0].ServiceID = id
	v, err := serviceauth.NewVerifier(cfg, func() time.Time { return shared.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx := serviceContext(shared.ServiceToken(1))
	md, _ := metadata.FromIncomingContext(ctx)
	md = md.Copy()
	md.Set("x-client-ip", ip...)
	ctx = metadata.NewIncomingContext(ctx, md)
	ctx, err = v.Authenticate(ctx, method)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func rateDetails(t *testing.T) {
	s, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
	encoder := rateEncoder()
	secret := []byte("test-only-account-secret")
	i, err := interceptors.NewAuthRateInterceptor(s, secret, encoder)
	if err != nil {
		t.Fatal(err)
	}
	secret[0] ^= 255
	ctx := locale.NewContext(rateCaller(t, "rest-orc", userv1.UserService_Login_FullMethodName), "id")
	info := &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Login_FullMethodName}
	calls := 0
	next := func(context.Context, any) (any, error) { calls++; return "called", nil }
	for _, email := range []string{" MEMBER@example.com ", "member@example.com", "Member@example.com"} {
		if _, err := i(ctx, &userv1.AuthRequest{Email: email}, info, next); err != nil {
			t.Fatal(err)
		}
	}
	_, err = interceptors.NewErrorInterceptor(encoder)(ctx, nil, info, func(ctx context.Context, req any) (any, error) {
		return i(ctx, &userv1.AuthRequest{Email: "member@example.com"}, info, next)
	})
	decoded, ok := grpcerror.Decode(err)
	if !ok || status.Code(err) != codes.ResourceExhausted || calls != 3 || decoded.Code != codes.ResourceExhausted || decoded.Message != common.ErrRateLimited.Message("id") || decoded.RetryInfo == nil || decoded.RetryInfo.RetryDelay.AsDuration() != 12*time.Second {
		t.Fatal("rate catalog lost", err)
	}
	found := false
	reason := false
	for _, detail := range status.Convert(err).Details() {
		switch d := detail.(type) {
		case *errdetails.RetryInfo:
			found = d.RetryDelay.AsDuration() == 12*time.Second
		case *errdetails.ErrorInfo:
			reason = d.Reason == common.ErrRateLimited.Key() && d.Metadata["rate_limit_reason"] == "quota"
		}
	}
	if !found || !reason {
		t.Fatal("retry/reason detail lost", err)
	}
	// Invalid/nil requests consume the canonical empty-account quota safely.
	for n := 0; n < 3; n++ {
		if _, err := i(ctx, (*userv1.AuthRequest)(nil), info, next); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := i(ctx, nil, info, next); status.Code(err) != codes.ResourceExhausted {
		t.Fatal("nil input did not count")
	}
	capStore, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
	for n := 0; n < 10000; n++ {
		capStore.Admit(ratelimit.Key{Group: ratelimit.RegisterIP, Value: strconv.Itoa(n)})
	}
	capInterceptor, _ := interceptors.NewAuthRateInterceptor(capStore, []byte("test-secret"), encoder)
	_, err = capInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Register_FullMethodName}, next)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal("capacity error mapping")
	}
	for _, detail := range status.Convert(err).Details() {
		if retry, ok := detail.(*errdetails.RetryInfo); ok && retry.RetryDelay.AsDuration() != time.Minute {
			t.Fatal("capacity retry")
		}
	}
}

func rateTrust(t *testing.T) {
	for _, kind := range []string{"trusted", "partner", "missing", "malformed", "multiple"} {
		s, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
		i, _ := interceptors.NewAuthRateInterceptor(s, []byte("test-secret"), rateEncoder())
		calls := 0
		for n := 0; n < 3; n++ {
			id := "rest-orc"
			ip := []string{"192.0.2." + strconv.Itoa(n+1)}
			switch kind {
			case "partner":
				id = "partner"
			case "missing":
				ip = nil
			case "malformed":
				ip = []string{"spoof-" + strconv.Itoa(n)}
			case "multiple":
				ip = append(ip, "198.51.100.1")
			}
			ctx := rateCaller(t, id, userv1.UserService_Register_FullMethodName, ip...)
			_, err := i(ctx, nil, &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Register_FullMethodName}, func(context.Context, any) (any, error) { calls++; return nil, nil })
			if n < 2 && err != nil {
				t.Fatal(err)
			}
			if n == 2 && kind != "trusted" && status.Code(err) != codes.ResourceExhausted {
				t.Fatal("spoofed IP bypass", kind)
			}
		}
		want := 2
		if kind == "trusted" {
			want = 3
		}
		if calls != want {
			t.Fatal("trusted/fallback admission", kind, calls)
		}
	}
}

func ratePrincipal(t *testing.T) {
	s, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
	i, _ := interceptors.NewAuthRateInterceptor(s, []byte("test-secret"), rateEncoder())
	info := &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Me_FullMethodName}
	next := func(context.Context, any) (any, error) { return nil, nil }
	if _, err := i(context.Background(), nil, info, next); !errors.Is(err, auth.ErrServiceAuthenticationFailed) || s.Counts().Entries != 0 {
		t.Fatal("unverified limiter allocation")
	}
	ctx := rateCaller(t, "rest-orc", info.FullMethod)
	if _, err := i(ctx, nil, info, next); !errors.Is(err, auth.ErrInvalidSession) || s.Counts().Entries != 0 {
		t.Fatal("unverified user allocation")
	}
	p, err := sessions.PrincipalForState(shared.AccessClaims(t), shared.SessionState(), shared.ServiceTime)
	if err != nil {
		t.Fatal(err)
	}
	ctx = p.Context(ctx)
	for n := 0; n < 20; n++ {
		if n%2 == 0 {
			info.FullMethod = userv1.UserService_Me_FullMethodName
		} else {
			info.FullMethod = userv1.UserService_ListUsers_FullMethodName
		}
		if _, err := i(ctx, nil, info, next); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := i(ctx, nil, info, next); status.Code(err) != codes.ResourceExhausted || s.Counts().Entries != 1 {
		t.Fatal("protected group quota not shared")
	}
}
