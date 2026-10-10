package mocks

import "github.com/siti-nabila/grpc-auth/internal/repositories/domain"

type AuthReader struct {
	Get func(string) (domain.Auth, error)
}

func (r AuthReader) GetByEmail(email string) (domain.Auth, error) { return r.Get(email) }
func (AuthReader) GetById(uint64) (domain.Auth, error)            { panic("unexpected GetById") }

type RoleReader struct{ Get func() (domain.Role, error) }

func (r RoleReader) GetDefaultRole() (domain.Role, error) { return r.Get() }

type AuthWriter struct {
	Write func(*domain.AuthRequest) error
}

func (w AuthWriter) Create(req *domain.AuthRequest) error { return w.Write(req) }

type ProfileWriter struct {
	Write func(*domain.ProfileRequest) error
}

func (w ProfileWriter) Create(req *domain.ProfileRequest) error { return w.Write(req) }
func (ProfileWriter) Update(*domain.UpdateProfileRequest) error { panic("unexpected profile Update") }

type UserRoleWriter struct {
	Write func(*domain.UserRoleRequest) error
}

func (w UserRoleWriter) Create(req *domain.UserRoleRequest) error { return w.Write(req) }
