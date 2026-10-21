package test_scenarios

import (
	"context"
	"database/sql"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/configs"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/tests/configs/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"
	"time"
)

func TelemetryLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "registered server keeps RPC and cleanup live and joins blocked telemetry on stop", Run: telemetryLifecycle}}
}

func telemetryLifecycle(t *testing.T) {
	cfg, cert := serverConfig(t)
	calls := new(mocks.Calls)
	p := serverPipeline(t, func(s *grpc.Server) { userv1.RegisterUserServiceServer(s, &mocks.Users{Calls: calls}) })
	ticks, cleanupTicks := make(chan time.Time), make(chan struct{})
	entered, release := make(chan struct{}), make(chan struct{})
	cleanupAck, cleanupStopped := make(chan struct{}), make(chan struct{})
	r, err := telemetry.NewSampler(p.Metrics, telemetry.RuntimeDependencies{
		Pending: func(context.Context) (int64, *time.Time, error) { return 0, nil, nil }, Pool: func() sql.DBStats { return sql.DBStats{} }, Limiter: p.Limiter.Counts, Now: p.Now,
		Write: func(telemetry.Snapshot) error { close(entered); <-release; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Telemetry = func(ctx context.Context) { r.Run(ctx, ticks) }
	p.Cleanup = func(ctx context.Context) {
		defer close(cleanupStopped)
		for {
			select {
			case <-ctx.Done():
				return
			case <-cleanupTicks:
				p.Limiter.Cleanup()
				cleanupAck <- struct{}{}
			}
		}
	}
	svr, err := configs.NewGRPCServer(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	serveDone := make(chan struct{})
	go func() { _ = svr.Server.Serve(listener); close(serveDone) }()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		svr.Stop()
		listener.Close()
		<-serveDone
	}()
	conn := memoryClient(t, listener, credentials.NewTLS(clientTLS(cert, "auth.test")))
	ticks <- fixtures.ServiceTime
	<-entered
	if err = callRPC(conn, userv1.UserService_TesRPC_FullMethodName, &emptypb.Empty{}, &userv1.TestRPC{}, serviceauth.MetadataKey, fixtures.ServiceToken(1)); err != nil {
		t.Fatal("logger blocked RPC", err)
	}
	cleanupTicks <- struct{}{}
	<-cleanupAck
	stopped := make(chan struct{})
	go func() { svr.Stop(); close(stopped) }()
	<-cleanupStopped
	select {
	case <-stopped:
		t.Fatal("telemetry writer was abandoned")
	default:
	}
	close(release)
	released = true
	<-stopped
	svr.Stop()
}
