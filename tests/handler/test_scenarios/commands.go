package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"testing"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type commandContextKey struct{}

func SessionCommands() []testutils.Scenario {
	return []testutils.Scenario{{Name: "Register maps pending additive response without credentials", Run: registrationHandler}, {Name: "Profile preserves absent mask and selected empty fields", Run: sessionProfileHandler}}
}
func registrationHandler(t *testing.T) {
	if _, err := handler.NewPendingRegistrationHandler(nil); err == nil {
		t.Fatal("nil service accepted")
	}
	for _, kind := range []string{"success", "nil", "error", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), commandContextKey{}, "request")
			calls := 0
			cause := errors.New("controlled command failure")
			h, err := handler.NewPendingRegistrationHandler(mocks.PendingRegistration{Call: func(got context.Context, req domain.AuthRequest) (*authfeature.RegistrationResult, error) {
				calls++
				if got != ctx || req != (domain.AuthRequest{Email: " User@example.test ", Password: "secret12"}) {
					t.Fatal("request/context changed")
				}
				if kind == "error" {
					return nil, cause
				}
				if kind == "malformed" {
					return nil, nil
				}
				return &authfeature.RegistrationResult{ID: 42, Status: types.UserPending}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			req := &userv1.AuthRequest{Email: " User@example.test ", Password: "secret12"}
			if kind == "nil" {
				req = nil
			}
			res, err := h.Register(ctx, req)
			if kind == "success" {
				if err != nil || res.RegistrationId != 42 || res.RegistrationStatus != "PENDING" || res.Token != "" || res.RefreshToken != "" || res.SessionId != "" || res.AccessExpiresAt != nil || res.SessionExpiresAt != nil || res.MustChangePassword {
					t.Fatal("credentials escaped registration", res, err)
				}
			} else if res != nil || kind == "nil" && !errors.Is(err, common.ErrBadRequest) || kind == "error" && !errors.Is(err, cause) || kind == "malformed" && !errors.Is(err, common.ErrInternalServerError) {
				t.Fatal("failure lost", err)
			}
			if calls != map[bool]int{true: 0, false: 1}[kind == "nil"] {
				t.Fatal("feature count")
			}
		})
	}
}
func sessionProfileHandler(t *testing.T) {
	if _, err := handler.NewSessionProfileHandler(nil); err == nil {
		t.Fatal("nil service accepted")
	}
	for _, kind := range []string{"legacy", "partial", "empty mask", "nil", "error", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			calls := 0
			cause := errors.New("controlled profile failure")
			input := &profilev1.ProfileRequest{Name: "New", Address: "", Phone: ""}
			if kind == "partial" {
				input.FieldMask = &fieldmaskpb.FieldMask{Paths: []string{"address"}}
			}
			if kind == "empty mask" {
				input.FieldMask = &fieldmaskpb.FieldMask{}
			}
			want := domain.ProfileUpdate{Name: "New", MaskPresent: input.FieldMask != nil, Mask: append([]string(nil), input.GetFieldMask().GetPaths()...)}
			h, err := handler.NewSessionProfileHandler(mocks.SessionProfile{Call: func(got context.Context, req domain.ProfileUpdate) (*profile.UpdateResult, error) {
				calls++
				if got != ctx || !reflect.DeepEqual(req, want) {
					t.Fatal("presence lost", req)
				}
				if kind == "error" {
					return nil, cause
				}
				if kind == "malformed" {
					return nil, nil
				}
				return &profile.UpdateResult{Profile: domain.Profile{Id: 7, UserId: 42, Name: "Committed"}, Version: 5, Changed: true}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "nil" {
				input = nil
			}
			res, err := h.UpdateProfile(ctx, input)
			if kind == "nil" || kind == "error" || kind == "malformed" {
				if res != nil || kind == "nil" && !errors.Is(err, common.ErrBadRequest) || kind == "error" && !errors.Is(err, cause) || kind == "malformed" && !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("failure lost", err)
				}
			} else if err != nil || res.Profile.Id != 7 || res.Profile.UserId != 42 || res.Profile.Name != "Committed" || res.Version != 5 || !res.Changed {
				t.Fatal("committed response lost", res, err)
			}
			if calls != map[bool]int{true: 0, false: 1}[kind == "nil"] {
				t.Fatal("feature count")
			}
		})
	}
}
