package test_scenarios

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/emptypb"
)

func MeAccess() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered Me-only service returns scoped access and rejects stale session state", Run: meRegisteredAccess},
		{Name: "Me scopes access independently of permission mappings and legacy roles", Run: meScopedAccess},
		{Name: "Me accepts a valid empty union and never replays incidental idempotency metadata", Run: meFreshAccess},
		{Name: "Me concatenates every stored phone literally including restricted and legacy values", Run: mePhoneConcat},
		{Name: "Me read and finalization failures suppress partial response and preserve cause", Run: meAccessFailures},
		{Name: "Me panic and cancellation release the snapshot without returning privileges", Run: meAccessCleanup},
	}
}

func meRegisteredAccess(t *testing.T) {
	for _, kind := range []string{"scoped", "restricted", "expired", "revoked", "cross user", "stale version", "suspended"} {
		t.Run(kind, func(t *testing.T) {
			first := sessionStep(kind == "restricted")
			valid := kind == "scoped" || kind == "restricted"
			switch kind {
			case "expired":
				first.Rows[0][10] = fixtures.ServiceTime.Add(-time.Second)
			case "revoked":
				first.Rows[0][8] = "REVOKED"
				first.Rows[0][11] = fixtures.ServiceTime
			case "cross user":
				first.Rows[0][7] = int64(99)
			case "stale version":
				first.Rows[0][2] = int64(99)
			case "suspended":
				first.Rows[0][1] = "SUSPENDED"
			}
			steps := []mocks.SQLStep{first}
			if valid {
				view := meSteps(kind == "restricted", "+62", "812")
				if kind == "scoped" {
					view[7].Rows = fixtures.PermissionRows()
				}
				steps = append(steps, view...)
			} else {
				steps = append(steps, registeredAttempt(userv1.UserService_Me_FullMethodName, "AUTH_INVALID_SESSION")...)
			}
			mock := &mocks.SQL{Steps: steps}
			db := mock.Open()
			t.Cleanup(func() { _ = db.Close() })
			cfg := fixtures.ServiceConfig()
			cfg.Callers[0].Methods = []string{userv1.UserService_Me_FullMethodName}
			cfg.Callers[0].Namespaces = []string{"clm"}
			conn, _ := fixtures.FoundationServerWithService(t, db, func() time.Time { return fixtures.ServiceTime }, bytes.NewReader(make([]byte, 96)), cfg)
			ctx := fixtures.RPCContext(context.Background(), fixtures.SignAccess(t, fixtures.AccessPayload()))
			res, err := userv1.NewUserServiceClient(conn).Me(ctx, &emptypb.Empty{})
			if !valid {
				registeredError(t, err, codes.Unauthenticated, auth.ErrInvalidSession.Key(), auth.ErrInvalidSession.Message("id"))
			} else if err != nil || res.Id != 42 || res.Phone != "+62812" || res.CreatedAt == nil || res.MustChangePassword != (kind == "restricted") {
				t.Fatal("registered safe Me", res, err)
			} else if kind == "restricted" {
				if len(res.Permissions) != 0 || len(res.RoleCodes) != 0 || len(res.Roles) != 0 || len(res.ApplicationAccess) != 0 {
					t.Fatal("registered restricted privilege leak", res)
				}
			} else if !reflect.DeepEqual(res.Permissions, map[string]bool{"clm.contract.read": true, "clm.contract.review": true}) {
				t.Fatal("registered Me requires no IAM authority or other RPC", res)
			}
			if err := mock.Verify(); err != nil || db.Stats().InUse != 0 {
				t.Fatal("registered Me effects or resource leak", err)
			}
		})
	}
}

