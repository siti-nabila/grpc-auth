package test_scenarios

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const evaluationMethod = authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName

func RegisteredAccessEvaluation() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered evaluator enforces identity namespace decision and audit outcomes", Run: registeredEvaluation},
		{Name: "original Login credentials perform fresh evaluation after response loss", Run: registeredEvaluationLogin},
		{Name: "evaluator shares old-read quota including invalid inputs without per-rate audit", Run: registeredEvaluationQuota},
	}
}

func evaluationRPCs() []lifecycleRPC {
	return []lifecycleRPC{{evaluationMethod, &authorizationv1.EvaluateAccessRequest{}, &authorizationv1.EvaluateAccessResponse{}}}
}

func registeredEvaluation(t *testing.T) {
	for _, kind := range []string{"allowed", "business only", "missing", "unknown app", "inactive app", "absent access", "revoked access", "restricted", "revoked", "missing JWT", "expired token", "expired session", "wrong binding", "wrong auth version", "revoked after interception", "restricted after interception", "namespace", "invalid", "dependency", "commit", "cancel", "deadline", "audit failure"} {
		t.Run(kind, func(t *testing.T) {
			service := shared.ServiceConfig()
			service.Callers[0].Methods = append(service.Callers[0].Methods, evaluationMethod)
			if kind == "business only" {
				service.Callers[0].Namespaces = []string{"clm"}
			}
			if kind == "namespace" {
				service.Callers[0].Namespaces = []string{"iam"}
			}
			req := &authorizationv1.EvaluateAccessRequest{ApplicationCode: "clm", PermissionCode: "clm.contract.read"}
			work := shared.EvaluationSteps("clm")
			state := shared.PermissionSessionStep()
			steps := []mocks.SQLStep{state}
			code := codes.OK
			reason := authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_ALLOWED
			catalog := ""
			message := ""
			audit := false
			switch kind {
			case "missing", "audit failure":
				work[4].Rows = nil
				reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING
				audit = true
			case "unknown app":
				req.ApplicationCode = "unknown"
				req.PermissionCode = "unknown.resource.read"
				service.Callers[0].Namespaces = append(service.Callers[0].Namespaces, "unknown")
				work[2] = shared.EvaluationApplicationStep("unknown", "ACTIVE")
				work[2].Rows = nil
				work = append(work[:3], mocks.SQLStep{Op: "commit"})
				reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING
				audit = true
			case "inactive app":
				work[2].Rows[0][3] = "DISABLED"
				work = append(work[:3], mocks.SQLStep{Op: "commit"})
				reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_INACTIVE
				audit = true
			case "absent access", "revoked access":
				if kind == "absent access" {
					work[3].Rows = nil
				} else {
					work[3].Rows[0][6], work[3].Rows[0][10], work[3].Rows[0][11] = "REVOKED", shared.ServiceTime, "Access removed"
				}
				work = append(work[:4], mocks.SQLStep{Op: "commit"})
				reason = authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_ACCESS_REQUIRED
				audit = true
			case "restricted":
				steps[0].Rows[0][4] = true
				work = nil
				code = codes.PermissionDenied
				catalog = common.ErrForbidden.Key()
				message = common.ErrForbidden.Message(locale.Indonesian)
			case "revoked":
				steps[0].Rows = nil
				work = nil
				code = codes.Unauthenticated
				catalog = auth.ErrInvalidSession.Key()
				message = auth.ErrInvalidSession.Message(locale.Indonesian)
			case "missing JWT":
				steps = nil
				work = nil
				code = codes.Unauthenticated
				catalog = auth.ErrInvalidSession.Key()
				message = auth.ErrInvalidSession.Message(locale.Indonesian)
			case "expired token":
				steps = nil
				work = nil
				code = codes.Unauthenticated
				catalog = auth.ErrInvalidSession.Key()
				message = auth.ErrInvalidSession.Message(locale.Indonesian)
			case "expired session", "wrong binding", "wrong auth version":
				if kind == "expired session" {
					steps[0].Rows[0][10] = shared.ServiceTime.Add(-time.Second)
				}
				if kind == "wrong binding" {
					steps[0].Rows[0][7] = int64(999)
				}
				if kind == "wrong auth version" {
					steps[0].Rows[0][2] = int64(4)
				}
				work = nil
				code = codes.Unauthenticated
				catalog = auth.ErrInvalidSession.Key()
				message = auth.ErrInvalidSession.Message(locale.Indonesian)
			case "revoked after interception":
				work[1].Rows = nil
				work = append(work[:2], mocks.SQLStep{Op: "rollback"})
				code = codes.Unauthenticated
				catalog = auth.ErrInvalidSession.Key()
				message = auth.ErrInvalidSession.Message(locale.Indonesian)
			case "restricted after interception":
				work[1].Rows[0][4] = true
				work = append(work[:2], mocks.SQLStep{Op: "rollback"})
				code = codes.PermissionDenied
				catalog = common.ErrForbidden.Key()
				message = common.ErrForbidden.Message(locale.Indonesian)
			case "namespace":
				work = nil
				code = codes.PermissionDenied
				catalog = common.ErrForbidden.Key()
				message = common.ErrForbidden.Message(locale.Indonesian)
			case "invalid":
				req.PermissionCode = "iam.user.read"
				work = nil
				code = codes.InvalidArgument
				catalog = common.ErrBadRequest.Key()
				message = common.ErrBadRequest.Message(locale.Indonesian)
			case "dependency":
				work[2].Error = errors.New("private database failure")
				work = append(work[:3], mocks.SQLStep{Op: "rollback"})
				code = codes.Unavailable
				catalog = common.ErrDependencyUnavailable.Key()
				message = common.ErrDependencyUnavailable.Message(locale.Indonesian)
			case "commit":
				work[5].Error = errors.New("private finalization failure")
				code = codes.Unavailable
				catalog = common.ErrDependencyUnavailable.Key()
				message = common.ErrDependencyUnavailable.Message(locale.Indonesian)
			case "cancel":
				work[2].Error = context.Canceled
				work = append(work[:3], mocks.SQLStep{Op: "rollback"})
				code = codes.Canceled
				catalog = common.ErrRequestCanceled.Key()
				message = common.ErrRequestCanceled.Message(locale.Indonesian)
			case "deadline":
				work[2].Error = context.DeadlineExceeded
				work = append(work[:3], mocks.SQLStep{Op: "rollback"})
				code = codes.DeadlineExceeded
				catalog = common.ErrDeadlineExceeded.Key()
				message = common.ErrDeadlineExceeded.Message(locale.Indonesian)
			}
			steps = append(steps, work...)
			if audit || code != codes.OK {
				auditReason := "REQUEST_DENIED"
				if code != codes.OK {
					auditReason = map[codes.Code]string{codes.PermissionDenied: "COMMON_FORBIDDEN", codes.Unauthenticated: "AUTH_INVALID_SESSION", codes.InvalidArgument: "COMMON_BAD_REQUEST", codes.Unavailable: "COMMON_DEPENDENCY_UNAVAILABLE", codes.Canceled: "REQUEST_CANCELED", codes.DeadlineExceeded: "DEADLINE_EXCEEDED"}[code]
				}
				attempt := registeredAttempt(evaluationMethod, auditReason)
				if kind == "audit failure" {
					attempt[1].Error = errors.New("private audit failure")
					attempt[2].Op = "rollback"
				}
				steps = append(steps, attempt...)
			}
			mock := &mocks.SQL{Steps: steps}
			db := mock.Open()
			t.Cleanup(func() { _ = db.Close() })
			conn, foundation := shared.FoundationServerWithService(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), service)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			payload := shared.AccessPayload()
			if kind == "expired token" {
				payload["exp"] = shared.ServiceTime.Add(-time.Minute).Unix()
			}
			token := shared.SignAccess(t, payload)
			if kind == "missing JWT" {
				token = ""
			}
			res, err := authorizationv1.NewAuthorizationServiceClient(conn).EvaluateAccess(shared.RPCContext(ctx, token), req)
			if code != codes.OK {
				registeredError(t, err, code, catalog, message)
				if res != nil {
					t.Fatal("partial response", res)
				}
			} else if err != nil || res.Reason != reason || res.Allowed != (reason == authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_ALLOWED) || res.UserId != 42 || res.Sid != shared.SessionState().SessionID || res.UserVersion != uint64(shared.SessionState().UserVersion) || !res.EvaluatedAt.AsTime().Equal(shared.ServiceTime) {
				t.Fatal(res, err, reason)
			}
			wantFailures := uint64(0)
			if kind == "audit failure" {
				wantFailures = 1
			}
			if foundation.Login.AuditFailures() != wantFailures {
				t.Fatal("audit failure observation", foundation.Login.AuditFailures())
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
			if db.Stats().InUse != 0 {
				t.Fatal("lease leak")
			}
		})
	}
}

