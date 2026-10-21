package common

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type finalizationObserverKey struct{}

// WithFinalizationObserver injects a bounded local callback, never request data.
func WithFinalizationObserver(ctx context.Context, observe func()) context.Context {
	return context.WithValue(ctx, finalizationObserverKey{}, observe)
}

func observeSafely(observe func()) {
	defer func() { _ = recover() }()
	if observe != nil {
		observe()
	}
}

func (o *TransactionOwner) observeFinalization() {
	observe, _ := o.ctx.Value(finalizationObserverKey{}).(func())
	observeSafely(observe)
}

type observedAudit struct {
	writer  domain.AuditWriter
	failure func()
}

// ObserveAudit retains the original contract and the exact error/panic outcome.
func ObserveAudit(writer domain.AuditWriter, failure func()) domain.AuditWriter {
	if writer == nil {
		return nil
	}
	return &observedAudit{writer: writer, failure: failure}
}

func (w *observedAudit) Create(req domain.AuditRequest) (err error) {
	finished := false
	defer func() {
		if !finished || err != nil {
			observeSafely(w.failure)
		}
	}()
	err = w.writer.Create(req)
	finished = true
	return err
}
