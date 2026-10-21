package test_scenarios

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

func ServiceAdmission() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "all37 methods and rotated credentials share early quota", Run: allServiceMethods},
		{Name: "service stage nonrefund precedes identity atomic rejection", Run: nonrefundedService},
	}
}

func allServiceMethods(t *testing.T) {
	cfg := shared.ServiceConfig()
	names := telemetry.MethodNames()
	cfg.Callers[0].Methods = names[:37]
	other := cfg.Callers[0]
	other.ServiceID = "partner"
	other.Previous = nil
	other.Current.SHA256 = shared.ServiceDigest(shared.ServiceToken(3))
	cfg.Callers = append(cfg.Callers, other)
	v, e := serviceauth.NewVerifier(cfg, func() time.Time { return shared.ServiceTime })
	if e != nil {
		t.Fatal(e)
	}
	s, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
	admit, _ := interceptors.NewServiceAdmission(s, rateEncoder())
	calls := 0
	next := func(context.Context, any) (any, error) { calls++; return nil, common.ErrForbidden }
	for n := 0; n < 100; n++ {
		method := names[n%37]
		ctx, e := v.Authenticate(serviceContext(shared.ServiceToken(byte(1+n%2))), method)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = admit(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, next)
	}
	for _, method := range names[:37] {
		ctx, e := v.Authenticate(serviceContext(shared.ServiceToken(2)), method)
		if e != nil {
			t.Fatal(e)
		}
		_, e = admit(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, next)
		if status.Code(e) != codes.ResourceExhausted {
			t.Fatal("uncovered method", method, e)
		}
		details, retry := 0, 0
		for _, d := range status.Convert(e).Details() {
			switch x := d.(type) {
			case *errdetails.ErrorInfo:
				details++
				if x.Metadata["rate_limit_reason"] != "quota" {
					t.Fatal("reason")
				}
			case *errdetails.RetryInfo:
				retry++
				if x.RetryDelay.AsDuration() != time.Second {
					t.Fatal("retry")
				}
			}
		}
		if details != 1 || retry != 1 {
			t.Fatal("duplicate/missing detail")
		}
	}
	if calls != 100 || s.Counts().ServiceQuotaRejected != 37 || s.Counts().Entries != 1 {
		t.Fatal("early side effects", calls, s.Counts())
	}
	before := s.Counts()
	_, _ = admit(context.Background(), nil, &grpc.UnaryServerInfo{}, next)
	if s.Counts() != before {
		t.Fatal("unverified allocation")
	}
	ctx, e := v.Authenticate(serviceContext(shared.ServiceToken(3)), names[0])
	if e != nil {
		t.Fatal(e)
	}
	_, e = admit(ctx, nil, &grpc.UnaryServerInfo{FullMethod: names[0]}, func(context.Context, any) (any, error) { return "other service", nil })
	if e != nil || s.Counts().Entries != 2 {
		t.Fatal("configured service quotas merged", e, s.Counts())
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	before = s.Counts()
	_, _ = admit(canceled, nil, &grpc.UnaryServerInfo{FullMethod: names[0]}, next)
	if s.Counts() != before {
		t.Fatal("pre-canceled stage consumed quota")
	}
}

func nonrefundedService(t *testing.T) {
	s, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
	admit, _ := interceptors.NewServiceAdmission(s, rateEncoder())
	identity, _ := interceptors.NewAuthRateInterceptor(s, []byte("test-hmac-secret"), rateEncoder())
	method := telemetry.MethodNames()[telemetry.MethodIndex("/user.v1.UserService/Login")]
	ctx := rateCaller(t, "rest-orc", method)
	info := &grpc.UnaryServerInfo{FullMethod: method}
	business := 0
	for range 100 {
		_, _ = admit(ctx, nil, info, func(ctx context.Context, req any) (any, error) {
			return identity(ctx, req, info, func(context.Context, any) (any, error) { business++; return nil, common.ErrBadRequest })
		})
	}
	_, e := admit(ctx, nil, info, func(context.Context, any) (any, error) {
		t.Fatal("exhausted service entered identity")
		return nil, nil
	})
	if status.Code(e) != codes.ResourceExhausted || business != 3 || s.Counts().QuotaRejected != 98 || s.Counts().ServiceQuotaRejected != 1 {
		t.Fatal("refund/atomicity", business, s.Counts(), e)
	}
	// Only3 IP tokens were consumed; rejected account checks did not consume IP.
	for range 7 {
		d, _ := s.Admit(ratelimit.Key{Group: ratelimit.LoginIP, Value: "service:rest-orc"})
		if !d.Allowed {
			t.Fatal("identity partial consumption")
		}
	}
}
