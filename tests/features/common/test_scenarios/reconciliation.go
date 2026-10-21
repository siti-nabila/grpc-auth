package test_scenarios

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	dictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	feature "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/common/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
)

func CommitAcknowledgement() []testutils.Scenario {
	return []testutils.Scenario{{Name: "lost commit acknowledgement preserves cause and never retries or claims rollback", Run: commitAcknowledgement}}
}

func commitAcknowledgement(t *testing.T) {
	cause := errors.New("controlled commit acknowledgement loss")
	tx := &mocks.CommittedAcknowledgement{Failure: cause}
	owner := feature.NewTransactionOwner(context.Background(), tx)
	err := func() (err error) {
		defer owner.Finish(&err)
		return feature.DependencyError(owner.Commit())
	}()
	if !tx.Committed || !errors.Is(err, cause) || !errors.Is(err, dictionary.ErrDependencyUnavailable) {
		t.Fatal("unknown commit outcome or failure cause was lost")
	}
	if errors.Is(err, sql.ErrTxDone) || !slices.Equal(tx.Calls, []string{"commit", "rollback"}) {
		t.Fatal("cleanup hid failure or retried a committed mutation", tx.Calls)
	}
	if owner.Rollback() != nil || !errors.Is(owner.Commit(), sql.ErrTxDone) || len(tx.Calls) != 2 {
		t.Fatal("closed transaction was finalized again")
	}
}

func MutationEventCorrelation() []testutils.Scenario {
	return []testutils.Scenario{{Name: "all shared mutation producers preserve bounded correlation and argument owned actor", Run: mutationEventCorrelation}}
}

func mutationEventCorrelation(t *testing.T) {
	const trace = "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"
	for _, tc := range []struct {
		name, correlation, trace string
		md                       metadata.MD
	}{
		{name: "absent", correlation: fixtures.EventID},
		{name: "valid", correlation: "request-42", trace: trace, md: metadata.Pairs("x-request-id", "request-42", "traceparent", trace)},
		{name: "boundary", correlation: strings.Repeat("a", 100), md: metadata.Pairs("x-request-id", strings.Repeat("a", 100))},
		{name: "duplicate", correlation: fixtures.EventID, md: metadata.Pairs("x-request-id", "one", "x-request-id", "two", "traceparent", trace, "traceparent", trace)},
		{name: "oversize", correlation: fixtures.EventID, md: metadata.Pairs("x-request-id", strings.Repeat("a", 101), "traceparent", "invalid")},
		{name: "invalid UTF8", correlation: fixtures.EventID, md: metadata.Pairs("x-request-id", string([]byte{0xff}), "traceparent", "invalid")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := tc.md.Copy()
			if md == nil {
				md = metadata.MD{}
			}
			md.Set("authorization", "Bearer SENTINEL-ACCESS-CREDENTIAL")
			md.Set("x-service-token", "SENTINEL-SERVICE-CREDENTIAL")
			md.Set("x-user-id", "999")
			ctx := metadata.NewIncomingContext(context.Background(), md)
			actor := uint64(42)
			role := domain.RoleSummary{ID: 7, ApplicationID: 2, ApplicationCode: types.ApplicationCLM, Version: 3, Status: types.RoleActive}
			for _, event := range []domain.EventEnvelope{
				feature.UserEvent(ctx, fixtures.EventID, types.EventUserProfileUpdated, 1, 2, &actor, types.UserActive, fixtures.PersistenceTime),
				feature.RoleEvent(ctx, fixtures.EventID, types.EventRoleUpdated, role, actor, []string{"clm.read"}, fixtures.PersistenceTime),
				feature.SessionRevokedEvent(ctx, fixtures.EventID, 1, actor, fixtures.SessionID, types.AuditSelfSessionRevoked, fixtures.PersistenceTime),
			} {
				record, err := event.Record()
				if err != nil || record.PublishedAt != nil || event.CorrelationID != tc.correlation || event.CausationID != tc.correlation || event.Traceparent != tc.trace || event.ActorUserID == nil || *event.ActorUserID != actor {
					t.Fatal("producer changed safe metadata or actor", event.EventType, err)
				}
				if strings.Contains(record.Payload, "SENTINEL-") || strings.Contains(record.Payload, "authorization") || strings.Contains(record.Payload, "x-service-token") {
					t.Fatal("producer persisted authentication metadata")
				}
			}
		})
	}
}
