package test_scenarios

import (
	"context"
	"errors"
	"testing"
	"time"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Observer() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "outer observation preserves encoded locale dependency and denial", Run: encodedObservation},
		{Name: "valid allowed false remains OK and panic still propagates", Run: decisionObservation},
	}
}

func encodedObservation(t *testing.T) {
	for _, tc := range []struct {
		err     error
		code    codes.Code
		outcome telemetry.AuthOutcome
	}{
		{nil, codes.OK, telemetry.AuthNone}, {auth.ErrServiceAuthenticationFailed, codes.Unauthenticated, telemetry.AuthFailure}, {auth.ErrInvalidSession, codes.Unauthenticated, telemetry.AuthFailure},
		{common.ErrForbidden, codes.PermissionDenied, telemetry.AuthDenied}, {common.ErrDependencyUnavailable, codes.Unavailable, telemetry.AuthDependency},
		{common.ErrRateLimited, codes.ResourceExhausted, telemetry.AuthNone}, {context.Canceled, codes.Canceled, telemetry.AuthNone}, {context.DeadlineExceeded, codes.DeadlineExceeded, telemetry.AuthNone}, {errors.New("private-error-sentinel"), codes.Internal, telemetry.AuthNone},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			s := telemetry.NewStore()
			now := fixtures.ServiceTime
			observe, err := interceptors.NewMetricsInterceptor(s, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			enc := grpcerror.NewEncoder(append(mapping.CommonCodeMappings(), mapping.AuthCodeMappings()...)...)
			info := &grpc.UnaryServerInfo{FullMethod: telemetry.MethodNames()[0]}
			calls := 0
			ctx := locale.NewContext(context.Background(), "id")
			res, err := observe(ctx, nil, info, func(ctx context.Context, req any) (any, error) {
				return interceptors.NewErrorInterceptor(enc)(ctx, req, info, func(context.Context, any) (any, error) {
					calls++
					now = now.Add(5 * time.Millisecond)
					return "safe", tc.err
				})
			})
			if calls != 1 || status.Code(err) != tc.code {
				t.Fatal("outcome changed", calls, err)
			}
			if tc.err == nil && res != "safe" {
				t.Fatal("response changed")
			}
			r := s.Snapshot(now).RPC[0]
			if r.Codes[tc.code] != 1 || r.DurationNS != uint64(5*time.Millisecond) {
				t.Fatal("observation count/duration", r)
			}
			if tc.outcome != telemetry.AuthNone && r.Auth[tc.outcome-1] != 1 {
				t.Fatal("wrong classification", r.Auth)
			}
			if errors.Is(tc.err, auth.ErrInvalidSession) {
				if x, ok := grpcerror.Decode(err); !ok || x.Message != auth.ErrInvalidSession.Message("id") {
					t.Fatal("locale changed")
				}
			}
		})
	}
	if _, err := interceptors.NewMetricsInterceptor(nil, time.Now); err == nil {
		t.Fatal("missing store accepted")
	}
}

func decisionObservation(t *testing.T) {
	s := telemetry.NewStore()
	observe, _ := interceptors.NewMetricsInterceptor(s, func() time.Time { return fixtures.ServiceTime })
	info := &grpc.UnaryServerInfo{FullMethod: authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName}
	decision := &authorizationv1.EvaluateAccessResponse{UserId: 1, Sid: fixtures.SessionID, UserVersion: 1, EvaluatedAt: timestamppb.New(fixtures.ServiceTime), Reason: authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING}
	r, e := observe(context.Background(), nil, info, func(context.Context, any) (any, error) { return decision, nil })
	i := telemetry.MethodIndex(info.FullMethod)
	if r != decision || e != nil || s.Snapshot(fixtures.ServiceTime).RPC[i].Auth[1] != 1 || s.Snapshot(fixtures.ServiceTime).RPC[i].Codes[codes.OK] != 1 {
		t.Fatal("allowed false changed")
	}
	cleaned := false
	func() {
		defer func() {
			if recover() != "original-panic" {
				t.Error("panic changed")
			}
		}()
		_, _ = observe(context.Background(), nil, info, func(context.Context, any) (any, error) { defer func() { cleaned = true }(); panic("original-panic") })
	}()
	if !cleaned || s.Snapshot(fixtures.ServiceTime).RPC[i].Codes[codes.Internal] != 1 {
		t.Fatal("panic cleanup/observation")
	}
}
