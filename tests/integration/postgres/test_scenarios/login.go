//go:build postgres

package test_scenarios

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/configs"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func LoginProducer() []testutils.Scenario {
	return []testutils.Scenario{{Name: "production Login commits digest-only rows and feeds normal/restricted consumers", Run: postgresLogin}, {Name: "real required-write/sign/commit failures roll back initial issuance", Run: postgresLoginRollback}, {Name: "Login race rejects stale password or status on distinct physical backends", Run: postgresLoginProofRace}, {Name: "Login holds locks until terminal outcome and releases pool slots", Run: postgresLoginContention}}
}

func postgresLogin(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "restricted"}[restricted], func(t *testing.T) {
			ctx, fixture := loginFixture(t, restricted)
			now := fixtures.ServiceTime
			clock := func() time.Time { return now }
			security := loginSecurity(t, clock)
			producer, err := configs.PrepareLoginProducer(ctx, security, fixture.DB, dialect.NewPostgres(), database.GetORMConfig(), clock, loginEntropy(0))
			if err != nil {
				t.Fatal(err)
			}
			var dbName, serverVersion string
			var columns, constraints, indexes int
			if err := fixture.DB.QueryRowContext(ctx, "SELECT current_database(),current_setting('server_version')").Scan(&dbName, &serverVersion); err != nil {
				t.Fatal("safe dedicated test identity unavailable")
			}
			t.Logf("dedicated database=%s PostgreSQL=%s; owned schema=%s", dbName, serverVersion, fixture.Name)
			if err := fixture.DB.QueryRowContext(ctx, `SELECT
			 (SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name IN ('sessions','refresh_tokens')),
			 (SELECT count(*) FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=$1 AND r.relname IN ('sessions','refresh_tokens') AND c.convalidated),
			 (SELECT count(*) FROM pg_indexes WHERE schemaname=$1 AND tablename IN ('sessions','refresh_tokens'))`, fixture.Name).Scan(&columns, &constraints, &indexes); err != nil || columns != 22 || constraints != 15 || indexes != 7 {
				t.Fatal("actual producer schema column/constraint/index prerequisite mismatch")
			}
			var responses []*userv1.UserTokenResponse
			for n := 0; n < 2; n++ {
				res, err := producer.Handler.Login(ctx, &userv1.AuthRequest{Email: " FIXTURE@example.test ", Password: "secret12"})
				if err != nil || res == nil || res.MustChangePassword != restricted {
					t.Fatal("production Login failed")
				}
				responses = append(responses, res)
				assertLoginRows(t, ctx, fixture, n+1, 0)
				digest := sha256.Sum256([]byte(res.RefreshToken))
				var stored []byte
				var end, issued time.Time
				var initial, safe bool
				var userID, version, authVersion int
				err = fixture.DB.QueryRowContext(ctx, `SELECT r.token_hash,r.issued_at,s.expires_at,
				 r.consumed_at IS NULL AND r.revoked_at IS NULL AND r.replaced_by IS NULL,
				 s.user_id,a.version,a.auth_version,
				 l.actor_user_id=a.id AND l.actor_service IS NULL AND l.target_id=s.id::text AND l.safe_after->>'session_id'=s.id::text
				 FROM sessions s JOIN refresh_tokens r ON r.session_id=s.id JOIN auth a ON a.id=s.user_id
				 JOIN audit_logs l ON l.action='SESSION_CREATED' AND l.target_id=s.id::text
				 WHERE s.id=$1 AND r.expires_at=s.expires_at AND s.status='ACTIVE'`, res.SessionId).Scan(&stored, &issued, &end, &initial, &userID, &version, &authVersion, &safe)
				if err != nil || !bytes.Equal(stored, digest[:]) || !initial || !safe || userID != 1 || version != 1 || authVersion != 1 || !issued.Equal(now) || !end.Equal(now.Add(7*24*time.Hour)) || !res.SessionExpiresAt.AsTime().Equal(end) || !res.AccessExpiresAt.AsTime().Equal(now.Add(15*time.Minute)) {
					t.Fatal("committed initial state or published expiry mismatch")
				}
				claims, err := security.Verifier.Verify(res.Token)
				if err != nil || claims.UserID() != 1 || claims.SessionID() != res.SessionId || claims.AuthVersion() != 1 {
					t.Fatal("actual producer credential rejected by foundation")
				}
				provider := database.NewTransactionProvider(fixture.DB, dialect.NewPostgres(), database.GetORMConfig())
				for _, method := range []string{userv1.UserService_Me_FullMethodName, userv1.UserService_ListUsers_FullMethodName} {
					readCtx := loginCaller(t, ctx, method)
					md, _ := metadata.FromIncomingContext(readCtx)
					md = md.Copy()
					md.Set("authorization", "Bearer "+res.Token)
					readCtx = metadata.NewIncomingContext(readCtx, md)
					info := &grpc.UnaryServerInfo{FullMethod: method}
					result, err := producer.SessionInterceptor(readCtx, nil, info, func(ctx context.Context, req any) (any, error) {
						return producer.RateInterceptor(ctx, req, info, func(ctx context.Context, _ any) (any, error) {
							if method == userv1.UserService_Me_FullMethodName {
								return authfeature.ReadUserDataSnapshot(ctx, claims, provider, now)
							}
							return user.ReadUsersSnapshot(ctx, claims, domain.UserListRequest{Query: orm.QueryOptions{Page: 1, Limit: 10}}, provider, now)
						})
					})
					if method == userv1.UserService_Me_FullMethodName {
						data, ok := result.(*userv1.UserData)
						if err != nil || !ok || data.Id != 1 || data.MustChangePassword != restricted || restricted && (len(data.RoleCodes) != 0 || len(data.Roles) != 0 || len(data.ApplicationAccess) != 0) || !restricted && len(data.RoleCodes) != 1 {
							t.Fatal("actual producer Me snapshot failed")
						}
					} else if !errors.Is(err, common.ErrForbidden) {
						t.Fatal("actual producer ListUsers bypassed permission/restriction", err)
					}
				}
			}
			if responses[0].SessionId == responses[1].SessionId || responses[0].RefreshToken == responses[1].RefreshToken || responses[0].Token == responses[1].Token {
				t.Fatal("lost response was replayed")
			}
			var families int
			if err := fixture.DB.QueryRowContext(ctx, "SELECT count(DISTINCT family_id) FROM refresh_tokens").Scan(&families); err != nil || families != 2 {
				t.Fatal("repeated Login shared a refresh family")
			}
			// Prove database uniqueness and foreign keys independently of validation.
			for _, tc := range []struct{ query, code string }{
				{`INSERT INTO sessions SELECT * FROM sessions WHERE id=$1`, "23505"},
				{`INSERT INTO refresh_tokens(id,session_id,token_hash,family_id,issued_at,expires_at) SELECT 'ffffffff-ffff-4fff-8fff-ffffffffffff',session_id,token_hash,family_id,issued_at,expires_at FROM refresh_tokens WHERE session_id=$1`, "23505"},
				{`INSERT INTO refresh_tokens(id,session_id,token_hash,family_id,issued_at,expires_at) VALUES ('ffffffff-ffff-4fff-8fff-ffffffffffff','eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee',decode('aa','hex'),'dddddddd-dddd-4ddd-8ddd-dddddddddddd','2026-10-10','2026-10-11')`, "23503"},
			} {
				var err error
				if strings.Contains(tc.query, "$1") {
					_, err = fixture.DB.ExecContext(ctx, tc.query, responses[0].SessionId)
				} else {
					_, err = fixture.DB.ExecContext(ctx, tc.query)
				}
				var failure *pq.Error
				if !errors.As(err, &failure) || string(failure.Code) != tc.code {
					t.Fatal("actual FK/uniqueness guard missing")
				}
			}
			assertLoginRows(t, ctx, fixture, 2, 0)
			// Exact absolute expiry uses primary state even while verifier skew applies.
			claims, err := security.Verifier.Verify(responses[0].Token)
			if err != nil {
				t.Fatal("initial committed credential unexpectedly invalid")
			}
			now = now.Add(7 * 24 * time.Hour)
			cfg := database.GetORMConfig()
			cfg.EnableDebug = false
			stateReader := reader.NewSessionStateReader(orm.NewSqlQueryAdapter(ctx, fixture.DB, dialect.NewPostgres(), cfg))
			if _, err := authfeature.AuthenticateSession(ctx, claims, stateReader, now); !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("absolute PostgreSQL session expiry extended")
			}
			_, err = security.Verifier.Verify(responses[0].Token)
			if !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("expired original access accepted")
			}
			if fixture.DB.Stats().InUse != 0 {
				t.Fatal("producer/read pool slot leak")
			}
		})
	}
}

