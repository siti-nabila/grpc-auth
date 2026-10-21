package configs

import (
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
)

func observeRequiredAudit(writer domain.AuditWriter, metrics *telemetry.Store) domain.AuditWriter {
	return common.ObserveAudit(writer, func() { metrics.AuditFailure(telemetry.RequiredWrite) })
}
