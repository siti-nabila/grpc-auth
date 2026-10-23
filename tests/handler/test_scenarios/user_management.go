package test_scenarios

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func UserManagementHandlers() []testutils.Scenario {
	return []testutils.Scenario{{Name: "management handlers preserve target mask presence metadata context and safe response", Run: managementHandlers}}
}
func managementHandlers(t *testing.T) {
	for _, method := range []string{"create", "update", "status"} {
		for _, kind := range []string{"success", "nil request", "missing key", "multiple keys", "blank key", "oversized key", "missing version", "overflow version", "missing mask", "error", "nil result", "invalid result"} {
			if method == "create" && (kind == "missing version" || kind == "overflow version" || kind == "missing mask") {
				continue
			}
			if method == "status" && kind == "missing mask" {
				continue
			}
			t.Run(method+"/"+kind, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				md := metadata.Pairs("idempotency-key", "key", "user_id", "999")
				switch kind {
				case "missing key":
					md.Delete("idempotency-key")
				case "multiple keys":
					md.Append("idempotency-key", "other")
				case "blank key":
					md.Set("idempotency-key", " ")
				case "oversized key":
					md.Set("idempotency-key", strings.Repeat("x", 201))
				}
				ctx = metadata.NewIncomingContext(ctx, md)
				version := uint64(7)
				if kind == "overflow version" {
					version = math.MaxUint64
				}
				name, address, phone, status, countryCode := "Target user", "", "123", "ACTIVE", "+62"
				create := &userv1.CreateUserRequest{Name: name, Email: "target@example.test", Password: "synthetic-password", Status: &status, Address: address, Phone: phone, CountryCode: &countryCode}
				update := &userv1.UpdateUserRequest{UserId: 43, ExpectedUserVersion: &version, Name: &name, Address: &address, CountryCode: &countryCode, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "address", "country_code"}}}
				change := &userv1.ChangeUserStatusRequest{UserId: 43, ExpectedUserVersion: &version, Status: &status, Reason: "Reviewed"}
				if kind == "nil request" {
					create = nil
					update = nil
					change = nil
				}
				if kind == "missing version" {
					update.ExpectedUserVersion = nil
					change.ExpectedUserVersion = nil
				}
				if kind == "missing mask" {
					update.FieldMask = nil
				}
				detail := domain.UserDetail{ID: 43, Email: "target@example.test", Name: name, Address: address, Phone: phone, CountryCode: countryCode, Status: types.UserActive, Version: 8, CreatedAt: fixtures.ServiceTime, MustChangePassword: true}
				result := domain.UserDetailCommandResult(detail, true)
				cause := errors.New("controlled feature error")
				calls := 0
				respond := func(got context.Context) (*domain.SafeCommandResult, error) {
					calls++
					if got != ctx {
						t.Fatal("request context changed")
					}
					switch kind {
					case "error":
						return nil, cause
					case "nil result":
						return nil, nil
					case "invalid result":
						return &domain.SafeCommandResult{}, nil
					}
					return &result, nil
				}
				key := "key"
				if kind == "missing key" {
					key = ""
				}
				mock := mocks.UserManagement{
					Create: func(got context.Context, r domain.CreateUserInput) (*domain.SafeCommandResult, error) {
						if r.Name != name || r.Email != create.Email || r.Password != create.Password || r.Status == nil || *r.Status != types.UserActive || r.Key != key || r.CountryCode == nil || *r.CountryCode != countryCode {
							t.Fatal("create mapping")
						}
						return respond(got)
					},
					Update: func(got context.Context, r domain.UpdateUserInput) (*domain.SafeCommandResult, error) {
						if r.UserID != 43 || r.ExpectedVersion != 7 || r.Address == nil || *r.Address != "" || r.Phone != nil || r.CountryCode == nil || *r.CountryCode != countryCode || !reflect.DeepEqual(r.Fields, []string{"name", "address", "country_code"}) || r.Key != key {
							t.Fatal("update presence/target mapping")
						}
						return respond(got)
					},
					Status: func(got context.Context, r domain.ChangeUserStatusInput) (*domain.SafeCommandResult, error) {
						if r.UserID != 43 || r.ExpectedVersion != 7 || r.Status == nil || *r.Status != types.UserActive || r.Reason != "Reviewed" || r.Key != key {
							t.Fatal("status mapping")
						}
						return respond(got)
					},
				}
				h, e := handler.NewUserManagementHandler(mock)
				if e != nil {
					t.Fatal(e)
				}
				var response any
				switch method {
				case "create":
					response, e = h.CreateUser(ctx, create)
				case "update":
					response, e = h.UpdateUser(ctx, update)
				case "status":
					response, e = h.ChangeUserStatus(ctx, change)
				}
				valid := kind == "success" || method == "update" && kind == "missing key"
				if valid {
					if e != nil || calls != 1 {
						t.Fatal("valid mapping failed", e)
					}
					switch r := response.(type) {
					case *userv1.CreateUserResponse:
						b, _ := protojson.Marshal(r)
						if r.User.CountryCode != countryCode || r.User.Password != "" || strings.Contains(string(b), `"password":`) {
							t.Fatal("unsafe creation response")
						}
					case *userv1.UpdateUserResponse:
						if !r.Changed || r.User.CountryCode != countryCode || r.User.Version != 8 || r.User.Password != "" {
							t.Fatal("unsafe update response")
						}
					case *userv1.ChangeUserStatusResponse:
						if !r.Changed || r.UserId != 43 || r.Version != 8 {
							t.Fatal("status response")
						}
					}
					return
				}
				want := error(common.ErrBadRequest)
				wantCalls := 0
				switch kind {
				case "error":
					want = cause
					wantCalls = 1
				case "nil result", "invalid result":
					want = common.ErrInternalServerError
					wantCalls = 1
				}
				if !errors.Is(e, want) || calls != wantCalls {
					t.Fatal("wrong boundary error or feature calls", e, calls)
				}
			})
		}
	}
}
