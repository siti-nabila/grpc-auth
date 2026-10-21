package test_scenarios

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/protobuf/encoding/protojson"
)

func AccessEvaluation() []testutils.Scenario {
	return []testutils.Scenario{{Name: "authorization handler validates construction and maps only safe current decisions", Run: evaluationHandler}}
}

func evaluationHandler(t *testing.T) {
	clock := func() time.Time { return shared.ServiceTime }
	if h, err := handler.NewAuthorizationHandler(nil, clock); err == nil || h != nil {
		t.Fatal("nil provider accepted")
	}
	for _, kind := range []string{"allowed", "denied", "nil request", "no principal", "invalid input", "dependency", "panic"} {
		t.Run(kind, func(t *testing.T) {
			ctx := shared.PrincipalContext(t, shared.PermissionCallerContext(t, authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName, "clm"))
			req := &authorizationv1.EvaluateAccessRequest{ApplicationCode: " clm ", PermissionCode: " clm.contract.read "}
			steps := shared.EvaluationSteps("clm")
			var want error
			switch kind {
			case "denied":
				steps[4].Rows = nil
			case "nil request":
				req = nil
				steps = nil
				want = common.ErrBadRequest
			case "no principal":
				ctx = context.Background()
				steps = nil
				want = auth.ErrInvalidSession
			case "invalid input":
				req.PermissionCode = "iam.user.read"
				steps = nil
				want = common.ErrBadRequest
			case "dependency":
				steps[2].Error = common.ErrDependencyUnavailable
				steps = append(steps[:3], mocks.SQLStep{Op: "rollback"})
				want = common.ErrDependencyUnavailable
			case "panic":
				steps = []mocks.SQLStep{{Op: "begin"}, {Op: "rollback"}}
				want = common.ErrInternalServerError
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			t.Cleanup(func() { _ = db.Close() })
			var provider domain.ReadSnapshotProvider = database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
			if kind == "panic" {
				provider = featuremocks.PermissionSnapshot{Provider: provider, PanicAt: 1}
			}
			if h, err := handler.NewAuthorizationHandler(provider, nil); err == nil || h != nil {
				t.Fatal("nil clock accepted")
			}
			calls := 0
			h, err := handler.NewAuthorizationHandler(provider, func() time.Time { calls++; return shared.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			res, err := h.EvaluateAccess(ctx, req)
			if want != nil {
				if !errors.Is(err, want) || res != nil {
					t.Fatal("partial response", res, err, want)
				}
			} else {
				if err != nil || res.UserId != 42 || res.Sid != shared.SessionState().SessionID || res.UserVersion != uint64(shared.SessionState().UserVersion) || !res.EvaluatedAt.AsTime().Equal(shared.ServiceTime) || res.Allowed != (kind == "allowed") {
					t.Fatal(res, err)
				}
				reason := authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_ALLOWED
				if kind == "denied" {
					reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING
				}
				if res.Reason != reason {
					t.Fatal(res)
				}
				b, e := protojson.Marshal(res)
				if e != nil {
					t.Fatal(e)
				}
				for _, field := range []string{"password", "token", "refresh", "permissions", "roles", "authVersion"} {
					if strings.Contains(string(b), field) {
						t.Fatal("unsafe response", string(b))
					}
				}
			}
			wantCalls := 1
			if kind == "nil request" || kind == "no principal" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatal("clock calls", calls, wantCalls)
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
			if db.Stats().InUse != 0 {
				t.Fatal("leaked lease")
			}
		})
	}
}
