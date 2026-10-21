package mocks

import (
	"context"
	"errors"
	"sync/atomic"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Calls struct{ Count atomic.Int32 }

func (c *Calls) accept(ctx context.Context, protected bool) error {
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil || caller.ID() != "rest-orc" {
		return errors.New("verified service caller missing")
	}
	if protected {
		user, err := sessions.PrincipalFromContext(ctx)
		if err != nil || user.UserID() != 42 {
			return errors.New("verified session principal missing")
		}
	}
	c.Count.Add(1)
	return nil
}

type Users struct {
	userv1.UnimplementedUserServiceServer
	Calls *Calls
}
type Profiles struct {
	profilev1.UnimplementedProfileServiceServer
	Calls *Calls
}

func (s *Users) Register(ctx context.Context, _ *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	return &userv1.UserTokenResponse{}, s.Calls.accept(ctx, false)
}
func (s *Users) Login(ctx context.Context, _ *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	return &userv1.UserTokenResponse{}, s.Calls.accept(ctx, false)
}
func (s *Users) TesRPC(ctx context.Context, _ *emptypb.Empty) (*userv1.TestRPC, error) {
	return &userv1.TestRPC{}, s.Calls.accept(ctx, false)
}
func (s *Users) Me(ctx context.Context, _ *emptypb.Empty) (*userv1.UserData, error) {
	return &userv1.UserData{}, s.Calls.accept(ctx, true)
}
func (s *Users) ListUsers(ctx context.Context, _ *userv1.ListUsersRequest) (*userv1.ListUsersResponse, error) {
	return &userv1.ListUsersResponse{}, s.Calls.accept(ctx, true)
}
func (s *Profiles) UpdateProfile(ctx context.Context, _ *profilev1.ProfileRequest) (*profilev1.ProfileResponse, error) {
	return &profilev1.ProfileResponse{}, s.Calls.accept(ctx, true)
}

type State struct{}

func (State) GetSessionState(context.Context, string) (domain.SessionState, error) {
	return fixtures.SessionState(), nil
}
