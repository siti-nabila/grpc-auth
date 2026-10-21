package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func AccessEvaluation() []testutils.Scenario {
	return []testutils.Scenario{{Name: "typed access decisions audit deny once after next and preserve outcomes on audit failure", Run: evaluationAttempt}}
}

func evaluationAttempt(t *testing.T) {
	for _, kind := range []string{"allowed", "missing", "inactive app", "access required", "audit fails", "audit panic", "error", "quota", "nil result", "wrong type", "unspecified", "true deny", "false allowed", "invalid sid", "invalid version", "missing time"} {
		t.Run(kind, func(t *testing.T) {
			method := authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName
			ctx := shared.PermissionCallerContext(t, method, "clm")
			res := &authorizationv1.EvaluateAccessResponse{UserId: 42, Sid: shared.SessionState().SessionID, UserVersion: 4, EvaluatedAt: timestamppb.New(shared.ServiceTime), Reason: authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING}
			var out any = res
			var baseErr error
			wantAudit := true
			invalid := false
			switch kind {
			case "allowed":
				res.Allowed = true
				res.Reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_ALLOWED
				wantAudit = false
			case "inactive app":
				res.Reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_INACTIVE
			case "access required":
				res.Reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_ACCESS_REQUIRED
			case "error":
				out = nil
				baseErr = common.ErrForbidden
			case "quota":
				out = nil
				baseErr = common.ErrRateLimited
				wantAudit = false
			case "nil result":
				out = (*authorizationv1.EvaluateAccessResponse)(nil)
				invalid = true
			case "wrong type":
				out = "private response"
				invalid = true
			case "unspecified":
				res.Reason = 0
				invalid = true
			case "true deny":
				res.Allowed = true
				invalid = true
			case "false allowed":
				res.Reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_ALLOWED
				invalid = true
			case "invalid sid":
				res.Sid = "private invalid sid"
				invalid = true
			case "invalid version":
				res.UserVersion = 0
				invalid = true
			case "missing time":
				res.EvaluatedAt = nil
				invalid = true
			}
			trace := []string{}
			observed := 0
			interceptor, err := interceptors.NewSecurityAttemptInterceptor(func(c context.Context, r domain.AuditRequest) error {
				trace = append(trace, "audit")
				if c.Err() != nil || r.Action != types.AuditSecurityAttemptFailed || r.TargetType != types.AuditTargetRPC || r.TargetID != method || r.Before != nil || r.After != nil {
					t.Fatal("unsafe audit", r)
				}
				reason := types.AuditRequestDenied
				if kind == "error" {
					reason = "COMMON_FORBIDDEN"
				}
				if invalid {
					reason = "COMMON_INTERNAL_SERVER_ERROR"
				}
				if r.Reason != reason {
					t.Fatal("audit reason", r.Reason, reason)
				}
				record, e := r.Record()
				if e != nil || record.ActorUserID != nil || record.ActorService == nil || *record.ActorService != "rest-orc" {
					t.Fatal("unverified audit actor", record, e)
				}
				if kind == "audit fails" {
					return errors.New("controlled audit error")
				}
				if kind == "audit panic" {
					panic("controlled audit panic")
				}
				return nil
			}, func() { observed++ }, func() time.Time { return shared.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			got, err := interceptor(ctx, "private request", &grpc.UnaryServerInfo{FullMethod: method}, func(c context.Context, _ any) (any, error) {
				if c != ctx {
					t.Fatal("context replaced")
				}
				trace = append(trace, "snapshot finished")
				return out, baseErr
			})
			if invalid {
				if got != nil || !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal(got, err)
				}
			} else if got != out || !errors.Is(err, baseErr) {
				t.Fatal("outcome changed", got, err)
			}
			want := []string{"snapshot finished"}
			if wantAudit {
				want = append(want, "audit")
			}
			if !reflect.DeepEqual(trace, want) {
				t.Fatal(trace, want)
			}
			wantObserved := 0
			if kind == "audit fails" || kind == "audit panic" {
				wantObserved = 1
			}
			if observed != wantObserved {
				t.Fatal("audit telemetry", observed, wantObserved)
			}
		})
	}
}
