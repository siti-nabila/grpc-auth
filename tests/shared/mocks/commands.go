package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type StateReader struct {
	Read func(context.Context, string) (domain.SessionState, error)
}

func (r StateReader) GetSessionState(ctx context.Context, id string) (domain.SessionState, error) {
	return r.Read(ctx, id)
}

type VersionWriter struct{ Write func(uint64, int64) error }

func (w VersionWriter) SetVersion(id uint64, version int64) error { return w.Write(id, version) }

type AuditWriter struct {
	Write func(domain.AuditRequest) error
}

func (w AuditWriter) Create(req domain.AuditRequest) error { return w.Write(req) }

type OutboxWriter struct {
	Write func(domain.EventEnvelope) error
}

func (w OutboxWriter) Create(req domain.EventEnvelope) error { return w.Write(req) }
