package test_scenarios

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func RegisteredEffectivePermissions() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered E methods enforce current authority safe failures and independent audit", Run: registeredPermissionViews},
		{Name: "real Login JWT survives fresh permission reads after source revocation and response loss", Run: registeredPermissionLogin},
		{Name: "E reads consume shared quota including invalid inputs with RetryInfo and no per-rate audit", Run: registeredPermissionQuota},
	}
}

func registeredPermissionViews(t *testing.T) {
	for _, call := range effectivePermissionRPCs() {
		for _, kind := range []string{"normal", "business only", "inactive", "restricted", "denied", "dependency", "cancellation", "deadline", "audit failure", "namespace", "no IAM"} {
			admin := call.method == userv1.UserService_GetUserEffectivePermissions_FullMethodName
			if admin && kind == "business only" {
				continue
			}
			if !admin && (kind == "inactive" || kind == "denied" || kind == "audit failure" || kind == "no IAM") {
				continue
			}
			t.Run(call.method+"/"+kind, func(t *testing.T) {
				service := shared.ServiceConfig()
				service.Callers[0].Methods = append(service.Callers[0].Methods, call.method)
				filter := ""
				if kind == "business only" {
					service.Callers[0].Namespaces = []string{"clm"}
				}
				if kind == "namespace" {
					service.Callers[0].Namespaces = []string{"iam"}
					filter = "clm"
				}
				if kind == "no IAM" {
					service.Callers[0].Namespaces = []string{"clm"}
				}
				state := shared.PermissionSessionStep()
				if kind == "restricted" {
					state.Rows[0][4] = true
				}
				steps := []mocks.SQLStep{}
				if kind != "no IAM" {
					steps = append(steps, state)
				}
				want := codes.OK
				reason := ""
				target := uint64(42)
				if admin {
					target = 80
				}
				if kind == "restricted" || kind == "namespace" || kind == "no IAM" {
					want = codes.PermissionDenied
					reason = common.ErrForbidden.Key()
				} else {
					steps = append(steps, mocks.SQLStep{Op: "begin"}, state)
					if admin {
						authority := shared.PermissionRowsStep(42, shared.PermissionRows())
						if kind == "denied" || kind == "audit failure" {
							authority.Rows = nil
							want = codes.PermissionDenied
							reason = common.ErrForbidden.Key()
						}
						steps = append(steps, authority)
					}
					if want == codes.OK {
						status := "ACTIVE"
						if kind == "inactive" {
							status = "SUSPENDED"
						}
						targetStep := shared.PermissionTargetStep(target, status)
						if kind == "dependency" {
							targetStep.Error = errors.New("private database detail")
							want = codes.Unavailable
							reason = common.ErrDependencyUnavailable.Key()
						}
						if kind == "cancellation" {
							targetStep.Error = context.Canceled
							want = codes.Canceled
							reason = common.ErrRequestCanceled.Key()
						}
						if kind == "deadline" {
							targetStep.Error = context.DeadlineExceeded
							want = codes.DeadlineExceeded
							reason = common.ErrDeadlineExceeded.Key()
						}
						steps = append(steps, targetStep)
						if want == codes.OK {
							steps = append(steps, shared.PermissionRowsStep(target, shared.PermissionRows()))
						}
					}
					if want == codes.OK {
						steps = append(steps, mocks.SQLStep{Op: "commit"})
					} else {
						steps = append(steps, mocks.SQLStep{Op: "rollback"})
					}
				}
				if want != codes.OK {
					audit := registeredAttempt(call.method, map[bool]string{true: "COMMON_DEPENDENCY_UNAVAILABLE", false: "COMMON_FORBIDDEN"}[want == codes.Unavailable])
					if kind == "cancellation" {
						audit = registeredAttempt(call.method, "REQUEST_CANCELED")
					}
					if kind == "deadline" {
						audit = registeredAttempt(call.method, "DEADLINE_EXCEEDED")
					}
					if kind == "audit failure" {
						audit[1].Error = errors.New("private audit detail")
						audit[2].Op = "rollback"
					}
					steps = append(steps, audit...)
				}
				mock := &mocks.SQL{Steps: steps}
				db := mock.Open()
				t.Cleanup(func() { _ = db.Close() })
				conn, foundation := shared.FoundationServerWithService(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), service)
				client := userv1.NewUserServiceClient(conn)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				ctx = shared.RPCContext(ctx, shared.SignAccess(t, shared.AccessPayload()))
				var res *userv1.GetEffectivePermissionsResponse
				var err error
				if admin {
					res, err = client.GetUserEffectivePermissions(ctx, &userv1.GetUserEffectivePermissionsRequest{UserId: target, ApplicationCode: filter})
				} else {
					res, err = client.GetMyEffectivePermissions(ctx, &userv1.GetMyEffectivePermissionsRequest{ApplicationCode: filter})
				}
				if status.Code(err) != want {
					t.Fatal(res, err, want)
				}
				if want == codes.OK {
					expected := 3
					if kind == "business only" {
						expected = 2
					}
					if res.UserId != target || len(res.Permissions) != expected || res.Version != 7 || kind == "inactive" && res.UserStatus != "SUSPENDED" {
						t.Fatal(res)
					}
				} else {
					decoded, ok := grpcerror.Decode(err)
					if res != nil || !ok || decoded.Reason != reason {
						t.Fatal("unsafe error", res, err)
					}
					if want == codes.Unavailable && decoded.Message != common.ErrDependencyUnavailable.Message(locale.Indonesian) {
						t.Fatal("redaction/locale", err)
					}
				}
				if foundation.Login.AuditFailures() != map[bool]uint64{true: 1, false: 0}[kind == "audit failure"] {
					t.Fatal("audit failure count", foundation.Login.AuditFailures())
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
}

func registeredPermissionLogin(t *testing.T) {
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
		rows := shared.PermissionRows()
		if attempt == 1 {
			rows[1][6], rows[2][6] = "REVOKED", "REVOKED"
		}
		steps = append(steps, state, mocks.SQLStep{Op: "begin"}, state, shared.PermissionTargetStep(42, "ACTIVE"), shared.PermissionRowsStep(42, rows), mocks.SQLStep{Op: "commit"})
	}
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	t.Cleanup(func() { _ = db.Close() })
	conn, foundation := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(data), userv1.UserService_GetMyEffectivePermissions_FullMethodName)
	client := userv1.NewUserServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	login, err := client.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "member@example.com", Password: "secret12"})
	if err != nil {
		t.Fatal(err)
	}
	actor := shared.RPCContext(ctx, login.Token)
	first, err := client.GetMyEffectivePermissions(actor, &userv1.GetMyEffectivePermissionsRequest{})
	if err != nil || len(first.GetPermissions()) != 3 {
		t.Fatal(first, err)
	}
	second, err := client.GetMyEffectivePermissions(actor, &userv1.GetMyEffectivePermissionsRequest{})
	if err != nil || len(second.GetPermissions()) != 2 || len(second.Permissions[0].Roles) != 1 || second.Permissions[0].Roles[0].RoleId != 4 || first.Version != second.Version {
		t.Fatal("stale response replay", second, err)
	}
	if login.SessionId != credentials.SessionID || foundation.Login.AuditFailures() != 0 {
		t.Fatal("credential/audit changed")
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
	if db.Stats().InUse != 0 {
		t.Fatal("lease leak")
	}
}

func registeredPermissionQuota(t *testing.T) {
	methods := []string{userv1.UserService_GetMyEffectivePermissions_FullMethodName, userv1.UserService_GetUserEffectivePermissions_FullMethodName}
	steps := []mocks.SQLStep{}
	for i := 0; i < 20; i++ {
		method := methods[i%2]
		steps = append(steps, shared.PermissionSessionStep())
		steps = append(steps, registeredAttempt(method, "COMMON_BAD_REQUEST")...)
	}
	steps = append(steps, shared.PermissionSessionStep())
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	t.Cleanup(func() { _ = db.Close() })
	conn, foundation := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), methods...)
	client := userv1.NewUserServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = shared.RPCContext(ctx, shared.SignAccess(t, shared.AccessPayload()))
	for i := 0; i < 20; i++ {
		var err error
		if i%2 == 0 {
			_, err = client.GetMyEffectivePermissions(ctx, &userv1.GetMyEffectivePermissionsRequest{ApplicationCode: "*"})
		} else {
			_, err = client.GetUserEffectivePermissions(ctx, &userv1.GetUserEffectivePermissionsRequest{})
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
	}
	_, err := client.ListUsers(ctx, &userv1.ListUsersRequest{})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal("shared old/new quota", err)
	}
	retry := false
	for _, detail := range status.Convert(err).Details() {
		if info, ok := detail.(*errdetails.RetryInfo); ok && info.RetryDelay.AsDuration() > 0 {
			retry = true
		}
	}
	if !retry || foundation.Login.Limiter.Counts().Entries != 2 || foundation.Login.AuditFailures() != 0 {
		t.Fatal("bounded rate guidance/audit", err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}
