package test_scenarios

import (
	"context"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/configs"
	"google.golang.org/grpc"
)

func limiterWorkerLifecycle(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown", true: "listen failure"}[failure], func(t *testing.T) {
			cfg, _ := serverConfig(t)
			pipeline := serverPipeline(t, func(*grpc.Server) {})
			started, canceled, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			pipeline.Cleanup = func(ctx context.Context) { close(started); <-ctx.Done(); close(canceled); <-release; close(finished) }
			server, err := configs.NewGRPCServer(cfg, pipeline)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			defer server.Stop()
			defer close(release)
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("worker did not start")
			}
			stopped := make(chan error, 1)
			go func() {
				if failure {
					cfg.Host = "127.0.0.1"
					cfg.Port = -1
					stopped <- server.Start()
					return
				}
				server.GracefulStop()
				stopped <- nil
			}()
			select {
			case <-canceled:
			case <-ctx.Done():
				t.Fatal("worker was not canceled")
			}
			select {
			case <-stopped:
				t.Fatal("server returned before cleanup joined")
			default:
			}
			release <- struct{}{}
			select {
			case err := <-stopped:
				if failure && err == nil || !failure && err != nil {
					t.Fatal("shutdown result", err)
				}
			case <-ctx.Done():
				t.Fatal("worker did not join")
			}
			select {
			case <-finished:
			default:
				t.Fatal("worker still running")
			}
			server.GracefulStop()
			server.Stop()
		})
	}
}

func rejectIncompletePipeline(t *testing.T) {
	for _, field := range []string{"register", "attempt", "session", "rate", "cleanup", "clock"} {
		t.Run(field, func(t *testing.T) {
			cfg, _ := serverConfig(t)
			p := serverPipeline(t, func(*grpc.Server) { t.Fatal("incomplete pipeline registered") })
			switch field {
			case "register":
				p.Register = nil
			case "attempt":
				p.Attempt = nil
			case "session":
				p.Session = nil
			case "rate":
				p.Rate = nil
			case "cleanup":
				p.Cleanup = nil
			case "clock":
				p.Now = nil
			}
			if s, err := configs.NewGRPCServer(cfg, p); s != nil || err == nil {
				t.Fatal("incomplete pipeline accepted")
			}
		})
	}
}
