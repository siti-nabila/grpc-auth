package test_scenarios

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "Me uses one primary snapshot and omits restricted privileges", Run: meReads}, {Name: "ListUsers checks actor and permission before count and data", Run: listReads}, {Name: "missing caller or handle never falls back to pool", Run: missingDependencies}}
}

func callerContext(t *testing.T, method string) context.Context {
	t.Helper()
	v, err := serviceauth.NewVerifier(fixtures.ServiceConfig(), func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, fixtures.ServiceToken(1), "user_id", "999"))
	ctx, err = v.Authenticate(ctx, method)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func sessionStep(restricted bool) mocks.SQLStep {
	s := fixtures.SessionState()
	return mocks.SQLStep{Op: "query", SQL: "JOIN auth a ON a.id = s.user_id", Args: []driver.Value{s.SessionID},
		Columns: []string{"user_id", "user_status", "auth_version", "user_version", "must_change_password", "deleted_at", "session_id", "session_user_id", "session_status", "created_at", "expires_at", "revoked_at", "session_auth_version"},
		Rows:    [][]driver.Value{{int64(s.UserID), s.UserStatus, s.AuthVersion, s.UserVersion, restricted, nil, s.SessionID, int64(s.SessionUserID), s.SessionStatus, s.CreatedAt, s.ExpiresAt, nil, int64(3)}}}
}

func permissionStep(allowed bool) mocks.SQLStep {
	s := mocks.SQLStep{Op: "query", SQL: "LEFT JOIN user_application_access", Args: []driver.Value{int64(42)}, Columns: []string{"permission_code", "permission_application_id", "role_id", "role_code", "role_application_id", "role_status", "assignment_status", "application_code", "application_status", "access_status"}}
	if allowed {
		s.Rows = [][]driver.Value{{"iam.user.read", int64(1), int64(2), "iam.reader", int64(1), "ACTIVE", "ACTIVE", "iam", "ACTIVE", nil}}
	}
	return s
}

func verify(t *testing.T, mock *mocks.SQL) {
	t.Helper()
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
	if mock.Options.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) || !mock.Options.ReadOnly {
		t.Fatal("read isolation changed")
	}
}

