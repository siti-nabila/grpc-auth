package test_scenarios

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	auditv1 "github.com/siti-nabila/api-contracts/pb/audit/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const auditMethod = auditv1.AuditService_ListAuditLogs_FullMethodName

func RegisteredAuditLogReads() []testutils.Scenario {
	return []testutils.Scenario{{Name: "registered audit reads enforce current authority safe history and independent errors", Run: registeredAudit}, {Name: "audit reads share old quota and never database audit rate rejections", Run: auditQuota}, {Name: "same incidental key retries use fresh current authority", Run: auditRetry}}
}
func auditRPCs() []lifecycleRPC {
	return []lifecycleRPC{{auditMethod, &auditv1.ListAuditLogsRequest{}, &auditv1.ListAuditLogsResponse{}}}
}
func registeredAudit(t *testing.T) {
	for _, kind := range []string{"success", "historical selector", "empty", "namespace", "missing JWT", "expired JWT", "restricted", "revoked session", "wrong binding", "wrong auth version", "stale interceptor", "restricted after interception", "missing permission", "invalid", "count dependency", "page dependency", "corrupt snapshot", "commit", "cancel", "deadline", "audit failure"} {
		t.Run(kind, func(t *testing.T) {
			cfg := shared.ServiceConfig()
			cfg.Callers[0].Methods = append(cfg.Callers[0].Methods, auditMethod)
			req := &auditv1.ListAuditLogsRequest{}
			steps := []mocks.SQLStep{shared.PermissionSessionStep()}
			work := shared.AuditReadSteps()
			code := codes.OK
			catalog, message := "", ""
			switch kind {
			case "historical selector":
				req.Filter = &auditv1.AuditLogFilter{ActorUserId: proto.Uint64(7)}
				work[3].Args = []driver.Value{int64(7)}
				work[4].Args = []driver.Value{int64(7)}
			case "empty":
				work[3] = shared.AuditCountStep(0)
				work[4] = shared.AuditPageStep()
			case "namespace":
				cfg.Callers[0].Namespaces = []string{"clm"}
				steps = nil
				work = nil
				code = codes.PermissionDenied
			case "missing JWT", "expired JWT":
				steps = nil
				work = nil
				code = codes.Unauthenticated
			case "restricted":
				steps[0].Rows[0][4] = true
				work = nil
				code = codes.PermissionDenied
			case "revoked session":
				steps[0].Rows = nil
				work = nil
				code = codes.Unauthenticated
			case "wrong binding":
				steps[0].Rows[0][7] = int64(999)
				work = nil
				code = codes.Unauthenticated
			case "wrong auth version":
				steps[0].Rows[0][2] = int64(4)
				work = nil
				code = codes.Unauthenticated
			case "stale interceptor":
				work[1].Rows = nil
				work = append(work[:2], mocks.SQLStep{Op: "rollback"})
				code = codes.Unauthenticated
			case "restricted after interception":
				work[1].Rows[0][4] = true
				work = append(work[:2], mocks.SQLStep{Op: "rollback"})
				code = codes.PermissionDenied
			case "missing permission":
				work[2].Rows = nil
				work = append(work[:3], mocks.SQLStep{Op: "rollback"})
				code = codes.PermissionDenied
			case "invalid":
				req.Query = &paginatorv1.PageQuery{Search: &paginatorv1.Search{}}
				work = nil
				code = codes.InvalidArgument
			case "count dependency", "audit failure":
				work[3].Error = errors.New("synthetic private failure")
				work = append(work[:4], mocks.SQLStep{Op: "rollback"})
				code = codes.Unavailable
			case "page dependency":
				work[4].Error = errors.New("synthetic private failure")
				work = append(work[:5], mocks.SQLStep{Op: "rollback"})
				code = codes.Unavailable
			case "corrupt snapshot":
				work[4].Rows[0][7] = `{"access_token":"synthetic"}`
				work = append(work[:5], mocks.SQLStep{Op: "rollback"})
				code = codes.Unavailable
			case "commit":
				work[5].Error = errors.New("synthetic commit failure")
				code = codes.Unavailable
			case "cancel", "deadline":
				work[3].Error = context.Canceled
				code = codes.Canceled
				if kind == "deadline" {
					work[3].Error = context.DeadlineExceeded
					code = codes.DeadlineExceeded
				}
				work = append(work[:4], mocks.SQLStep{Op: "rollback"})
			}
			steps = append(steps, work...)
			if code != codes.OK {
				reason := ""
				switch code {
				case codes.PermissionDenied:
					catalog = common.ErrForbidden.Key()
					message = common.ErrForbidden.Message(locale.Indonesian)
					reason = "COMMON_FORBIDDEN"
				case codes.Unauthenticated:
					catalog = auth.ErrInvalidSession.Key()
					message = auth.ErrInvalidSession.Message(locale.Indonesian)
					reason = "AUTH_INVALID_SESSION"
				case codes.InvalidArgument:
					catalog = common.ErrBadRequest.Key()
					message = common.ErrBadRequest.Message(locale.Indonesian)
					reason = "COMMON_BAD_REQUEST"
				case codes.Unavailable:
					catalog = common.ErrDependencyUnavailable.Key()
					message = common.ErrDependencyUnavailable.Message(locale.Indonesian)
					reason = "COMMON_DEPENDENCY_UNAVAILABLE"
				case codes.Canceled:
					catalog = common.ErrRequestCanceled.Key()
					message = common.ErrRequestCanceled.Message(locale.Indonesian)
					reason = "REQUEST_CANCELED"
				case codes.DeadlineExceeded:
					catalog = common.ErrDeadlineExceeded.Key()
					message = common.ErrDeadlineExceeded.Message(locale.Indonesian)
					reason = "DEADLINE_EXCEEDED"
				}
				attempt := registeredAttempt(auditMethod, reason)
				if kind == "audit failure" {
					attempt[1].Error = errors.New("synthetic audit failure")
					attempt[2].Op = "rollback"
				}
				steps = append(steps, attempt...)
			}
			mock := &mocks.SQL{Steps: steps}
			db := mock.Open()
			defer db.Close()
			conn, foundation := shared.FoundationServerWithService(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), cfg)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			payload := shared.AccessPayload()
			if kind == "expired JWT" {
				payload["exp"] = shared.ServiceTime.Add(-time.Minute).Unix()
			}
			token := shared.SignAccess(t, payload)
			if kind == "missing JWT" {
				token = ""
			}
			res, err := auditv1.NewAuditServiceClient(conn).ListAuditLogs(metadata.AppendToOutgoingContext(shared.RPCContext(ctx, token), "user_id", "999", "role", "platform_admin"), req)
			if code != codes.OK {
				registeredError(t, err, code, catalog, message)
				if res != nil {
					t.Fatal("partial result", res)
				}
			} else if err != nil || res.Meta.Page != 1 || res.Meta.Limit != 20 || kind != "empty" && (res.Meta.Total != 1 || len(res.Items) != 1 || res.Items[0].Id != 2147483648 || res.Items[0].ActorUserId == nil || *res.Items[0].ActorUserId != 7 || res.Items[0].SafeBefore != nil || res.Items[0].SafeAfter == nil) || kind == "empty" && (res.Meta.Total != 0 || res.Meta.TotalPages != 0 || len(res.Items) != 0) {
				t.Fatal(res, err)
			}
			wantFailures := uint64(0)
			if kind == "audit failure" {
				wantFailures = 1
			}
			if foundation.Login.AuditFailures() != wantFailures {
				t.Fatal("attempt failure observation")
			}
			if err = mock.Verify(); err != nil {
				t.Fatal(err)
			}
			if db.Stats().InUse != 0 {
				t.Fatal("lease leak")
			}
		})
	}
}
func auditQuota(t *testing.T) {
	old := userv1.UserService_GetMyEffectivePermissions_FullMethodName
	steps := []mocks.SQLStep{}
	for i := 0; i < 20; i++ {
		method := auditMethod
		if i%2 == 1 {
			method = old
		}
		steps = append(steps, shared.PermissionSessionStep())
		steps = append(steps, registeredAttempt(method, "COMMON_BAD_REQUEST")...)
	}
	steps = append(steps, shared.PermissionSessionStep())
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	defer db.Close()
	conn, foundation := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), auditMethod, old)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = shared.RPCContext(ctx, shared.SignAccess(t, shared.AccessPayload()))
	client := auditv1.NewAuditServiceClient(conn)
	for i := 0; i < 20; i++ {
		var err error
		if i%2 == 0 {
			_, err = client.ListAuditLogs(ctx, &auditv1.ListAuditLogsRequest{Query: &paginatorv1.PageQuery{Fields: []string{"id"}}})
		} else {
			_, err = userv1.NewUserServiceClient(conn).GetMyEffectivePermissions(ctx, &userv1.GetMyEffectivePermissionsRequest{ApplicationCode: "*"})
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
	}
	_, err := client.ListAuditLogs(ctx, &auditv1.ListAuditLogsRequest{})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal(err)
	}
	retry := false
	for _, d := range status.Convert(err).Details() {
		if r, ok := d.(*errdetails.RetryInfo); ok && r.RetryDelay.AsDuration() > 0 {
			retry = true
		}
	}
	if !retry || foundation.Login.Limiter.Counts().Entries != 2 || foundation.Login.AuditFailures() != 0 {
		t.Fatal("quota guidance/bound")
	}
	if err = mock.Verify(); err != nil {
		t.Fatal(err)
	}
}
func auditRetry(t *testing.T) {
	steps := append([]mocks.SQLStep{shared.PermissionSessionStep()}, shared.AuditReadSteps()...)
	second := shared.AuditReadSteps()
	second[2].Rows = nil
	steps = append(steps, shared.PermissionSessionStep())
	steps = append(steps, second[:3]...)
	steps = append(steps, mocks.SQLStep{Op: "rollback"})
	steps = append(steps, registeredAttempt(auditMethod, "COMMON_FORBIDDEN")...)
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	defer db.Close()
	conn, _ := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), auditMethod)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(shared.RPCContext(ctx, shared.SignAccess(t, shared.AccessPayload())), "idempotency-key", "same-key")
	client := auditv1.NewAuditServiceClient(conn)
	if _, err := client.ListAuditLogs(ctx, &auditv1.ListAuditLogsRequest{}); err != nil {
		t.Fatal(err)
	}
	if res, err := client.ListAuditLogs(ctx, &auditv1.ListAuditLogsRequest{}); res != nil || status.Code(err) != codes.PermissionDenied {
		t.Fatal(res, err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}
