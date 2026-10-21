package mocks

import "github.com/siti-nabila/grpc-auth/internal/repositories/domain"

type RegistrationRole struct{ Read func() (uint64, error) }

func (r RegistrationRole) DefaultMember() (uint64, error) { return r.Read() }

type RegistrationWriter struct {
	Write func(*domain.PendingRegistration) error
}

func (w RegistrationWriter) Create(req *domain.PendingRegistration) error { return w.Write(req) }

type RegistrationMember struct{ Write func(uint64, uint64) error }

func (w RegistrationMember) Assign(userID, roleID uint64) error { return w.Write(userID, roleID) }
