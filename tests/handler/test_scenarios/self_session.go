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
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"
)

func SelfSessionHandler() []testutils.Scenario {
	return []testutils.Scenario{{Name: "self session handler preserves context IDs safe results and errors", Run: selfSessionHandler}}
}
func selfSessionHandler(t *testing.T) {
	for _, mode := range []string{"single", "all"} {
		for _, kind := range []string{"success", "noop", "nil request", "nil result", "failure"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				cause := errors.New("controlled failure")
				calls := 0
				check := func(got context.Context) error {
					calls++
					if got != ctx {
						t.Fatal("context replaced")
					}
					if kind == "failure" {
						return cause
					}
					return nil
				}
				h, err := handler.NewSelfSessionHandler(mocks.SelfSession{Single: func(got context.Context, id string) (*session.RevokeResult, error) {
					if id != fixtures.SessionID {
						t.Fatal("target ID")
					}
					if e := check(got); e != nil {
						return nil, e
					}
					if kind == "nil result" {
						return nil, nil
					}
					return &session.RevokeResult{Changed: kind != "noop"}, nil
				}, All: func(got context.Context) (*session.RevokeAllResult, error) {
					if e := check(got); e != nil {
						return nil, e
					}
					if kind == "nil result" {
						return nil, nil
					}
					return &session.RevokeAllResult{Changed: kind != "noop", LoginRequired: kind != "noop"}, nil
				}})
				if err != nil {
					t.Fatal(err)
				}
				var changed, login bool
				if mode == "single" {
					req := &sessionv1.RevokeMySessionRequest{SessionId: fixtures.SessionID}
					if kind == "nil request" {
						req = nil
					}
					r, e := h.RevokeMySession(ctx, req)
					err = e
					if r != nil {
						changed = r.Changed
					}
				} else {
					req := &emptypb.Empty{}
					if kind == "nil request" {
						req = nil
					}
					r, e := h.RevokeAllMySessions(ctx, req)
					err = e
					if r != nil {
						changed = r.Changed
						login = r.LoginRequired
					}
				}
				wantCalls := 1
				if kind == "nil request" {
					wantCalls = 0
				}
				if calls != wantCalls {
					t.Fatal("call count", calls)
				}
				if kind == "success" || kind == "noop" {
					if err != nil || changed != (kind == "success") || login != (kind == "success" && mode == "all") {
						t.Fatal("response mapping", err)
					}
					return
				}
				want := error(common.ErrBadRequest)
				if kind == "nil result" {
					want = common.ErrInternalServerError
				}
				if kind == "failure" {
					want = cause
				}
				if !errors.Is(err, want) || changed || login {
					t.Fatal("error outcome", err)
				}
			})
		}
	}
	if _, err := handler.NewSelfSessionHandler(nil); err == nil {
		t.Fatal("nil dependency accepted")
	}
}