func meCallerContext(t *testing.T, namespaces ...string) context.Context {
	t.Helper()
	cfg := fixtures.ServiceConfig()
	cfg.Callers[0].Methods = []string{userv1.UserService_Me_FullMethodName}
	cfg.Callers[0].Namespaces = namespaces
	v, err := serviceauth.NewVerifier(cfg, func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, fixtures.ServiceToken(1), "user_id", "999", "permissions", "iam.user.read"))
	ctx, err = v.Authenticate(ctx, userv1.UserService_Me_FullMethodName)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func meExtraSteps() []mocks.SQLStep {
	return []mocks.SQLStep{
		{Op: "query", SQL: "ORDER BY r.id ASC", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE", "ACTIVE", "iam", "ACTIVE"}, Columns: []string{"role_id", "code", "name", "status", "role_status", "application_code"}},
		{Op: "query", SQL: "ORDER BY a.id ASC", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE"}, Columns: []string{"application_id", "code", "name", "status", "application_status"}},
		fixtures.PermissionRowsStep(42, nil),
	}
}

func meSteps(restricted bool, country driver.Value, phone string) []mocks.SQLStep {
	steps := []mocks.SQLStep{
		{Op: "begin"}, sessionStep(restricted),
		{Op: "query", SQL: "SELECT id, email, status, version, must_change_password, created_at FROM auth", Args: []driver.Value{int64(42)}, Columns: []string{"id", "email", "status", "version", "must_change_password", "created_at"}, Rows: [][]driver.Value{{int64(42), "member@example.test", "ACTIVE", int64(4), restricted, fixtures.ServiceTime.Add(-time.Hour)}}},
		{Op: "query", SQL: "FROM profile", Args: []driver.Value{int64(42)}, Columns: []string{"id", "user_id", "name", "address", "phone", "country_code"}, Rows: [][]driver.Value{{int64(7), int64(42), "Member", "Address", phone, country}}},
	}
	if !restricted {
		steps = append(steps, mocks.SQLStep{Op: "query", SQL: "ORDER BY r.id ASC", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE"}, Columns: []string{"id", "role_name", "role_code", "role_description"}, Rows: [][]driver.Value{{int64(1), "Member", int64(1), nil}}})
		steps = append(steps, meExtraSteps()...)
	}
	return append(steps, mocks.SQLStep{Op: "commit"})
}

func readMeCase(t *testing.T, ctx context.Context, steps []mocks.SQLStep, wrap func(domain.ReadSnapshotProvider) domain.ReadSnapshotProvider) (*userv1.UserData, error) {
	t.Helper()
	mock := &mocks.SQL{Context: ctx, Steps: steps}
	db := mock.Open()
	t.Cleanup(func() { _ = db.Close() })
	var provider domain.ReadSnapshotProvider = database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
	if wrap != nil {
		provider = wrap(provider)
	}
	res, err := authfeature.ReadUserDataSnapshot(ctx, fixtures.AccessClaims(t), provider, fixtures.ServiceTime)
	verify(t, mock)
	if db.Stats().InUse != 0 {
		t.Fatal("Me snapshot connection not released")
	}
	return res, err
}

func meScopedAccess(t *testing.T) {
	ctx := meCallerContext(t, "clm")
	steps := meSteps(false, "+62", "81234567890")
	steps[5].Rows = [][]driver.Value{
		{int64(2), "iam.reader", "Reader", "ACTIVE", "ACTIVE", "iam"},
		{int64(4), "clm.author", "Author", "ACTIVE", "ACTIVE", "clm"},
		{int64(6), "sales.reader", "Sales", "ACTIVE", "ACTIVE", "sales"},
	}
	steps[6].Rows = [][]driver.Value{
		{int64(2), "clm", "Claims", "ACTIVE", "ACTIVE"},
		{int64(3), "sales", "Sales", "ACTIVE", "ACTIVE"},
	}
	rows := fixtures.PermissionRows()
	for _, kind := range []string{"assignment", "role", "application", "access", "mismatch", "wildcard"} {
		row := []driver.Value{"clm.contract.delete", int64(2), int64(9), "clm.invalid", int64(2), "ACTIVE", "ACTIVE", "clm", "ACTIVE", "ACTIVE"}
		switch kind {
		case "assignment":
			row[6] = "REVOKED"
		case "role":
			row[5] = "DISABLED"
		case "application":
			row[8] = "DISABLED"
		case "access":
			row[9] = nil
		case "mismatch":
			row[4] = int64(3)
		case "wildcard":
			row[0] = "clm.contract.*"
		}
		rows = append(rows, row)
	}
	steps[7].Rows = rows
	res, err := readMeCase(t, ctx, steps, nil)
	if err != nil || res.Id != 42 || !res.CreatedAt.AsTime().Equal(fixtures.ServiceTime.Add(-time.Hour)) {
		t.Fatal("Me identity or creation time", res, err)
	}
	if !reflect.DeepEqual(res.Permissions, map[string]bool{"clm.contract.read": true, "clm.contract.review": true}) || len(res.Roles) != 1 || res.Roles[0].RoleId != 4 || len(res.ApplicationAccess) != 1 || res.ApplicationAccess[0].ApplicationId != 2 {
		t.Fatal("Me namespace or union", res)
	}
	if !reflect.DeepEqual(res.RoleCodes, []int32{1}) || !reflect.DeepEqual(res.RoleNames, []string{"Member"}) {
		t.Fatal("legacy numeric/name metadata changed", res)
	}
	// A role/application can remain visible with no role-permission mappings.
	steps[7].Rows = nil
	res, err = readMeCase(t, ctx, steps, nil)
	if err != nil || len(res.Permissions) != 0 || len(res.Roles) != 1 || len(res.ApplicationAccess) != 1 {
		t.Fatal("permissionless metadata was lost", res, err)
	}
}

func meFreshAccess(t *testing.T) {
	ctx := meCallerContext(t, "iam")
	md, _ := metadata.FromIncomingContext(ctx)
	md = md.Copy()
	md.Set("idempotency-key", "same-read-key")
	ctx = metadata.NewIncomingContext(ctx, md)
	for _, granted := range []bool{true, false} {
		steps := meSteps(false, nil, "123")
		if granted {
			steps[7] = fixtures.PermissionRowsStep(42, fixtures.PermissionRows())
		}
		res, err := readMeCase(t, ctx, steps, nil)
		if err != nil || res.Permissions["iam.user.read"] != granted || res.Version != 4 || len(res.Roles) != 0 || len(res.ApplicationAccess) != 0 {
			t.Fatal("Me replayed stale grant or changed safe state", res, err)
		}
	}
}

func mePhoneConcat(t *testing.T) {
	for _, c := range []struct {
		country     driver.Value
		phone, want string
	}{
		{"+62", "81234567890", "+6281234567890"},
		{"+62", "081234567890", "+62081234567890"},
		{"+62", "+6281234567890", "+62+6281234567890"},
		{nil, "+6281234567890", "+6281234567890"},
		{"", "+6281234567890", "+6281234567890"},
		{"+62", "", "+62"}, {nil, "", ""}, {"+62", " 812 ", "+62 812 "},
	} {
		for _, restricted := range []bool{false, true} {
			ctx := callerContext(t, userv1.UserService_Me_FullMethodName)
			res, err := readMeCase(t, ctx, meSteps(restricted, c.country, c.phone), nil)
			if err != nil || res.Phone != c.want || res.MustChangePassword != restricted || len(res.Permissions) != 0 || restricted && (len(res.RoleCodes) != 0 || len(res.RoleNames) != 0 || len(res.Roles) != 0 || len(res.ApplicationAccess) != 0) {
				t.Fatal("literal phone or restricted minimum", res, err, c)
			}
			if c.country == nil && res.CountryCode != "" || c.country != nil && res.CountryCode != c.country {
				t.Fatal("separate calling code changed", res.CountryCode)
			}
		}
	}
}

func meAccessFailures(t *testing.T) {
	for index, name := range []string{"begin", "session", "identity", "profile", "legacy roles", "scoped roles", "applications", "permissions", "commit"} {
		t.Run(name, func(t *testing.T) {
			ctx := callerContext(t, userv1.UserService_Me_FullMethodName)
			cause := errors.New("controlled Me failure")
			steps := meSteps(false, "+62", "812")[:index+1]
			steps[index].Error = cause
			if index > 0 && name != "commit" {
				steps = append(steps, mocks.SQLStep{Op: "rollback"})
			}
			res, err := readMeCase(t, ctx, steps, nil)
			if res != nil || !errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable) {
				t.Fatal("partial Me or dependency cause lost", res, err)
			}
			verifyReadErrorLocale(t, ctx, err)
		})
	}
	ctx := callerContext(t, userv1.UserService_Me_FullMethodName)
	cause, cleanup := errors.New("read failure"), errors.New("rollback failure")
	steps := meSteps(false, nil, "")[:4]
	steps[3].Error = cause
	steps = append(steps, mocks.SQLStep{Op: "rollback", Error: cleanup})
	res, err := readMeCase(t, ctx, steps, nil)
	if res != nil || !errors.Is(err, cause) || !errors.Is(err, cleanup) {
		t.Fatal("cleanup hid failed Me", res, err)
	}
}

func meAccessCleanup(t *testing.T) {
	for _, kind := range []string{"panic", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(callerContext(t, userv1.UserService_Me_FullMethodName))
			defer cancel()
			steps := append(meSteps(false, "+62", "812")[:7], mocks.SQLStep{Op: "rollback"})
			want := error(common.ErrInternalServerError)
			wrap := func(p domain.ReadSnapshotProvider) domain.ReadSnapshotProvider {
				return featuremocks.PermissionSnapshot{Provider: p, PanicAt: 7}
			}
			if kind == "cancel" {
				want = context.Canceled
				wrap = func(p domain.ReadSnapshotProvider) domain.ReadSnapshotProvider {
					return featuremocks.PermissionSnapshot{Provider: p, CancelAt: 7, Cancel: cancel}
				}
			}
			res, err := readMeCase(t, ctx, steps, wrap)
			if res != nil || !errors.Is(err, want) {
				t.Fatal("Me cleanup leaked response", res, err)
			}
		})
	}
	ctx, cancel := context.WithDeadline(callerContext(t, userv1.UserService_Me_FullMethodName), time.Unix(0, 0))
	defer cancel()
	res, err := authfeature.ReadUserDataSnapshot(ctx, fixtures.AccessClaims(t), nil, fixtures.ServiceTime)
	if res != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Me deadline changed category", res, err)
	}
}
