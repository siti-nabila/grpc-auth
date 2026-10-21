package fixtures

import (
	"database/sql/driver"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
)

var AuditColumns = []string{"id", "actor_user_id", "actor_service", "action", "target_type", "target_id", "safe_before", "safe_after", "reason", "request_id", "trace_id", "occurred_at"}

func AuditRow(id int64) []driver.Value {
	return []driver.Value{id, int64(7), nil, "NO_OP", "user", "7", nil, "{}", nil, nil, nil, ServiceTime}
}
func AuditCountStep(total int64) mocks.SQLStep {
	return mocks.SQLStep{Op: "query", SQL: "COUNT(*) AS total", Columns: []string{"total"}, Rows: [][]driver.Value{{total}}}
}
func AuditPageStep(rows ...[]driver.Value) mocks.SQLStep {
	return mocks.SQLStep{Op: "query", SQL: "ORDER BY occurred_at DESC, id DESC", Columns: AuditColumns, Rows: rows}
}
func AuditReadSteps() []mocks.SQLStep {
	permission := PermissionRows()[:1]
	permission[0][0] = "iam.audit.read"
	return []mocks.SQLStep{{Op: "begin"}, PermissionSessionStep(), PermissionRowsStep(42, permission), AuditCountStep(1), AuditPageStep(AuditRow(2147483648)), {Op: "commit"}}
}
