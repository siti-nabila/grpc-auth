package test_scenarios

import (
	"context"
	"errors"
	auditv1 "github.com/siti-nabila/api-contracts/pb/audit/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"reflect"
	"testing"
	"time"
)

func AuditLogReads() []testutils.Scenario {
	return []testutils.Scenario{{Name: "audit-list success has no audit and error attempt failure cannot change original result", Run: auditAttempts}}
}
func auditAttempts(t *testing.T) {
	for _, kind := range []string{"success", "error", "audit fails", "audit panic", "quota"} {
		t.Run(kind, func(t *testing.T) {
			ctx := shared.PermissionCallerContext(t, domain.AuditLogMethod, "iam")
			trace := []string{}
			observed := 0
			var original error = common.ErrForbidden
			if kind == "success" {
				original = nil
			}
			if kind == "quota" {
				original = common.ErrRateLimited
			}
			out := &auditv1.ListAuditLogsResponse{}
			attempt, err := interceptors.NewSecurityAttemptInterceptor(func(c context.Context, r domain.AuditRequest) error {
				trace = append(trace, "audit")
				row, err := r.Record()
				if err != nil || c.Err() != nil || row.ActorUserID != nil || row.ActorService == nil || *row.ActorService != "rest-orc" || r.TargetType != types.AuditTargetRPC || r.TargetID != domain.AuditLogMethod || r.Before != nil || r.After != nil || r.Reason != "COMMON_FORBIDDEN" {
					t.Fatal("unsafe attempt", r, err)
				}
				if kind == "audit fails" {
					return errors.New("controlled failure")
				}
				if kind == "audit panic" {
					panic("controlled panic")
				}
				return nil
			}, func() { observed++ }, func() time.Time { return shared.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			res, err := attempt(ctx, &auditv1.ListAuditLogsRequest{Filter: &auditv1.AuditLogFilter{ActorService: "private selector"}}, &grpc.UnaryServerInfo{FullMethod: domain.AuditLogMethod}, func(c context.Context, _ any) (any, error) {
				if c != ctx {
					t.Fatal("context changed")
				}
				trace = append(trace, "snapshot finished")
				return out, original
			})
			if res != out || !errors.Is(err, original) {
				t.Fatal("outcome changed", res, err)
			}
			want := []string{"snapshot finished"}
			if original != nil && kind != "quota" {
				want = append(want, "audit")
			}
			if !reflect.DeepEqual(trace, want) {
				t.Fatal(trace, want)
			}
			failures := 0
			if kind == "audit fails" || kind == "audit panic" {
				failures = 1
			}
			if observed != failures {
				t.Fatal(observed, failures)
			}
		})
	}
}
