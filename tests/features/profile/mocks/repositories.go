package mocks

import "github.com/siti-nabila/grpc-auth/internal/repositories/domain"

type Reader struct {
	Get func(uint64) (domain.Profile, error)
}

func (r Reader) GetByUserId(id uint64) (domain.Profile, error) { return r.Get(id) }

type Writer struct {
	Write func(*domain.UpdateProfileRequest) error
}

func (Writer) Create(*domain.ProfileRequest) error             { panic("unexpected Create") }
func (w Writer) Update(req *domain.UpdateProfileRequest) error { return w.Write(req) }
