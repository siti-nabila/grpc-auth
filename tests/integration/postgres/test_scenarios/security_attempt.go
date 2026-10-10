//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"testing"
	"time"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
)

func SecurityAttemptPersistence() []testutils.Scenario {
	return []testutils.Scenario{{Name: "independent attempt commits only after rollback and never masks command failure", Run: postgresSecurityAttempt}}
}
func postgresSecurityAttempt(t *testing.T) {
	for _, kind := range []string{"recorded", "audit failure", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			ctx = foundationActor(t, ctx, fixture, profilev1.ProfileService_UpdateProfile_FullMethodName)
			requestCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if kind == "audit failure" {
				if _, err := fixture.DB.ExecContext(ctx, `CREATE FUNCTION reject_attempt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='SECURITY_ATTEMPT_FAILED' THEN RAISE EXCEPTION 'controlled attempt failure' USING ERRCODE='23514'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_attempt BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_attempt()`); err != nil {
					t.Fatal("attempt failure fixture failed")
				}
			}
			deps := foundationProfileDeps(fixture)
			cause := errors.New("controlled event dependency failure")
			deps.EventID = func() (string, error) {
				if kind == "canceled" {
					cancel()
					return "", requestCtx.Err()
				}
				return "", cause
			}
			service, err := profile.NewSessionProfileService(deps)
			if err != nil {
				t.Fatal(err)
			}
			observed, recorded := 0, 0
			attempt, err := interceptors.NewSecurityAttemptInterceptor(func(attemptCtx context.Context, req domain.AuditRequest) error {
				recorded++
				if attemptCtx.Err() != nil {
					t.Fatal("independent canceled attempt")
				}
				assertFoundationProfile(t, attemptCtx, fixture, "Fixture user", 1, 0, 0)
				return featurecommon.RecordSecurityAttempt(attemptCtx, productionProvider(fixture), req)
			}, func() { observed++ }, func() time.Time { return shared.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			res, err := attempt(requestCtx, nil, &grpc.UnaryServerInfo{FullMethod: profilev1.ProfileService_UpdateProfile_FullMethodName}, func(ctx context.Context, _ any) (any, error) {
				result, err := service.UpdateProfile(ctx, domain.ProfileUpdate{Name: "Tentative"})
				if err != nil {
					return nil, err
				}
				return result, nil
			})
			if res != nil || kind != "canceled" && (!errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable)) || kind == "canceled" && !errors.Is(err, context.Canceled) || recorded != 1 {
				t.Fatal("attempt changed original command outcome", err)
			}
			want := 1
			if kind == "audit failure" {
				want = 0
			}
			var attempts int
			var safe bool
			err = fixture.DB.QueryRowContext(ctx, `SELECT count(*),coalesce(bool_and(actor_user_id IS NULL AND actor_service='rest-orc' AND target_type='rpc' AND target_id='/profile.v1.ProfileService/UpdateProfile' AND safe_before IS NULL AND safe_after IS NULL AND reason='REQUEST_DENIED'),true) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED'`).Scan(&attempts, &safe)
			if err != nil || attempts != want || !safe || observed != 1-want {
				t.Fatal("safe independent attempt counts", attempts, observed, err)
			}
			assertFoundationProfile(t, ctx, fixture, "Fixture user", 1, 0, 0)
			if fixture.DB.Stats().InUse != 0 {
				t.Fatal("independent attempt leaked pool slot")
			}
		})
	}
}
