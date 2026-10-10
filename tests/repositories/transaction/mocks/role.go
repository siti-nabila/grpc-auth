package mocks

import "github.com/siti-nabila/grpc-auth/internal/repositories/domain"

// Every unrequested lifecycle/query call panics through the nil interface.
type RoleTransaction struct {
	domain.Transaction
	Request any
	Calls   int
	Error   error
}

func (t *RoleTransaction) Create(req any) error { t.Request = req; t.Calls++; return t.Error }
