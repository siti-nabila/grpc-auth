package test_scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	userdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
)

func SecurityAttempts() []testutils.Scenario {
	return []testutils.Scenario{{Name: "independent safe attempts preserve denial after rollback and audit failure", Run: securityAttempts}}
}
func securityAttempts(t *testing.T) {
	for _, baseMethod := range []string{userv1.UserService_Me_FullMethodName, userv1.UserService_ListUserRoles_FullMethodName, userv1.UserService_GrantUserRoles_FullMethodName, userv1.UserService_RevokeUserRoles_FullMethodName} {
		t.Run(baseMethod, func(t *testing.T) {
			for _, kind := range []string{"denied", "dependency", "canceled", "audit fails", "audit panic", "unknown caller", "login already audited", "reuse already audited", "quota", "encoded quota", "success", "deadline", "version", "validation", "unknown", "handler panic"} {
				t.Run(kind, func(t *testing.T) {
					base, cancel := context.WithCancel(context.Background())
					defer cancel()
					extra := []string{}
					if baseMethod != userv1.UserService_Me_FullMethodName {
						extra = append(extra, baseMethod)
					}
					ctx := fixtures.CallerContext(t, base, baseMethod, extra...)
					if kind == "unknown caller" {
						ctx = base
					}
					var calls []string
					observed := 0
					denial := error(common.ErrForbidden)
					expectedReason := types.AuditReason("COMMON_FORBIDDEN")
					if kind == "dependency" {
						denial = common.ErrDependencyUnavailable
						expectedReason = "COMMON_DEPENDENCY_UNAVAILABLE"
					}
					if kind == "canceled" {
						denial = context.Canceled
						expectedReason = types.AuditRequestCanceled
					}
					if kind == "login already audited" {
						denial = auth.ErrLoginFailed
					}
					if kind == "deadline" {
						denial = context.DeadlineExceeded
						expectedReason = types.AuditDeadlineExceeded
					}
					if kind == "version" {
						denial = errors.Join(errors.New("private-credential"), userdictionary.ErrVersionConflict)
						expectedReason = "USER_VERSION_CONFLICT"
					}
					if kind == "validation" {
						denial = dictionary.FieldErrors{"private-credential": {auth.ErrRequired}}
						expectedReason = types.AuditValidationFailed
					}
					if kind == "unknown" {
						denial = errors.New("private-credential")
						expectedReason = types.AuditInternalFailure
					}
					if kind == "handler panic" {
						denial = common.ErrInternalServerError
						expectedReason = "COMMON_INTERNAL_SERVER_ERROR"
					}
					if kind == "reuse already audited" {
						denial = auth.ErrRefreshTokenReused
					}
					if kind == "success" {
						denial = nil
					}
					if kind == "quota" {
						denial = common.ErrRateLimited
					}
					if kind == "encoded quota" {
						denial = helpers.HandleErrorContext(ctx, common.ErrRateLimited)
					}
					interceptor, err := interceptors.NewSecurityAttemptInterceptor(func(got context.Context, req domain.AuditRequest) error {
						calls = append(calls, "audit")
						if got.Err() != nil {
							t.Fatal("canceled request reused for independent audit")
						}
						if _, ok := got.Deadline(); !ok {
							t.Fatal("attempt not bounded")
						}
						record, err := req.Record()
						if err != nil || record.ActorUserID != nil || record.ActorService == nil || *record.ActorService != "rest-orc" || req.Action != types.AuditSecurityAttemptFailed || req.Reason != expectedReason || req.Before != nil || req.After != nil {
							t.Fatal("fabricated user or unsafe audit", err)
						}
						data, _ := json.Marshal(record)
						if strings.Contains(string(data), "private-credential") {
							t.Fatal("credential leak")
						}
						if kind == "audit fails" {
							return errors.New("private-credential")
						}
						if kind == "audit panic" {
							panic("private-credential")
						}
						return nil
					}, func() { observed++ }, func() time.Time { return fixtures.ServiceTime })
					if err != nil {
						t.Fatal(err)
					}
					method := baseMethod
					if kind == "login already audited" {
						method = userv1.UserService_Login_FullMethodName
					}
					if kind == "reuse already audited" {
						method = authv1.AuthService_RefreshSession_FullMethodName
					}
					result, err := interceptor(ctx, "private-credential", &grpc.UnaryServerInfo{FullMethod: method}, func(got context.Context, _ any) (any, error) {
						if got != ctx {
							t.Fatal("request context changed")
						}
						calls = append(calls, "rollback")
						if kind == "handler panic" {
							panic("private-credential")
						}
						if kind == "canceled" {
							cancel()
						}
						return nil, denial
					})
					want := []string{"rollback"}
					if kind != "unknown caller" && kind != "login already audited" && kind != "reuse already audited" && kind != "success" && kind != "quota" && kind != "encoded quota" {
						want = append(want, "audit")
					}
					sameError := errors.Is(err, denial)
					if kind == "validation" {
						_, sameError = err.(dictionary.FieldErrors)
					}
					if !reflect.DeepEqual(calls, want) || result != nil || !sameError {
						t.Fatal("outcome changed", calls, err)
					}
					if observed != map[bool]int{true: 1, false: 0}[kind == "audit fails" || kind == "audit panic"] {
						t.Fatal("unbounded or missing telemetry")
					}
				})
			}

		})
	}
}