func registeredEvaluationLogin(t *testing.T) {
	data := make([]byte, 96)
	for i := range data {
		data[i] = byte(i)
	}
	credentials, err := authfeature.GenerateInitialCredentials(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	state := shared.PermissionSessionStep()
	state.Args = []driver.Value{credentials.SessionID}
	state.Rows[0][6] = credentials.SessionID
	state.Rows[0][9] = shared.ServiceTime
	state.Rows[0][10] = shared.ServiceTime.Add(7 * 24 * time.Hour)
	steps := registeredLoginSteps(t, data, false)
	for attempt := 0; attempt < 2; attempt++ {
		work := shared.EvaluationSteps("clm")
		work[1] = state
		if attempt == 1 {
			work[4].Rows = nil
		}
		steps = append(steps, state)
		steps = append(steps, work...)
		if attempt == 1 {
			steps = append(steps, registeredAttempt(evaluationMethod, "REQUEST_DENIED")...)
		}
	}
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	t.Cleanup(func() { _ = db.Close() })
	conn, foundation := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(data), evaluationMethod)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	login, err := userv1.NewUserServiceClient(conn).Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "member@example.com", Password: "secret12"})
	if err != nil {
		t.Fatal(err)
	}
	actor := metadata.AppendToOutgoingContext(shared.RPCContext(ctx, login.Token), "idempotency-key", "same-key", "user_id", "999")
	client := authorizationv1.NewAuthorizationServiceClient(conn)
	req := &authorizationv1.EvaluateAccessRequest{ApplicationCode: "clm", PermissionCode: "clm.contract.read"}
	first, err := client.EvaluateAccess(actor, req)
	if err != nil || !first.Allowed {
		t.Fatal(first, err)
	}
	second, err := client.EvaluateAccess(actor, req)
	if err != nil || second.Allowed || second.Reason != authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING || second.Sid != login.SessionId || second.UserId != 42 {
		t.Fatal("stale decision replay or identity override", second, err)
	}
	if foundation.Login.AuditFailures() != 0 {
		t.Fatal("audit failure")
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
	if db.Stats().InUse != 0 {
		t.Fatal("pool leak")
	}
}

