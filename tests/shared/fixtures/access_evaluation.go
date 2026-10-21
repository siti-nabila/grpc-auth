package fixtures

import (
	"database/sql/driver"

	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
)

func EvaluationApplicationStep(code, status string) mocks.SQLStep {
	id := int64(2)
	if code == "iam" {
		id = 1
	}
	return mocks.SQLStep{Op: "query", SQL: "FROM applications a", Args: []driver.Value{code}, Columns: []string{"id", "code", "name", "status", "is_system"}, Rows: [][]driver.Value{{id, code, "Application", status, code == "iam"}}}
}

func EvaluationAccessStep() mocks.SQLStep {
	return mocks.SQLStep{Op: "query", SQL: "FROM user_application_access access", Args: []driver.Value{int64(42), int64(2)}, Columns: []string{"user_id", "application_id", "application_code", "name", "application_status", "is_system", "status", "granted_by", "granted_at", "revoked_by", "revoked_at", "revoke_reason"}, Rows: [][]driver.Value{{int64(42), int64(2), "clm", "Application", "ACTIVE", false, "ACTIVE", nil, ServiceTime, nil, nil, nil}}}
}

func EvaluationSteps(application string) []mocks.SQLStep {
	steps := []mocks.SQLStep{{Op: "begin"}, PermissionSessionStep(), EvaluationApplicationStep(application, "ACTIVE")}
	if application != "iam" {
		steps = append(steps, EvaluationAccessStep())
	}
	rows := PermissionRowsStep(42, PermissionRows())
	rows.SQL = "rp.deleted_at IS NULL"
	return append(steps, rows, mocks.SQLStep{Op: "commit"})
}
