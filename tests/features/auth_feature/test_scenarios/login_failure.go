package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func FailedLoginAudit() []testutils.Scenario {
	return []testutils.Scenario{{Name: "failed Login audit owns an independent transaction and preserves errors", Run: failedLoginAudit}}
}

func failedLoginAudit(t *testing.T) {
	for _, kind := range []string{"success", "begin", "write", "commit"} {
		ctx := callerContext(t)
		actor, err := domain.ActorForVerifiedService("rest-orc")
		if err != nil {
			t.Fatal(err)
		}
		r := domain.AuditRequest{Actor: actor, Action: "LOGIN_FAILED", TargetType: "rpc", TargetID: "/user.v1.UserService/Login", Reason: "CREDENTIAL_DENIED", OccurredAt: fixtures.ServiceTime}
		cause := errors.New("controlled failure audit dependency")
		begin := mocks.SQLStep{Op: "begin"}
		write := mocks.SQLStep{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{nil, "rest-orc", "LOGIN_FAILED", "rpc", r.TargetID, nil, nil, "CREDENTIAL_DENIED", nil, nil, r.OccurredAt}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(1)}}}
		commit := mocks.SQLStep{Op: "commit"}
		steps := []mocks.SQLStep{begin, write, commit}
		switch kind {
		case "begin":
			begin.Error = cause
			steps = []mocks.SQLStep{begin}
		case "write":
			write.Error = cause
			steps = []mocks.SQLStep{begin, write, {Op: "rollback"}}
		case "commit":
			commit.Error = cause
			steps = []mocks.SQLStep{begin, write, commit}
		}
		mock := &mocks.SQL{Context: ctx, Steps: steps}
		db := mock.Open()
		err = authfeature.RecordFailedLogin(ctx, database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()), r)
		if kind == "success" && err != nil || kind != "success" && !errors.Is(err, cause) {
			t.Fatal("failure audit outcome", kind, err)
		}
		if err := mock.Verify(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := authfeature.RecordFailedLogin(ctx, nil, domain.AuditRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatal("detached audit on canceled request")
	}
}
