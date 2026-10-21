package test_scenarios

import (
	"context"
	"errors"
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
	"reflect"
	"testing"
	"time"
)

func AssignmentHandlers() []testutils.Scenario {
	return []testutils.Scenario{{Name: "C commands validate presence raw bounds key and forward canonical context", Run: assignmentCommandHandlers}, {Name: "historical list query projection provenance and failure mappings", Run: assignmentListHandler}}
}
func assignmentCommandHandlers(t *testing.T) {
	for _, command := range []string{"grant", "revoke"} {
		for _, kind := range []string{"success", "no-op", "nil", "missing version", "zero version", "overflow version", "target overflow", "role overflow", "empty roles", "21 roles", "missing key", "duplicate key", "blank key", "dependency", "nil result", "corrupt result"} {
			t.Run(command+"/"+kind, func(t *testing.T) {
				ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("idempotency-key", " exact key "))
				if kind == "missing key" {
					ctx = context.Background()
				}
				if kind == "duplicate key" {
					ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("idempotency-key", "first", "idempotency-key", "second"))
				}
				if kind == "blank key" {
					ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("idempotency-key", " "))
				}
				id := uint64(7)
				roles := []uint64{3, 1, 3}
				version := proto.Uint64(2)
				switch kind {
				case "missing version":
					version = nil
				case "zero version":
					version = proto.Uint64(0)
				case "overflow version":
					version = proto.Uint64(math.MaxInt64 + 1)
				case "target overflow":
					id = math.MaxInt32 + 1
				case "role overflow":
					roles = []uint64{math.MaxInt32 + 1}
				case "empty roles":
					roles = nil
				case "21 roles":
					roles = make([]uint64, 21)
					for i := range roles {
						roles[i] = 1
					}
				}
				calls := 0
				cause := errors.New("controlled downstream failure")
				run := func(got context.Context, in domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error) {
					calls++
					if got != ctx || in.UserID != 7 || in.ExpectedVersion != 2 || !reflect.DeepEqual(in.RoleIDs, []uint64{1, 3}) || in.Reason != "Review access" || in.Key != " exact key " {
						t.Fatal("input mapping", in)
					}
					if kind == "dependency" {
						return nil, cause
					}
					if kind == "nil result" {
						return nil, nil
					}
					r := &domain.SafeCommandResult{UserID: 7, Version: 3, Changed: true, ChangedRoleIDs: []uint64{1, 3}}
					if kind == "no-op" {
						r.Version = 2
						r.Changed = false
						r.ChangedRoleIDs = nil
					}
					if kind == "corrupt result" {
						r.ChangedRoleIDs = []uint64{3, 1}
					}
					return r, nil
				}
				h, e := handler.NewUserRoleAssignmentHandler(mocks.Assignment{Grant: run, Revoke: run})
				if e != nil {
					t.Fatal(e)
				}
				var outChanged bool
				if command == "grant" {
					r := &userv1.GrantUserRolesRequest{UserId: id, RoleIds: roles, Reason: " Review access ", ExpectedUserVersion: version}
					if kind == "nil" {
						r = nil
					}
					out, err := h.GrantUserRoles(ctx, r)
					e = err
					if out != nil {
						outChanged = out.Changed
						if out.UserId != 7 || out.Version < 2 || out.Changed != (len(out.ChangedRoleIds) > 0) {
							t.Fatal(out)
						}
					}
				} else {
					r := &userv1.RevokeUserRolesRequest{UserId: id, RoleIds: roles, Reason: " Review access ", ExpectedUserVersion: version}
					if kind == "nil" {
						r = nil
					}
					out, err := h.RevokeUserRoles(ctx, r)
					e = err
					if out != nil {
						outChanged = out.Changed
						if out.UserId != 7 || out.Version < 2 || out.Changed != (len(out.ChangedRoleIds) > 0) {
							t.Fatal(out)
						}
					}
				}
				switch kind {
				case "success", "no-op":
					if e != nil || calls != 1 || outChanged != (kind == "success") {
						t.Fatal(e, calls)
					}
				case "dependency":
					if !errors.Is(e, cause) || calls != 1 {
						t.Fatal(e, calls)
					}
				case "nil result", "corrupt result":
					if !errors.Is(e, common.ErrInternalServerError) || calls != 1 {
						t.Fatal(e, calls)
					}
				default:
					if !errors.Is(e, common.ErrBadRequest) || calls != 0 {
						t.Fatal("validation bypass", e, calls)
					}
				}
			})
		}
	}
}

