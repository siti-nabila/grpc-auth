package test_scenarios

import (
	"context"
	"errors"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"math"
	"slices"
	"testing"
	"time"
)

func ApplicationHandlers() []testutils.Scenario {
	return []testutils.Scenario{{Name: "D transport raw presence metadata bounds cloning optional status safe result and list projection", Run: applicationHandlers}}
}
func applicationHandlers(t *testing.T) {
	if _, e := handler.NewApplicationAccessHandler(nil); e == nil {
		t.Fatal("nil constructor")
	}
	for _, cmd := range []string{"grant", "revoke"} {
		cases := []string{"success", "no-op", "absent status", "dependency", "nil result", "corrupt result", "nil request", "missing version", "zero version", "overflow version", "target overflow", "invalid code", "invalid reason", "missing key", "duplicate key", "blank key"}
		if cmd == "grant" {
			cases = append(cases, "empty roles", "21 roles", "role overflow")
		}
		for _, kind := range cases {
			t.Run(cmd+"/"+kind, func(t *testing.T) {
				ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", "exact key"))
				switch kind {
				case "missing key":
					ctx = context.Background()
				case "duplicate key":
					ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("idempotency-key", "one", "idempotency-key", "two"))
				case "blank key":
					ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("idempotency-key", " "))
				}
				user := uint64(7)
				v := proto.Uint64(2)
				code, reason := " Sales ", " Review access "
				roles := []uint64{3, 1, 3}
				switch kind {
				case "missing version":
					v = nil
				case "zero version":
					v = proto.Uint64(0)
				case "overflow version":
					v = proto.Uint64(math.MaxInt64 + 1)
				case "target overflow":
					user = math.MaxInt32 + 1
				case "invalid code":
					code = "bad.app"
				case "invalid reason":
					reason = "x"
				case "empty roles":
					roles = nil
				case "21 roles":
					roles = make([]uint64, 21)
					for i := range roles {
						roles[i] = 1
					}
				case "role overflow":
					roles = []uint64{math.MaxInt32 + 1}
				}
				calls := 0
				cause := errors.New("private controlled cause")
				run := func(got context.Context, in domain.ApplicationAccessInput) (*domain.SafeCommandResult, error) {
					calls++
					if got != ctx || in.UserID != 7 || in.ApplicationCode != "sales" || in.Reason != "Review access" || in.Key != "exact key" || in.ExpectedVersion != 2 {
						t.Fatal("input", in)
					}
					if cmd == "grant" && !slices.Equal(in.RoleIDs, []uint64{1, 3}) || cmd == "revoke" && len(in.RoleIDs) != 0 {
						t.Fatal("roles", in)
					}
					if len(in.RoleIDs) > 0 {
						in.RoleIDs[0] = 99
					}
					if kind == "dependency" {
						return nil, cause
					}
					if kind == "nil result" {
						return nil, nil
					}
					res := &domain.SafeCommandResult{UserID: 7, ApplicationID: 2, Status: "ACTIVE", Version: 3, Changed: true}
					if kind == "no-op" {
						res.Version = 2
						res.Changed = false
					}
					if kind == "absent status" {
						res.Status = ""
						for i := uint64(1); i <= 25; i++ {
							res.ChangedRoleIDs = append(res.ChangedRoleIDs, i)
						}
					}
					if kind == "corrupt result" {
						res.ApplicationID = 0
					}
					return res, nil
				}
				h, _ := handler.NewApplicationAccessHandler(mocks.Application{Grant: run, Revoke: run})
				var e error
				var outStatus *string
				var outChanged bool
				if cmd == "grant" {
					req := &applicationv1.GrantApplicationAccessRequest{UserId: user, ApplicationCode: code, RoleIds: roles, Reason: reason, ExpectedUserVersion: v}
					if kind == "nil request" {
						req = nil
					}
					out, err := h.GrantApplicationAccess(ctx, req)
					e = err
					if out != nil {
						outStatus = out.AccessStatus
						outChanged = out.Changed
					}
				} else {
					req := &applicationv1.RevokeApplicationAccessRequest{UserId: user, ApplicationCode: code, Reason: reason, ExpectedUserVersion: v}
					if kind == "nil request" {
						req = nil
					}
					out, err := h.RevokeApplicationAccess(ctx, req)
					e = err
					if out != nil {
						outStatus = out.AccessStatus
						outChanged = out.Changed
					}
				}
				switch kind {
				case "success", "no-op", "absent status":
					if e != nil || calls != 1 || outChanged != (kind != "no-op") || (outStatus == nil) != (kind == "absent status") {
						t.Fatal("response", e, calls, outStatus)
					}
				case "dependency":
					if !errors.Is(e, cause) || calls != 1 {
						t.Fatal(e)
					}
				case "nil result", "corrupt result":
					if !errors.Is(e, common.ErrInternalServerError) || calls != 1 {
						t.Fatal(e)
					}
				default:
					if !errors.Is(e, common.ErrBadRequest) || calls != 0 {
						t.Fatal("validation", e, calls)
					}
				}
				if kind == "success" && roles[0] != 3 {
					t.Fatal("request mutated")
				}
			})
		}
	}
	for _, kind := range []string{"full", "nullable", "projection", "nil", "bad query", "dependency", "foreign result"} {
		t.Run("list/"+kind, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "request")
			calls := 0
			cause := errors.New("read failure")
			at := time.Unix(1000, 0).UTC()
			actor := uint64(42)
			reason := "Review access"
			h, _ := handler.NewApplicationAccessHandler(mocks.Application{List: func(got context.Context, q domain.CatalogQuery) (domain.ApplicationAccessPage, error) {
				calls++
				if got != ctx || q.TargetUserID != 7 || !slices.Equal(q.Statuses, []string{"ACTIVE", "REVOKED"}) {
					t.Fatal("query", q)
				}
				if kind == "dependency" {
					return domain.ApplicationAccessPage{}, cause
				}
				row := domain.ApplicationAccessItem{ApplicationID: 2, ApplicationCode: "sales", Name: "Sales", Status: "REVOKED", ApplicationStatus: "DISABLED", IsSystem: true, GrantedAt: &at, GrantedBy: &actor, RevokedAt: &at, RevokedBy: &actor, RevokeReason: &reason}
				if kind == "nullable" {
					row.GrantedBy = nil
					row.RevokedBy = nil
					row.RevokedAt = nil
					row.RevokeReason = nil
				}
				out := domain.ApplicationAccessPage{UserID: 7, Version: 2, Page: orm.PageData[domain.ApplicationAccessItem]{Items: []domain.ApplicationAccessItem{row}, Page: 1, Limit: 20, Total: 1, TotalPages: 1}}
				if kind == "foreign result" {
					out.UserID = 8
				}
				return out, nil
			}})
			req := &userv1.ListUserApplicationAccessRequest{UserId: 7, Filter: &userv1.UserApplicationAccessFilter{Statuses: []string{"REVOKED", "ACTIVE"}}}
			if kind == "projection" {
				req.Query = &paginatorv1.PageQuery{Fields: []string{"name"}}
			}
			if kind == "bad query" {
				req.Query = &paginatorv1.PageQuery{Fields: []string{"password"}}
			}
			if kind == "nil" {
				req = nil
			}
			out, e := h.ListUserApplicationAccess(ctx, req)
			switch kind {
			case "nil", "bad query":
				if calls != 0 || out != nil || !errors.Is(e, common.ErrBadRequest) {
					t.Fatal(e)
				}
			case "dependency":
				if calls != 1 || out != nil || !errors.Is(e, cause) {
					t.Fatal(e)
				}
			case "foreign result":
				if out != nil || !errors.Is(e, common.ErrInternalServerError) {
					t.Fatal(e)
				}
			default:
				if e != nil || calls != 1 || out.Version != 2 || out.Meta.Total != 1 || len(out.Items) != 1 || out.Items[0].ApplicationId != 2 {
					t.Fatal(out, e)
				}
				r := out.Items[0]
				if kind == "projection" && (r.Name != "Sales" || r.ApplicationCode != "" || r.GrantedAt != nil) {
					t.Fatal("projection", r)
				}
				if kind == "nullable" && (r.GrantedBy != nil || r.RevokedBy != nil || r.RevokedAt != nil || r.RevokeReason != nil) {
					t.Fatal("null", r)
				}
				if kind == "full" && (r.GrantedAt.AsTime() != at || r.RevokeReason == nil || *r.RevokeReason != reason) {
					t.Fatal("provenance", r)
				}
			}
		})
	}
}
