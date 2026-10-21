package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
)

type AuditFunc func(context.Context, domain.AuditLogQuery) (orm.PageData[domain.AuditLogItem], error)

func (f AuditFunc) ListAuditLogs(ctx context.Context, q domain.AuditLogQuery) (orm.PageData[domain.AuditLogItem], error) {
	return f(ctx, q)
}