func meReads(t *testing.T) {
	for _, kind := range []string{"normal", "restricted", "missing session", "profile failure", "commit failure"} {
		t.Run(kind, func(t *testing.T) {
			ctx := callerContext(t, "/user.v1.UserService/Me")
			cause := errors.New("controlled read failure")
			state := sessionStep(kind == "restricted")
			identity := mocks.SQLStep{Op: "query", SQL: "SELECT id, email, status, version, must_change_password FROM auth", Args: []driver.Value{int64(42)}, Columns: []string{"id", "email", "status", "version", "must_change_password"}, Rows: [][]driver.Value{{int64(42), "member@example.test", "ACTIVE", int64(4), kind == "restricted"}}}
			profile := mocks.SQLStep{Op: "query", SQL: "FROM profile", Args: []driver.Value{int64(42)}, Columns: []string{"id", "user_id", "name", "address", "phone"}, Rows: [][]driver.Value{{int64(7), int64(42), "Member", "Address", "123"}}}
			roles := mocks.SQLStep{Op: "query", SQL: "ORDER BY r.id ASC", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE"}, Columns: []string{"id", "role_name", "role_code", "role_description"}, Rows: [][]driver.Value{{int64(1), "Member", int64(1), nil}}}
			steps := []mocks.SQLStep{{Op: "begin"}, state}
			var want error
			if kind == "missing session" {
				steps[1].Rows = nil
				want = auth.ErrInvalidSession
			} else {
				if kind == "profile failure" {
					profile.Error = cause
					want = cause
				}
				steps = append(steps, identity, profile)
				if want == nil && kind != "restricted" {
					steps = append(steps, roles)
				}
			}
			if want != nil {
				steps = append(steps, mocks.SQLStep{Op: "rollback"})
			} else {
				step := mocks.SQLStep{Op: "commit"}
				if kind == "commit failure" {
					step.Error = cause
					want = cause
				}
				steps = append(steps, step)
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			defer db.Close()
			reads, err := handler.NewSessionReadHandler(database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()), func() time.Time { return fixtures.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			principal, err := sessions.PrincipalForState(fixtures.AccessClaims(t), fixtures.SessionState(), fixtures.ServiceTime)
			if err != nil {
				t.Fatal(err)
			}
			ctx = principal.Context(ctx)
			mock.Context = ctx
			res, err := reads.Me(ctx, nil)
			if want != nil {
				if !errors.Is(err, want) || res != nil {
					t.Fatal("partial Me success or cause lost", res.String(), err)
				}
				if want == cause && !errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal("Me dependency not unavailable", err)
				}
				verifyReadErrorLocale(t, ctx, err)
			} else {
				if err != nil || res.Id != 42 || res.Email != "member@example.test" || res.Fullname != "Member" || res.Version != 4 || res.Status != "ACTIVE" {
					t.Fatal("Me response", res.String(), err)
				}
				if kind == "restricted" {
					if !res.MustChangePassword || len(res.RoleCodes) != 0 || len(res.Roles) != 0 || len(res.ApplicationAccess) != 0 {
						t.Fatal("restricted privileges exposed")
					}
				} else if len(res.RoleCodes) != 1 || res.RoleCodes[0] != 1 {
					t.Fatal("legacy roles lost")
				}
			}
			verify(t, mock)
		})
	}
}

func listReads(t *testing.T) {
	for _, kind := range []string{"allowed", "empty", "denied", "restricted", "missing session", "permission failure", "invalid field", "count failure", "data failure", "role failure", "application failure", "commit failure"} {
		t.Run(kind, func(t *testing.T) {
			ctx := callerContext(t, "/user.v1.UserService/ListUsers")
			cause := errors.New("controlled page failure")
			steps := []mocks.SQLStep{{Op: "begin"}, sessionStep(kind == "restricted")}
			var want error
			switch kind {
			case "missing session":
				steps[1].Rows = nil
				want = auth.ErrInvalidSession
			case "restricted":
				want = common.ErrForbidden
			}
			if want == nil {
				permission := permissionStep(kind != "denied")
				if kind == "permission failure" {
					permission.Error = cause
					want = cause
				}
				steps = append(steps, permission)
				if kind == "denied" {
					want = common.ErrForbidden
				}
			}
			req := &userv1.ListUsersRequest{Query: &paginatorv1.PageQuery{Page: 1, Limit: 10}}
			if kind == "invalid field" {
				req.Query.Fields = []string{"password"}
				want = common.ErrBadRequest
			}
			if want == nil {
				count := mocks.SQLStep{Op: "query", SQL: "COUNT(", Columns: []string{"total"}, Rows: [][]driver.Value{{int64(1)}}}
				if kind == "empty" {
					count.Rows[0][0] = int64(0)
				}
				if kind == "count failure" {
					count.Error = cause
					want = cause
				}
				steps = append(steps, count)
			}
			if want == nil {
				data := mocks.SQLStep{Op: "query", SQL: "ORDER BY a.id ASC", Columns: []string{"auth_id", "email", "name", "address", "phone"}, Rows: [][]driver.Value{{int64(80), "result@example.test", "Result", "", ""}}}
				if kind == "empty" {
					data.Rows = nil
				}
				if kind == "data failure" {
					data.Error = cause
					want = cause
				}
				steps = append(steps, data)
				if want == nil && kind != "empty" {
					role := mocks.SQLStep{Op: "query", SQL: "FROM user_role ur", Args: []driver.Value{int64(80), "ACTIVE", "ACTIVE"}, Columns: []string{"user_id", "role_id", "code", "name", "status", "role_status"}}
					if kind == "role failure" {
						role.Error = cause
						want = cause
					}
					steps = append(steps, role)
					if want == nil {
						app := mocks.SQLStep{Op: "query", SQL: "FROM user_application_access ua", Args: []driver.Value{int64(80), "ACTIVE", "ACTIVE"}, Columns: []string{"user_id", "application_id", "code", "name", "status", "application_status"}}
						if kind == "application failure" {
							app.Error = cause
							want = cause
						}
						steps = append(steps, app)
					}
				}
			}
			if want != nil {
				steps = append(steps, mocks.SQLStep{Op: "rollback"})
			} else {
				commit := mocks.SQLStep{Op: "commit"}
				if kind == "commit failure" {
					commit.Error = cause
					want = cause
				}
				steps = append(steps, commit)
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			defer db.Close()
			reads, err := handler.NewSessionReadHandler(database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()), func() time.Time { return fixtures.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			principal, err := sessions.PrincipalForState(fixtures.AccessClaims(t), fixtures.SessionState(), fixtures.ServiceTime)
			if err != nil {
				t.Fatal(err)
			}
			ctx = principal.Context(ctx)
			mock.Context = ctx
			res, err := reads.ListUsers(ctx, req)
			if want != nil {
				if !errors.Is(err, want) || len(res.GetItems()) != 0 {
					t.Fatal("partial page success or cause lost", res, err)
				}
				if want == cause && !errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal("ListUsers dependency not unavailable", err)
				}
				verifyReadErrorLocale(t, ctx, err)
			} else {
				if err != nil || res.Page != 1 || res.Limit != 10 || kind == "empty" && len(res.Items) != 0 || kind == "allowed" && (len(res.Items) != 1 || res.Items[0].Id != 80 || res.NextCursor != "80") {
					t.Fatal("authorized page", res, err)
				}
			}
			verify(t, mock)
		})
	}
}

func missingDependencies(t *testing.T) {
	if _, err := handler.NewSessionReadHandler(nil, func() time.Time { return fixtures.ServiceTime }); err == nil {
		t.Fatal("nil snapshot provider accepted")
	}
	mock := &mocks.SQL{}
	db := mock.Open()
	defer db.Close()
	provider := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
	if _, err := handler.NewSessionReadHandler(provider, nil); err == nil {
		t.Fatal("nil clock accepted")
	}
	reads, err := handler.NewSessionReadHandler(provider, func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	if res, err := reads.Me(context.Background(), nil); res != nil || !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("missing principal Me reached snapshot")
	}
	if res, err := reads.ListUsers(context.Background(), &userv1.ListUsersRequest{}); res != nil || !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("missing principal list reached snapshot")
	}
	principalCtx := fixtures.PrincipalContext(t, callerContext(t, userv1.UserService_ListUsers_FullMethodName))
	if res, err := reads.ListUsers(principalCtx, nil); res != nil || !errors.Is(err, common.ErrBadRequest) {
		t.Fatal("nil list request accepted")
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
	claims := fixtures.AccessClaims(t)
	if _, err := authfeature.ReadUserDataSnapshot(context.Background(), claims, nil, fixtures.ServiceTime); !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
		t.Fatal("Me unverified caller", err)
	}
	if _, err := user.ReadUsersSnapshot(context.Background(), claims, domain.UserListRequest{}, nil, fixtures.ServiceTime); !errors.Is(err, auth.ErrServiceAuthenticationFailed) {
		t.Fatal("ListUsers unverified caller", err)
	}
	ctx := callerContext(t, "/user.v1.UserService/Me")
	if _, err := authfeature.ReadUserDataSnapshot(ctx, claims, nil, fixtures.ServiceTime); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("Me nil provider", err)
	}
	if _, err := user.ReadUsersSnapshot(ctx, claims, domain.UserListRequest{}, nil, fixtures.ServiceTime); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("ListUsers nil provider", err)
	}
	if _, err := reader.NewUserReaderInSnapshot(ctx, nil).SearchUsers(orm.QueryOptions{}, domain.UserListFilter{}); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("nil handle fallback", err)
	}
}

func verifyReadErrorLocale(t *testing.T, ctx context.Context, err error) {
	t.Helper()
	decoded, ok := grpcerror.Decode(helpers.HandleErrorContext(locale.NewContext(ctx, "id"), err))
	if !ok {
		t.Fatal("read error not encoded")
	}
	if errors.Is(err, common.ErrDependencyUnavailable) && decoded.Message != common.ErrDependencyUnavailable.Message(locale.Indonesian) {
		t.Fatal("dependency locale or redaction changed")
	}
	if errors.Is(err, common.ErrForbidden) && decoded.Message != common.ErrForbidden.Message(locale.Indonesian) {
		t.Fatal("denial locale changed")
	}
}
