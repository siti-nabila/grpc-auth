package fixtures

import (
	"context"
	"crypto/tls"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func PermissionCallerContext(t *testing.T, method string, namespaces ...string) context.Context {
	t.Helper()
	cfg := ServiceConfig()
	cfg.Callers[0].Methods = append(cfg.Callers[0].Methods, method)
	if len(namespaces) > 0 {
		cfg.Callers[0].Namespaces = namespaces
	}
	v, err := serviceauth.NewVerifier(cfg, func() time.Time { return ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, ServiceToken(1), "user_id", "999"))
	ctx, err = v.Authenticate(ctx, method)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func PermissionSessionStep() mocks.SQLStep {
	s := SessionState()
	return mocks.SQLStep{Op: "query", SQL: "JOIN auth a ON a.id = s.user_id", Args: []driver.Value{s.SessionID},
		Columns: []string{"user_id", "user_status", "auth_version", "user_version", "must_change_password", "deleted_at", "session_id", "session_user_id", "session_status", "created_at", "expires_at", "revoked_at", "session_auth_version"},
		Rows:    [][]driver.Value{{int64(s.UserID), s.UserStatus, s.AuthVersion, s.UserVersion, false, nil, s.SessionID, int64(s.SessionUserID), s.SessionStatus, s.CreatedAt, s.ExpiresAt, nil, int64(3)}}}
}

func PermissionTargetStep(id uint64, status string) mocks.SQLStep {
	return mocks.SQLStep{Op: "query", SQL: "SELECT id, status, version, auth_version, deleted_at FROM auth", Args: []driver.Value{int64(id)},
		Columns: []string{"id", "status", "version", "auth_version", "deleted_at"}, Rows: [][]driver.Value{{int64(id), status, int64(7), int64(3), nil}}}
}

func PermissionRowsStep(id uint64, rows [][]driver.Value) mocks.SQLStep {
	return mocks.SQLStep{Op: "query", SQL: "LEFT JOIN user_application_access", Args: []driver.Value{int64(id)},
		Columns: []string{"permission_code", "permission_application_id", "role_id", "role_code", "role_application_id", "role_status", "assignment_status", "application_code", "application_status", "access_status"}, Rows: rows}
}

func PermissionRows() [][]driver.Value {
	return [][]driver.Value{
		{"iam.user.read", int64(1), int64(3), "iam.auditor", int64(1), "ACTIVE", "ACTIVE", "iam", "ACTIVE", nil},
		{"clm.contract.review", int64(2), int64(5), "clm.reviewer", int64(2), "ACTIVE", "ACTIVE", "clm", "ACTIVE", "ACTIVE"},
		{"clm.contract.read", int64(2), int64(5), "clm.reviewer", int64(2), "ACTIVE", "ACTIVE", "clm", "ACTIVE", "ACTIVE"},
		{"iam.user.read", int64(1), int64(2), "iam.reader", int64(1), "ACTIVE", "ACTIVE", "iam", "ACTIVE", nil},
		{"clm.contract.read", int64(2), int64(4), "clm.author", int64(2), "ACTIVE", "ACTIVE", "clm", "ACTIVE", "ACTIVE"},
		{"iam.user.read", int64(1), int64(2), "iam.reader", int64(1), "ACTIVE", "ACTIVE", "iam", "ACTIVE", nil},
	}
}
