package test_scenarios

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/configs"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func Producer() []testutils.Scenario {
	return []testutils.Scenario{{Name: "actual Login assembly persists atomically and feeds foundation consumers", Run: producerFlow}, {Name: "production Login denial and rate rejection preserve safe transport details", Run: producerDenial}}
}

func producerFlow(t *testing.T) {
	passwordHash := fixtures.LoginPasswordHash()
	for _, kind := range []string{"normal", "restricted", "session failure", "refresh failure", "audit failure", "commit failure"} {
		t.Run(kind, func(t *testing.T) {
			now := fixtures.ServiceTime
			clock := func() time.Time { return now }
			ctx := callerContext(t, userv1.UserService_Login_FullMethodName)
			priv, pub := fixtures.AccessPEM(t)
			security, err := configs.LoadLoginSecurity(&configs.AppConfig{ApplicationName: fixtures.AccessIssuer, AccessIssuer: config.AccessIssuerConfig{PrivateKeyFile: "private", PublicKeyFile: "public", KeyVersion: "v1"}, AuthLimiter: config.AuthLimiterConfig{AccountHMACSecret: "test-only-account-secret"}}, func(path string) ([]byte, error) {
				if path == "private" {
					return priv, nil
				}
				return pub, nil
			}, clock)
			if err != nil {
				t.Fatal(err)
			}
			data := make([]byte, 96)
			for n := range data {
				data[n] = byte(n)
			}
			c, err := authfeature.GenerateInitialCredentials(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			end := now.Add(7 * 24 * time.Hour)
			restricted := kind == "restricted"
			accountCols := []string{"id", "email", "password", "status", "auth_version", "version", "must_change_password", "deleted_at"}
			accountRow := []driver.Value{int64(42), "member@example.com", passwordHash, "ACTIVE", int64(3), int64(4), restricted, nil}
			steps := []mocks.SQLStep{{Op: "query", SQL: "FROM auth", Args: []driver.Value{"member@example.com"}, Columns: accountCols, Rows: [][]driver.Value{accountRow}}, {Op: "begin"}}
			d := dialect.NewPostgres()
			for _, key := range []string{"auth:user:42", "auth:session:" + c.SessionID} {
				query, args, err := d.TryLockQuery(ctx, key)
				if err != nil {
					t.Fatal(err)
				}
				steps = append(steps, mocks.SQLStep{Op: "query", SQL: query, Args: []driver.Value{args[0]}, Columns: []string{"acquired"}, Rows: [][]driver.Value{{true}}})
			}
			steps = append(steps, mocks.SQLStep{Op: "query", SQL: "FROM auth", Args: []driver.Value{int64(42)}, Columns: accountCols, Rows: [][]driver.Value{accountRow}})
			writes := []mocks.SQLStep{
				{Op: "query", SQL: "INSERT INTO sessions", Args: []driver.Value{c.SessionID, int64(42), "ACTIVE", now, now, end, nil, nil, nil, nil, nil, nil, int64(3)}, Columns: []string{"id"}, Rows: [][]driver.Value{{c.SessionID}}},
				{Op: "query", SQL: "INSERT INTO refresh_tokens", Args: []driver.Value{c.RefreshID, c.SessionID, c.RefreshDigest, c.FamilyID, now, end, nil, nil, nil}, Columns: []string{"id"}, Rows: [][]driver.Value{{c.RefreshID}}},
				{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{int64(42), nil, "SESSION_CREATED", "session", c.SessionID, nil, `{"status":"ACTIVE","session_id":"` + c.SessionID + `"}`, nil, nil, nil, now}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(1)}}},
				{Op: "commit"},
			}
			cause := errors.New("controlled production adapter failure")
			stop := len(writes)
			switch kind {
			case "session failure":
				stop = 1
			case "refresh failure":
				stop = 2
			case "audit failure":
				stop = 3
			case "commit failure":
				stop = 4
			}
			if kind != "normal" && kind != "restricted" {
				writes[stop-1].Error = cause
			}
			steps = append(steps, writes[:stop]...)
			if stop < 4 {
				steps = append(steps, mocks.SQLStep{Op: "rollback"})
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			defer db.Close()
			producer, err := configs.PrepareLoginProducer(ctx, security, db, d, database.GetORMConfig(), clock, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			info := &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Login_FullMethodName}
			var response *userv1.UserTokenResponse
			_, err = producer.SessionInterceptor(ctx, nil, info, func(ctx context.Context, req any) (any, error) {
				return producer.RateInterceptor(ctx, &userv1.AuthRequest{Email: " MEMBER@example.com ", Password: "secret12"}, info, func(ctx context.Context, req any) (any, error) {
					var err error
					response, err = producer.Handler.Login(ctx, req.(*userv1.AuthRequest))
					return response, err
				})
			})
			wantAuditFailures := uint64(0)
			if kind == "audit failure" {
				wantAuditFailures = 1
			}
			if producer.Metrics.Snapshot(now).Audit[telemetry.RequiredWrite] != wantAuditFailures {
				t.Fatal("required audit observation")
			}
			if kind != "normal" && kind != "restricted" {
				if !errors.Is(err, cause) || response != nil {
					t.Fatal("failed production adapter exposed credentials", err)
				}
				if err := mock.Verify(); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err != nil || response == nil || response.SessionId != c.SessionID || response.RefreshToken != c.RefreshToken || response.MustChangePassword != restricted || !response.AccessExpiresAt.AsTime().Equal(now.Add(15*time.Minute)) || !response.SessionExpiresAt.AsTime().Equal(end) {
				t.Fatal("real producer response", err)
			}
			_, err = security.Verifier.Verify(response.Token)
			if err != nil {
				t.Fatal(err)
			}
			stateStep := mocks.SQLStep{Op: "query", SQL: "JOIN auth a ON a.id = s.user_id", Args: []driver.Value{response.SessionId}, Columns: []string{"user_id", "user_status", "auth_version", "user_version", "must_change_password", "deleted_at", "session_id", "session_user_id", "session_status", "created_at", "expires_at", "revoked_at", "session_auth_version"}, Rows: [][]driver.Value{{int64(42), "ACTIVE", int64(3), int64(4), restricted, nil, response.SessionId, int64(42), "ACTIVE", now, end, nil, int64(3)}}}
			// Returned state is the expected result of actual session INSERT above;
			// only the SQL boundary is faked, never the Login producer or signer.
			provider := database.NewTransactionProvider(db, d, database.GetORMConfig())
			for _, method := range []string{userv1.UserService_Me_FullMethodName, userv1.UserService_ListUsers_FullMethodName} {
				requestCtx := callerContext(t, method)
				md, _ := metadata.FromIncomingContext(requestCtx)
				md = md.Copy()
				md.Set("authorization", "Bearer "+response.Token)
				requestCtx = metadata.NewIncomingContext(requestCtx, md)
				mock.Context = requestCtx
				mock.Steps = append(mock.Steps, stateStep)
				if method == userv1.UserService_Me_FullMethodName {
					mock.Steps = append(mock.Steps, mocks.SQLStep{Op: "begin"}, stateStep, mocks.SQLStep{Op: "query", SQL: "SELECT id, email, status, version, must_change_password FROM auth", Args: []driver.Value{int64(42)}, Columns: []string{"id", "email", "status", "version", "must_change_password"}, Rows: [][]driver.Value{{int64(42), "member@example.com", "ACTIVE", int64(4), restricted}}}, mocks.SQLStep{Op: "query", SQL: "FROM profile", Args: []driver.Value{int64(42)}, Columns: []string{"id", "user_id", "name", "address", "phone"}, Rows: [][]driver.Value{{int64(7), int64(42), "Member", "Address", "123"}}})
					if !restricted {
						mock.Steps = append(mock.Steps, mocks.SQLStep{Op: "query", SQL: "ORDER BY r.id ASC", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE"}, Columns: []string{"id", "role_name", "role_code", "role_description"}, Rows: [][]driver.Value{{int64(1), "Member", int64(1), nil}}})
					}
					mock.Steps = append(mock.Steps, mocks.SQLStep{Op: "commit"})
				} else if !restricted {
					mock.Steps = append(mock.Steps, mocks.SQLStep{Op: "begin"}, stateStep, permissionStep(false), mocks.SQLStep{Op: "rollback"})
				}
				info := &grpc.UnaryServerInfo{FullMethod: method}
				result, err := producer.SessionInterceptor(requestCtx, nil, info, func(inner context.Context, req any) (any, error) {
					return producer.RateInterceptor(inner, req, info, func(inner context.Context, _ any) (any, error) {
						mock.Context = inner
						if _, err := serviceauth.CallerFromContext(inner); err != nil {
							t.Fatal("caller context lost")
						}
						if method == userv1.UserService_Me_FullMethodName {
							reads, err := handler.NewSessionReadHandler(provider, clock)
							if err != nil {
								return nil, err
							}
							return reads.Me(inner, nil)
						}
						reads, err := handler.NewSessionReadHandler(provider, clock)
						if err != nil {
							return nil, err
						}
						return reads.ListUsers(inner, &userv1.ListUsersRequest{})
					})
				})
				if method == userv1.UserService_Me_FullMethodName {
					res, ok := result.(*userv1.UserData)
					if err != nil || !ok || res.Id != 42 || restricted && len(res.RoleCodes) != 0 {
						t.Fatal("producer credential Me failure", err)
					}
				} else if !errors.Is(err, common.ErrForbidden) {
					t.Fatal("read quota bypassed permission/restriction", err)
				}
			}
			// Invalid legacy/sid-less bearer cannot reach a session query or handler.
			if _, err := producer.SessionInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Me_FullMethodName}, func(context.Context, any) (any, error) { t.Fatal("missing bearer reached consumer"); return nil, nil }); !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("missing bearer accepted")
			}
			if _, err := producer.SessionInterceptor(context.Background(), nil, info, func(context.Context, any) (any, error) { t.Fatal("unverified caller reached Login"); return nil, nil }); !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
				t.Fatal("direct caller accepted", err)
			}
			// Exercise absolute persisted expiry while access is still valid. The
			// verifier's skew must not extend the database deadline by 30 seconds.
			requestCtx := callerContext(t, userv1.UserService_Me_FullMethodName)
			md, _ := metadata.FromIncomingContext(requestCtx)
			md = md.Copy()
			md.Set("authorization", "Bearer "+response.Token)
			requestCtx = metadata.NewIncomingContext(requestCtx, md)
			mock.Context = requestCtx
			expired := stateStep
			expired.Rows = [][]driver.Value{{int64(42), "ACTIVE", int64(3), int64(4), restricted, nil, response.SessionId, int64(42), "ACTIVE", now.Add(-7 * 24 * time.Hour), now, nil}}
			mock.Steps = append(mock.Steps, expired)
			if _, err := producer.SessionInterceptor(requestCtx, nil, &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Me_FullMethodName}, func(context.Context, any) (any, error) { t.Fatal("absolute expiry reached consumer"); return nil, nil }); !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("absolute expiry extended", err)
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func producerDenial(t *testing.T) {
	priv, pub := fixtures.AccessPEM(t)
	now := func() time.Time { return fixtures.ServiceTime }
	security, err := configs.LoadLoginSecurity(&configs.AppConfig{ApplicationName: fixtures.AccessIssuer, AccessIssuer: config.AccessIssuerConfig{PrivateKeyFile: "private", PublicKeyFile: "public", KeyVersion: "v1"}, AuthLimiter: config.AuthLimiterConfig{AccountHMACSecret: "test-only-account-secret"}}, func(path string) ([]byte, error) {
		if path == "private" {
			return priv, nil
		}
		return pub, nil
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"unknown", "mismatch", "pending"} {
		t.Run(kind, func(t *testing.T) {
			ctx := locale.NewContext(callerContext(t, userv1.UserService_Login_FullMethodName), "id")
			candidate := mocks.SQLStep{Op: "query", SQL: "FROM auth", Args: []driver.Value{"member@example.com"}, Columns: []string{"id", "email", "password", "status", "auth_version", "version", "must_change_password", "deleted_at"}}
			if kind != "unknown" {
				state := "ACTIVE"
				if kind == "pending" {
					state = "PENDING"
				}
				candidate.Rows = [][]driver.Value{{int64(42), "member@example.com", fixtures.LoginPasswordHash(), state, int64(3), int64(4), false, nil}}
			}
			mock := &mocks.SQL{Context: ctx}
			for n := 0; n < 3; n++ {
				mock.Steps = append(mock.Steps, candidate, mocks.SQLStep{Op: "begin"}, mocks.SQLStep{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{nil, "rest-orc", "LOGIN_FAILED", "rpc", userv1.UserService_Login_FullMethodName, nil, nil, "CREDENTIAL_DENIED", nil, nil, now()}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(n + 1)}}}, mocks.SQLStep{Op: "commit"})
			}
			db := mock.Open()
			defer db.Close()
			producer, err := configs.PrepareLoginProducer(ctx, security, db, dialect.NewPostgres(), database.GetORMConfig(), now, bytes.NewReader(nil))
			if err != nil {
				t.Fatal(err)
			}
			encoder := grpcerror.NewEncoder(append(grpcmapping.CommonCodeMappings(), grpcmapping.AuthCodeMappings()...)...)
			info := &grpc.UnaryServerInfo{FullMethod: userv1.UserService_Login_FullMethodName}
			for n := 0; n < 4; n++ {
				var response *userv1.UserTokenResponse
				_, err := interceptors.NewErrorInterceptor(encoder)(ctx, nil, info, func(ctx context.Context, req any) (any, error) {
					return producer.SessionInterceptor(ctx, req, info, func(ctx context.Context, req any) (any, error) {
						return producer.RateInterceptor(ctx, &userv1.AuthRequest{Email: " MEMBER@example.com ", Password: "wrong123"}, info, func(ctx context.Context, req any) (any, error) {
							var err error
							response, err = producer.Handler.Login(ctx, req.(*userv1.AuthRequest))
							return response, err
						})
					})
				})
				decoded, ok := grpcerror.Decode(err)
				if response != nil || !ok {
					t.Fatal("denial returned credentials or lost details")
				}
				wantCode, wantKey := codes.Unauthenticated, auth.ErrLoginFailed.Key()
				wantMessage := auth.ErrLoginFailed.Message("id")
				if n == 3 {
					wantCode, wantKey = codes.ResourceExhausted, common.ErrRateLimited.Key()
					wantMessage = common.ErrRateLimited.Message("id")
					if decoded.RetryInfo == nil || decoded.RetryInfo.RetryDelay.AsDuration() != 12*time.Second {
						t.Fatal("retry guidance lost")
					}
				}
				if status.Code(err) != wantCode || decoded.Message != wantMessage {
					t.Fatal("denial code", err)
				}
				found := false
				for _, detail := range status.Convert(err).Details() {
					if value, ok := detail.(*errdetails.ErrorInfo); ok && value.Reason == wantKey {
						found = true
					}
				}
				if !found || producer.AuditFailures() != 0 {
					t.Fatal("generic denial/audit failed")
				}
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
