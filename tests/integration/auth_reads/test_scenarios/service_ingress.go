package test_scenarios

import (
	"bytes"
	"context"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"
	"time"
)

func ServiceIngress() []testutils.Scenario {
	return []testutils.Scenario{{Name: "registered all37 exhausted service calls have zero SQL audit and handler effects", Run: registeredServiceIngress}}
}

func registeredServiceIngress(t *testing.T) {
	mock := &mocks.SQL{}
	db := mock.Open()
	t.Cleanup(func() { db.Close() })
	names := telemetry.MethodNames()
	service := shared.ServiceConfig()
	service.Callers[0].Methods = names[:37]
	conn, f := shared.FoundationServerWithService(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), service)
	for range 100 {
		d, e := f.Login.Limiter.Admit(ratelimit.Key{Group: ratelimit.ServiceIngress, Value: "rest-orc"})
		if e != nil || !d.Allowed {
			t.Fatal("precondition", d, e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, method := range names[:37] {
		err := conn.Invoke(shared.RPCContext(ctx, ""), method, &emptypb.Empty{}, &emptypb.Empty{})
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatal("service gate not outside session/attempt", method, err)
		}
		v := f.Login.Metrics.Snapshot(shared.ServiceTime).RPC[telemetry.MethodIndex(method)]
		if v.Codes[codes.ResourceExhausted] != 1 {
			t.Fatal("outer observer not wired", method, v)
		}
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
	if f.Login.Limiter.Counts().Entries != 1 || f.Login.Limiter.Counts().ServiceQuotaRejected != 37 || f.Login.AuditFailures() != 0 {
		t.Fatal("early rejection state")
	}
}
