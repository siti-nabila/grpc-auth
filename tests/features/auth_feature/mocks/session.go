package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type SessionReader struct {
	Context   context.Context
	SessionID string
	State     domain.SessionState
	Error     error
	Calls     int
}

func (r *SessionReader) GetSessionState(ctx context.Context, id string) (domain.SessionState, error) {
	if ctx != r.Context || id != r.SessionID {
		panic("session request context/ID not forwarded")
	}
	r.Calls++
	return r.State, r.Error
}