func registeredEvaluationQuota(t *testing.T) {
	old := userv1.UserService_GetMyEffectivePermissions_FullMethodName
	steps := []mocks.SQLStep{}
	for i := 0; i < 20; i++ {
		method := evaluationMethod
		if i%2 == 1 {
			method = old
		}
		steps = append(steps, shared.PermissionSessionStep())
		steps = append(steps, registeredAttempt(method, "COMMON_BAD_REQUEST")...)
	}
	steps = append(steps, shared.PermissionSessionStep())
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	t.Cleanup(func() { _ = db.Close() })
	conn, foundation := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), evaluationMethod, old)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = shared.RPCContext(ctx, shared.SignAccess(t, shared.AccessPayload()))
	client := authorizationv1.NewAuthorizationServiceClient(conn)
	for i := 0; i < 20; i++ {
		var err error
		if i%2 == 0 {
			_, err = client.EvaluateAccess(ctx, &authorizationv1.EvaluateAccessRequest{})
		} else {
			_, err = userv1.NewUserServiceClient(conn).GetMyEffectivePermissions(ctx, &userv1.GetMyEffectivePermissionsRequest{ApplicationCode: "*"})
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
	}
	_, err := client.EvaluateAccess(ctx, &authorizationv1.EvaluateAccessRequest{ApplicationCode: "clm", PermissionCode: "clm.contract.read"})
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
		t.Fatal("unbounded quota or missing guidance", err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}
