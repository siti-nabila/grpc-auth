package test_scenarios

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/audit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/features/audit/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
	"time"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "one current-authority primary snapshot preserves errors cleanup and historical selector", Run: reads}}
}
func reads(t *testing.T) {
	if _, err := audit.NewService(audit.Dependencies{}); err == nil {
		t.Fatal("incomplete deps")
	}
	for _, name := range []string{"success", "empty", "missing caller", "missing principal", "namespace", "invalid", "begin", "state", "permission dependency", "denied", "soft deleted", "restricted", "stale session", "audit", "panic", "commit", "cancel", "deadline", "nil repositories"} {
		t.Run(name, func(t *testing.T) {
			ctx := fixtures.PrincipalContext(t, fixtures.PermissionCallerContext(t, domain.AuditLogMethod, "iam"))
			id := uint64(7)
			q := domain.AuditLogQuery{ActorUserID: &id}
			if name == "missing caller" {
				ctx = fixtures.PrincipalContext(t, context.Background())
			}
			if name == "missing principal" {
				ctx = fixtures.PermissionCallerContext(t, domain.AuditLogMethod, "iam")
			}
			if name == "namespace" {
				ctx = fixtures.PrincipalContext(t, fixtures.PermissionCallerContext(t, domain.AuditLogMethod, "clm"))
			}
			s := mocks.New(ctx)
			s.Steps = []string{"begin", "bind", "state", "permissions", "audit", "commit"}
			cause := errors.New("synthetic dependency failure")
			var want error
			switch name {
			case "empty":
				s.Empty = true
			case "missing caller":
				s.Steps = nil
				want = auth.ErrServiceAuthenticationFailed
			case "missing principal":
				s.Steps = nil
				want = auth.ErrInvalidSession
			case "namespace":
				s.Steps = nil
				want = common.ErrForbidden
			case "invalid":
				s.Steps = nil
				q.Unsupported = true
				want = common.ErrBadRequest
			case "cancel":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				s.Context = ctx
				s.Steps = nil
				want = context.Canceled
			case "begin":
				s.Steps = []string{"begin"}
				s.Fail = "begin"
				s.Failure = cause
				want = common.ErrDependencyUnavailable
			case "state":
				s.Steps = []string{"begin", "bind", "state", "rollback"}
				s.Fail = "state"
				s.Failure = cause
				want = common.ErrDependencyUnavailable
			case "permission dependency":
				s.Steps = []string{"begin", "bind", "state", "permissions", "rollback"}
				s.Fail = "permissions"
				s.Failure = cause
				want = common.ErrDependencyUnavailable
			case "denied", "soft deleted":
				s.Steps = []string{"begin", "bind", "state", "permissions", "rollback"}
				s.Rows = nil
				want = common.ErrForbidden
			case "restricted":
				s.Steps = []string{"begin", "bind", "state", "rollback"}
				s.State.MustChangePassword = true
				want = common.ErrForbidden
			case "stale session":
				s.Steps = []string{"begin", "bind", "state", "rollback"}
				s.State.AuthVersion++
				want = auth.ErrInvalidSession
			case "audit", "deadline":
				s.Steps = []string{"begin", "bind", "state", "permissions", "audit", "rollback"}
				s.Fail = "audit"
				s.Failure = cause
				want = common.ErrDependencyUnavailable
				if name == "deadline" {
					s.Failure = context.DeadlineExceeded
					want = context.DeadlineExceeded
				}
			case "panic":
				s.Steps = []string{"begin", "bind", "state", "permissions", "audit", "rollback"}
				s.Panic = "audit"
				want = common.ErrInternalServerError
			case "commit":
				s.Steps = append(s.Steps, "rollback")
				s.Fail = "commit"
				s.Failure = cause
				want = common.ErrDependencyUnavailable
			case "nil repositories":
				s.Steps = []string{"begin", "bind", "rollback"}
				want = common.ErrDependencyUnavailable
			}
			service, err := audit.NewService(audit.Dependencies{Snapshots: s, Now: func() time.Time { return fixtures.ServiceTime }, Bind: func(tx domain.Transaction) audit.Repositories {
				if tx != s {
					t.Fatal("handle changed")
				}
				_ = s.Step(ctx, "bind")
				if name == "nil repositories" {
					return audit.Repositories{}
				}
				return audit.Repositories{State: s, Permissions: s, Audit: s}
			}})
			if err != nil {
				t.Fatal(err)
			}
			p, err := service.ListAuditLogs(ctx, q)
			if !errors.Is(err, want) || want != nil && p.Items != nil || want == nil && len(p.Items) != map[bool]int{true: 0, false: 1}[s.Empty] {
				t.Fatal(p, err, want)
			}
			if s.Failure == cause && !errors.Is(err, cause) {
				t.Fatal("cause lost")
			}
			s.Verify(t)
		})
	}
}