func assignmentListHandler(t *testing.T) {
	for _, kind := range []string{"full", "nullable", "projection", "nil", "bad query", "dependency", "corrupt response"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "request")
			calls := 0
			cause := errors.New("controlled read failure")
			at := time.Unix(1735689600, 0).UTC()
			actor := uint64(42)
			access, reason := "REVOKED", "Review access"
			h, _ := handler.NewUserRoleAssignmentHandler(mocks.Assignment{List: func(got context.Context, q domain.CatalogQuery) (domain.UserRoleAssignmentPage, error) {
				calls++
				if got != ctx || q.TargetUserID != 7 || q.Page != 1 || q.Limit != 20 || !reflect.DeepEqual(q.Statuses, []string{"ACTIVE", "REVOKED"}) {
					t.Fatal("list mapping", q)
				}
				if kind == "dependency" {
					return domain.UserRoleAssignmentPage{}, cause
				}
				row := domain.UserRoleAssignmentItem{ID: 11, RoleID: 3, Code: "sales.editor", RoleCode: 8, Name: "Editor", Status: "REVOKED", RoleStatus: "DISABLED", ApplicationID: 2, ApplicationCode: "sales", ApplicationStatus: "DISABLED", IsSystem: true, ApplicationAccessStatus: &access, GrantedBy: &actor, GrantedAt: &at, RevokedBy: &actor, RevokedAt: &at, RevokeReason: &reason}
				if kind == "nullable" {
					row.GrantedAt = nil
					row.GrantedBy = nil
					row.RevokedBy = nil
					row.ApplicationAccessStatus = nil
					row.RevokeReason = nil
					row.RevokedAt = nil
				}
				out := domain.UserRoleAssignmentPage{UserID: 7, Version: 2, Page: orm.PageData[domain.UserRoleAssignmentItem]{Items: []domain.UserRoleAssignmentItem{row}, Total: 1, Page: 1, Limit: 20, TotalPages: 1}}
				if kind == "corrupt response" {
					out.Version = 0
				}
				return out, nil
			}})
			in := &userv1.ListUserRolesRequest{UserId: 7, Filter: &userv1.UserRoleAssignmentFilter{Statuses: []string{"REVOKED", "ACTIVE"}}}
			if kind == "projection" {
				in.Query = &paginatorv1.PageQuery{Fields: []string{"name"}}
			}
			if kind == "bad query" {
				in.Query = &paginatorv1.PageQuery{Fields: []string{"password"}}
			}
			if kind == "nil" {
				in = nil
			}
			out, e := h.ListUserRoles(ctx, in)
			switch kind {
			case "nil", "bad query":
				if calls != 0 || !errors.Is(e, common.ErrBadRequest) || out != nil {
					t.Fatal(e, calls)
				}
			case "dependency":
				if calls != 1 || !errors.Is(e, cause) || out != nil {
					t.Fatal(e, calls)
				}
			case "corrupt response":
				if calls != 1 || !errors.Is(e, common.ErrInternalServerError) {
					t.Fatal(e)
				}
			default:
				if e != nil || calls != 1 || out.UserId != 7 || out.Version != 2 || out.Meta.Total != 1 || len(out.Items) != 1 {
					t.Fatal(out, e)
				}
				r := out.Items[0]
				if r.Id != 11 || r.RoleId != 3 || r.Name != "Editor" {
					t.Fatal(r)
				}
				if kind == "projection" {
					if r.Code != "" || r.GrantedAt != nil || r.ApplicationAccessStatus != nil {
						t.Fatal("projection", r)
					}
				} else if kind == "nullable" {
					if r.GrantedAt != nil || r.GrantedBy != nil || r.RevokedAt != nil || r.RevokeReason != nil {
						t.Fatal("null fabricated", r)
					}
				} else if r.GrantedAt.AsTime() != at || r.GrantedBy == nil || *r.GrantedBy != 42 || r.RevokedAt == nil || r.RevokeReason == nil {
					t.Fatal("provenance", r)
				}
			}
		})
	}
}
