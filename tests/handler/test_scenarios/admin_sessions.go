package test_scenarios

import (
	"context"
	"errors"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
	"math"
	"testing"
)

func AdminSessionsHandler() []testutils.Scenario {
	return []testutils.Scenario{{Name: "admin handler requires exact idempotency metadata and global entity version presence", Run: adminSessionsHandler}}
}
func adminSessionsHandler(t *testing.T) {
	for _, mode := range []string{"single", "global"} {
		for _, kind := range []string{"success", "noop", "nil request", "missing key", "empty key", "duplicate key", "nil result", "failure", "missing version", "zero version", "overflow version"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", "test-key"))
				switch kind {
				case "missing key":
					ctx = context.Background()
				case "empty key":
					ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", ""))
				case "duplicate key":
					ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", "test-key", "idempotency-key", "test-key"))
				}
				calls := 0
				cause := errors.New("controlled failure")
				call := func(got context.Context) (*session.AdminResult, error) {
					calls++
					if got != ctx {
						t.Fatal("context replaced")
					}
					if kind == "failure" {
						return nil, cause
					}
					if kind == "nil result" {
						return nil, nil
					}
					return &session.AdminResult{Changed: kind != "noop", Version: 8}, nil
				}
				h, e := handler.NewAdminSessionsHandler(mocks.AdminSessions{Single: func(got context.Context, r session.AdminSingleInput) (*session.AdminResult, error) {
					if r.UserID != 43 || r.SessionID != fixtures.SessionID || r.Key != "test-key" {
						t.Fatal("single mapping")
					}
					return call(got)
				}, All: func(got context.Context, r session.AdminGlobalInput) (*session.AdminResult, error) {
					if r.UserID != 43 || r.ExpectedVersion != 7 || r.Key != "test-key" {
						t.Fatal("global mapping")
					}
					return call(got)
				}})
				if e != nil {
					t.Fatal(e)
				}
				var changed bool
				var version uint64
				var err error
				if mode == "single" {
					req := &sessionv1.RevokeUserSessionRequest{UserId: 43, SessionId: fixtures.SessionID}
					if kind == "nil request" {
						req = nil
					}
					r, e := h.RevokeUserSession(ctx, req)
					err = e
					if r != nil {
						changed = r.Changed
					}
				} else {
					v := uint64(7)
					req := &sessionv1.RevokeAllUserSessionsRequest{UserId: 43, ExpectedUserVersion: &v}
					switch kind {
					case "nil request":
						req = nil
					case "missing version":
						req.ExpectedUserVersion = nil
					case "zero version":
						v = 0
					case "overflow version":
						v = math.MaxInt64 + 1
					}
					r, e := h.RevokeAllUserSessions(ctx, req)
					err = e
					if r != nil {
						changed = r.Changed
						version = r.Version
					}
				}
				invalid := kind == "nil request" || kind == "missing key" || kind == "empty key" || kind == "duplicate key" || mode == "global" && (kind == "missing version" || kind == "zero version" || kind == "overflow version")
				wantCalls := 1
				if invalid {
					wantCalls = 0
				}
				if calls != wantCalls {
					t.Fatal("call count", calls, wantCalls)
				}
				if invalid || kind == "nil result" || kind == "failure" {
					want := error(common.ErrBadRequest)
					if kind == "nil result" {
						want = common.ErrInternalServerError
					}
					if kind == "failure" {
						want = cause
					}
					if !errors.Is(err, want) || changed || version != 0 {
						t.Fatal("failure outcome", err)
					}
					return
				}
				if err != nil || changed != (kind != "noop") || mode == "global" && version != 8 {
					t.Fatal("safe result mapping", changed, version, err)
				}
			})
		}
	}
	if _, e := handler.NewAdminSessionsHandler(nil); e == nil {
		t.Fatal("nil service accepted")
	}
}