func postgresLoginRollback(t *testing.T) {
	for _, stage := range []string{"sessions", "refresh_tokens", "audit_logs", "sign", "commit"} {
		t.Run(stage, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			deps := loginDependencies(t, ctx, fixture, 0)
			if stage == "sign" {
				deps.Issue = func(jwt.AccessIssue) (jwt.IssuedAccess, error) {
					return jwt.IssuedAccess{}, errors.New("controlled signer failure")
				}
			} else if stage == "commit" {
				if _, err := fixture.DB.ExecContext(ctx, `ALTER TABLE refresh_tokens ADD CONSTRAINT test_late_failure FOREIGN KEY(family_id) REFERENCES sessions(id) DEFERRABLE INITIALLY DEFERRED`); err != nil {
					t.Fatal("deferred failure setup failed")
				}
			} else {
				if _, err := fixture.DB.ExecContext(ctx, `CREATE FUNCTION reject_issuance() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled test write failure' USING ERRCODE='23514'; END $$; CREATE TRIGGER reject_issuance AFTER INSERT ON `+stage+` FOR EACH ROW EXECUTE FUNCTION reject_issuance()`); err != nil {
					t.Fatal("required-write failure setup failed")
				}
			}
			res, err := loginService(t, deps).Login(ctx, loginRequest())
			if res != nil || !errors.Is(err, common.ErrDependencyUnavailable) {
				t.Fatal("real issuance failure returned credentials or wrong error", err)
			}
			assertLoginRows(t, ctx, fixture, 0, 0)
			err = nil
			if stage == "commit" {
				_, err = fixture.DB.ExecContext(ctx, "ALTER TABLE refresh_tokens DROP CONSTRAINT test_late_failure")
			} else if stage != "sign" {
				_, err = fixture.DB.ExecContext(ctx, "DROP TRIGGER reject_issuance ON "+stage)
			}
			if err != nil {
				t.Fatal("fault cleanup failed")
			}
			res, err = loginService(t, loginDependencies(t, ctx, fixture, 80)).Login(ctx, loginRequest())
			if err != nil || res == nil {
				t.Fatal("real failure leaked lock/connection")
			}
			assertLoginRows(t, ctx, fixture, 1, 0)
		})
	}
}
