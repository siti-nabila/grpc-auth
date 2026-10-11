package test_scenarios

import (
	"context"
	"database/sql"
	"errors"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/protobuf/encoding/protojson"
	"testing"
)

func ListSessionsHandler() []testutils.Scenario {
	return []testutils.Scenario{{Name: "session list handler maps supported query safe metadata null timestamps and errors", Run: listSessionsHandler}}
}
func listSessionsHandler(t *testing.T) {
	for _, mode := range []string{"self", "user"} {
		for _, kind := range []string{"success", "matching ID", "default", "empty", "nil request", "nil result", "failure", "fields", "search", "multi sort", "nil sort", "empty sort"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				cause := errors.New("controlled dependency")
				query := &paginatorv1.PageQuery{Page: 2, Limit: 2, LastId: "opaque-cursor", Sort: []*paginatorv1.Sort{{Field: "last_seen_at", Desc: false}}}
				switch kind {
				case "default":
					query = nil
				case "fields":
					query.Fields = []string{"password"}
				case "search":
					query.Search = &paginatorv1.Search{Keyword: "private"}
				case "matching ID":
					query.Sort = append(query.Sort, &paginatorv1.Sort{Field: "id"})
				case "multi sort":
					query.Sort = append(query.Sort, &paginatorv1.Sort{Field: "id", Desc: true})
				case "nil sort":
					query.Sort = []*paginatorv1.Sort{nil}
				case "empty sort":
					query.Sort = []*paginatorv1.Sort{{}}
				}
				call := func(got context.Context, q session.ListInput) (*session.ListResult, error) {
					calls++
					if got != ctx {
						t.Fatal("context replaced")
					}
					if kind == "default" {
						if q != (session.ListInput{}) {
							t.Fatal("default mapping")
						}
					} else if q.Page != 2 || q.Limit != 2 || q.Cursor != "opaque-cursor" || q.Field != domain.SessionSortLastSeenAt || q.Desc {
						t.Fatal("sort mapping")
					}
					if kind == "failure" {
						return nil, cause
					}
					if kind == "nil result" {
						return nil, nil
					}
					r := &session.ListResult{Page: 2, Limit: 2, Total: 3, HasPrev: true, NextCursor: "next", Items: []session.ListItem{{Session: domain.SessionSummary{ID: fixtures.SessionID, UserID: 42, Status: types.SessionActive, CreatedAt: fixtures.ServiceTime, ExpiresAt: fixtures.ServiceTime, DeviceLabel: sql.NullString{String: "browser", Valid: true}, UserAgent: sql.NullString{String: "agent", Valid: true}, IPAddress: sql.NullString{String: "127.0.0.1", Valid: true}}, Current: true}}}
					if kind == "empty" {
						r.Items = nil
						r.Total = 0
						r.NextCursor = ""
						r.Page = 1
						r.HasPrev = false
					}
					return r, nil
				}
				h, e := handler.NewListSessionsHandler(mocks.ListSessions{Self: call, User: func(got context.Context, id uint64, q session.ListInput) (*session.ListResult, error) {
					if id != 42 {
						t.Fatal("owner mapping")
					}
					return call(got, q)
				}})
				if e != nil {
					t.Fatal(e)
				}
				var res *sessionv1.ListSessionsResponse
				var err error
				if mode == "self" {
					req := &sessionv1.ListMySessionsRequest{Query: query}
					if kind == "nil request" {
						req = nil
					}
					res, err = h.ListMySessions(ctx, req)
				} else {
					req := &sessionv1.ListUserSessionsRequest{UserId: 42, Query: query}
					if kind == "nil request" {
						req = nil
					}
					res, err = h.ListUserSessions(ctx, req)
				}
				invalid := kind == "fields" || kind == "search" || kind == "multi sort" || kind == "nil sort" || kind == "empty sort" || kind == "nil request"
				want := 1
				if invalid {
					want = 0
				}
				if calls != want {
					t.Fatal("call count", calls, want)
				}
				if invalid || kind == "nil result" || kind == "failure" {
					if res != nil || err == nil {
						t.Fatal("failure outcome")
					}
					if kind == "failure" && !errors.Is(err, cause) {
						t.Fatal("cause lost")
					}
					return
				}
				if err != nil || res == nil || res.Meta.Limit != 2 {
					t.Fatal("page mapping", err)
				}
				if kind == "empty" {
					if len(res.Items) != 0 || res.Meta.Total != 0 || res.Meta.TotalPages != 0 || res.NextCursor != "" {
						t.Fatal("empty metadata")
					}
					return
				}
				if len(res.Items) != 1 || res.Meta.Page != 2 || res.Meta.Total != 3 || res.Meta.TotalPages != 2 || !res.Meta.HasPrev || res.Meta.HasNext || res.NextCursor != "next" {
					t.Fatal("page metadata")
				}
				s := res.Items[0]
				if s.Id != fixtures.SessionID || s.UserId != 42 || s.Status != "ACTIVE" || s.LastSeenAt != nil || s.RevokedAt != nil || !s.IsCurrent || s.DeviceLabel != "browser" || s.UserAgent != "agent" || s.IpAddress != "127.0.0.1" || !s.CreatedAt.AsTime().Equal(fixtures.ServiceTime) {
					t.Fatal("safe/null/current mapping")
				}
				data, e := protojson.Marshal(res)
				if e != nil || len(data) == 0 {
					t.Fatal("safe serialization", e)
				}
			})
		}
	}
	if _, err := handler.NewListSessionsHandler(nil); err == nil || errors.Is(err, common.ErrBadRequest) {
		t.Fatal("nil dependency accepted")
	}
}
