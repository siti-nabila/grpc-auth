package test_scenarios

import (
	"context"
	"errors"
	auditv1 "github.com/siti-nabila/api-contracts/pb/audit/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math"
	"testing"
)

func AuditLogReads() []testutils.Scenario {
	return []testutils.Scenario{{Name: "audit handler maps minimal presence UTC and typed safe snapshots", Run: auditHandler}}
}
func auditHandler(t *testing.T) {
	if _, err := handler.NewAuditHandler(nil); err == nil {
		t.Fatal("nil service")
	}
	for _, name := range []string{"success", "default", "nil request", "no principal", "zero actor", "fields", "sort", "search", "timestamp", "error"} {
		t.Run(name, func(t *testing.T) {
			ctx := fixtures.PrincipalContext(t, fixtures.PermissionCallerContext(t, domain.AuditLogMethod, "iam"))
			req := &auditv1.ListAuditLogsRequest{Filter: &auditv1.AuditLogFilter{ActorUserId: proto.Uint64(7), ActorService: " svc ", TargetType: "user", TargetId: "7", OccurredFrom: timestamppb.New(fixtures.ServiceTime)}}
			calls := 0
			var want error
			switch name {
			case "default":
				req = &auditv1.ListAuditLogsRequest{}
			case "nil request":
				req = nil
				want = common.ErrBadRequest
			case "no principal":
				ctx = context.Background()
				want = auth.ErrInvalidSession
			case "zero actor":
				req.Filter.ActorUserId = proto.Uint64(0)
				want = common.ErrBadRequest
			case "fields":
				req.Query = &paginatorv1.PageQuery{Fields: []string{"id"}}
				want = common.ErrBadRequest
			case "sort":
				req.Query = &paginatorv1.PageQuery{Sort: []*paginatorv1.Sort{{Field: "occurred_at", Desc: true}}}
				want = common.ErrBadRequest
			case "search":
				req.Query = &paginatorv1.PageQuery{Search: &paginatorv1.Search{}}
				want = common.ErrBadRequest
			case "timestamp":
				req.Filter.OccurredFrom = &timestamppb.Timestamp{Seconds: math.MaxInt64}
				want = common.ErrBadRequest
			case "error":
				want = common.ErrDependencyUnavailable
			}
			h, err := handler.NewAuditHandler(mocks.AuditFunc(func(c context.Context, q domain.AuditLogQuery) (orm.PageData[domain.AuditLogItem], error) {
				calls++
				if c != ctx || q.Page != 1 || q.Limit != 20 {
					t.Fatal("mapping/context", q)
				}
				if name != "default" && (q.ActorService != "svc" || q.ActorUserID == nil || *q.ActorUserID != 7) {
					t.Fatal(q)
				}
				return orm.PageData[domain.AuditLogItem]{Items: []domain.AuditLogItem{{ID: math.MaxInt64, ActorService: proto.String("service"), Action: "NO_OP", TargetType: "user", TargetID: "7", After: &domain.SafeSnapshot{CountryCode: proto.String("+62"), Name: proto.String(""), RoleIDs: []uint64{2}, PermissionCodes: []string{"iam.audit.read"}, NoOp: true}, OccurredAt: fixtures.ServiceTime}}, Page: 1, Limit: 20, Total: 1, TotalPages: 1}, want
			}))
			if err != nil {
				t.Fatal(err)
			}
			res, err := h.ListAuditLogs(ctx, req)
			if !errors.Is(err, want) {
				t.Fatal(err, want)
			}
			if want != nil {
				if res != nil {
					t.Fatal("partial response")
				}
			} else if res.Items[0].Id != math.MaxInt64 || res.Items[0].ActorUserId != nil || res.Items[0].SafeBefore != nil || res.Items[0].SafeAfter == nil || res.Items[0].SafeAfter.Name == nil || res.Items[0].SafeAfter.GetCountryCode() != "+62" || !res.Items[0].SafeAfter.NoOp || !res.Items[0].OccurredAt.AsTime().Equal(fixtures.ServiceTime) {
				t.Fatal(res)
			}
			expected := 0
			if want == nil || name == "error" {
				expected = 1
			}
			if calls != expected {
				t.Fatal("calls", calls, expected)
			}
		})
	}
}
