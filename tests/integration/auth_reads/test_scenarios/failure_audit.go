package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc"
)

func FailureAudit() []testutils.Scenario {
	return []testutils.Scenario{{Name: "business read conditional cancel and panic failures rollback before a separate audit transaction", Run: failureAuditAfterRollback}}
}
func failureAuditAfterRollback(t *testing.T) {
	for _, method := range []string{authv1.AuthService_Logout_FullMethodName, userv1.UserService_CreateUser_FullMethodName, userv1.UserService_UpdateUser_FullMethodName, userv1.UserService_ChangeUserStatus_FullMethodName} {
		t.Run(method, func(t *testing.T) { failureAuditMethod(t, method) })
	}
}

func failureAuditMethod(t *testing.T, method string) {
	for _, kind := range []string{"business", "read", "conditional", "cancelled", "panic", "audit failure", "audit panic"} {
		t.Run(kind, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := fixtures.CallerContext(t, base, method, method)
			cause := errors.New("private-business-detail")
			denial := error(common.ErrForbidden)
			reason := "COMMON_FORBIDDEN"
			if kind == "read" {
				denial = featurecommon.DependencyError(cause)
				reason = "COMMON_DEPENDENCY_UNAVAILABLE"
			}
			if kind == "conditional" {
				denial = user.ErrVersionConflict
				reason = "USER_VERSION_CONFLICT"
			}
			if kind == "cancelled" {
				denial = context.Canceled
				reason = string(types.AuditRequestCanceled)
			}
			if kind == "panic" {
				denial = common.ErrInternalServerError
				reason = "COMMON_INTERNAL_SERVER_ERROR"
			}
			steps := []mocks.SQLStep{{Op: "begin"}, {Op: "exec", SQL: "UPDATE sessions SET", Args: []driver.Value{"REVOKED", fixtures.ServiceTime, int64(42), "LOGOUT", int64(42), "ACTIVE", fixtures.SessionID}}}
			if kind == "read" {
				steps = append(steps, mocks.SQLStep{Op: "query", SQL: "WHERE id =", Args: []driver.Value{fixtures.SessionID}, Error: cause})
			}
			if kind == "conditional" {
				steps = append(steps, mocks.SQLStep{Op: "exec", SQL: "WHERE id = $4 AND version = $5 AND auth_version = $6", Args: []driver.Value{int64(5), int64(8), fixtures.ServiceTime, int64(42), int64(4), int64(7)}, Result: driver.RowsAffected(0)})
			}
			steps = append(steps, mocks.SQLStep{Op: "rollback"})
			if kind != "audit panic" {
				audit := mocks.SQLStep{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{nil, "rest-orc", "SECURITY_ATTEMPT_FAILED", "rpc", method, nil, nil, reason, nil, nil, fixtures.ServiceTime}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(1)}}}
				if kind == "audit failure" {
					audit.Error = errors.New("private-audit-detail")
				}
				final := "commit"
				if kind == "audit failure" {
					final = "rollback"
				}
				steps = append(steps, mocks.SQLStep{Op: "begin"}, audit, mocks.SQLStep{Op: final})
			}
			mock := &mocks.SQL{Steps: steps}
			db := mock.Open()
			defer db.Close()
			provider := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
			auditCalls, observed := 0, 0
			interceptor, err := interceptors.NewSecurityAttemptInterceptor(func(attemptCtx context.Context, req domain.AuditRequest) error {
				auditCalls++
				if attemptCtx.Err() != nil {
					t.Fatal("audit reused canceled context")
				}
				if _, ok := attemptCtx.Deadline(); !ok {
					t.Fatal("audit has no bound")
				}
				if req.Reason != types.AuditReason(reason) {
					t.Fatal("reason lost", req.Reason)
				}
				if kind == "audit panic" {
					panic("private-audit-detail")
				}
				return featurecommon.RecordSecurityAttempt(attemptCtx, provider, req)
			}, func() { observed++ }, func() time.Time { return fixtures.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			res, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(requestCtx context.Context, _ any) (res any, err error) {
				if requestCtx != ctx {
					t.Fatal("request context replaced")
				}
				tx, err := provider.Begin(requestCtx)
				if err != nil {
					return nil, err
				}
				owner := featurecommon.NewTransactionOwner(requestCtx, tx)
				defer owner.Finish(&err)
				changed, err := writer.NewSessionRevocationWriter(tx).RevokeSession(requestCtx, domain.SessionRevocation{UserID: 42, ActorID: 42, SessionID: fixtures.SessionID, At: fixtures.ServiceTime, Reason: types.AuditLogout})
				if err != nil || !changed {
					t.Fatal("arranged partial write failed", err)
				}
				switch kind {
				case "read":
					_, err = reader.NewSessionLifecycleReader(tx).GetByID(requestCtx, fixtures.SessionID)
					return nil, featurecommon.DependencyError(err)
				case "conditional":
					changed, err = writer.NewCredentialInvalidationWriter(tx).Invalidate(requestCtx, domain.CredentialInvalidation{UserID: 42, ExpectedVersion: 4, ExpectedAuthVersion: 7, At: fixtures.ServiceTime})
					if err != nil || changed {
						t.Fatal("arranged zero-row guard", changed, err)
					}
					return nil, user.ErrVersionConflict
				case "cancelled":
					cancel()
					return nil, requestCtx.Err()
				case "panic":
					panic("private-business-detail")
				}
				return nil, denial
			})
			matches := errors.Is(err, denial)
			if kind == "read" {
				matches = errors.Is(err, common.ErrDependencyUnavailable) && errors.Is(err, cause)
			}
			if res != nil || !matches {
				t.Fatal("original outcome changed", res, err)
			}
			if auditCalls != 1 || observed != map[bool]int{true: 1, false: 0}[kind == "audit failure" || kind == "audit panic"] {
				t.Fatal("duplicate/missing audit or telemetry", auditCalls, observed)
			}
			if err = mock.Verify(); err != nil {
				t.Fatal(err)
			}
			if db.Stats().InUse != 0 {
				t.Fatal("connection not returned after cleanup")
			}
		})
	}
}
