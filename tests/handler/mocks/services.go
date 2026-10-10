package mocks

import (
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type AuthService struct {
	Call func(string, domain.AuthRequest) (*string, error)
}

func (s AuthService) Register(req domain.AuthRequest) (*string, error) {
	return s.Call("register", req)
}
func (s AuthService) Login(req domain.AuthRequest) (*string, error) { return s.Call("login", req) }
func (AuthService) GetUserData() (userv1.UserData, error)           { panic("unexpected Me") }

type ProfileService struct {
	Call func(domain.UpdateProfileRequest) (domain.Profile, error)
}

func (s ProfileService) UpdateProfile(req domain.UpdateProfileRequest) (domain.Profile, error) {
	return s.Call(req)
}
