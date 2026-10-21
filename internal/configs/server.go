package configs

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"

	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"

	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type (
	GRPCServerClient struct {
		Server   *grpc.Server
		Config   *AppConfig
		Services map[string]*grpc.ClientConn
		cancel   context.CancelFunc
		done     chan struct{}
		stop     sync.Once
	}
)

func NewGRPCServer(cfg *AppConfig, pipeline ServerPipeline) (*GRPCServerClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("server configuration is required")
	}
	if err := pipeline.validate(); err != nil {
		return nil, err
	}
	tlsConfig, err := serverTLS(cfg.TLS, pipeline.Now())
	if err != nil {
		return nil, err
	}
	tlsConfig.Time = pipeline.Now
	verifier, err := serviceauth.NewVerifier(cfg.ServiceAuth, pipeline.Now)
	if err != nil {
		return nil, err
	}
	var mappings []grpcerror.CodeMapping
	mappings = append(mapping.CommonCodeMappings(), mapping.AuthCodeMappings()...)
	mappings = append(mappings, mapping.UserCodeMappings()...)
	mappings = append(mappings, mapping.RoleCodeMappings()...)
	mappings = append(mappings, mapping.ApplicationCodeMappings()...)
	errorEncoder := grpcerror.NewEncoder(mappings...)
	observe, err := interceptors.NewMetricsInterceptor(pipeline.Metrics, pipeline.Now)
	if err != nil {
		return nil, err
	}
	admit, err := interceptors.NewServiceAdmission(pipeline.Limiter, errorEncoder)
	if err != nil {
		return nil, err
	}
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ConnectionTimeout(cfg.Timeout),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: cfg.KeepAliveIdle,
			Time:              cfg.KeepAlive,
			Timeout:           cfg.KeepAliveTimeout,
		}),

		grpc.ChainUnaryInterceptor(
			observe,
			interceptors.LanguageInterceptor,
			interceptors.NewErrorInterceptor(errorEncoder),
			interceptors.ServiceInterceptor(verifier),
			admit,
			pipeline.Attempt,
			pipeline.Session,
			pipeline.Rate,
		),
	}

	server := grpc.NewServer(opts...)
	pipeline.Register(server)
	svr := &GRPCServerClient{
		Server:   server,
		Config:   cfg,
		Services: make(map[string]*grpc.ClientConn),
	}

	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		conn, err := dialService(cfg.Services[name])
		if err != nil {
			svr.Stop()
			return nil, fmt.Errorf("cannot configure outbound gRPC service: %w", err)
		}
		svr.Services[name] = conn
	}

	ctx, cancel := context.WithCancel(context.Background())
	svr.cancel, svr.done = cancel, make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); pipeline.Cleanup(ctx) }()
	go func() { defer workers.Done(); pipeline.Telemetry(ctx) }()
	go func() { workers.Wait(); close(svr.done) }()
	return svr, nil

}

func dialService(serviceConfig ServiceConfig) (*grpc.ClientConn, error) {
	if serviceConfig.Host == "" || serviceConfig.Port < 1 || serviceConfig.Port > 65535 {
		return nil, fmt.Errorf("outbound gRPC host and valid port are required")
	}
	addrs := fmt.Sprintf("%s:%d", serviceConfig.Host, serviceConfig.Port)

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Timeout:             serviceConfig.KeepAliveTimeout,
			Time:                serviceConfig.KeepAlive,
			PermitWithoutStream: true,
		}),
	}
	conn, err := grpc.NewClient(addrs, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (s *GRPCServerClient) Start() error {
	addrs := fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)
	lis, err := net.Listen("tcp", addrs)
	if err != nil {
		s.Stop()
		log.Println("err listen: ", err)
		return err
	}
	fmt.Printf("[gRPC] server listening on %s\n", addrs)

	// Jalankan server dalam goroutine agar tidak blocking
	go func() {
		if err := s.Server.Serve(lis); err != nil {
			s.Stop()
		}
	}()

	return nil
}

func (s *GRPCServerClient) GracefulStop() {
	s.stop.Do(func() { s.Server.GracefulStop(); s.closeResources() })
}

func (s *GRPCServerClient) Stop() {
	s.stop.Do(func() { s.Server.Stop(); s.closeResources() })
}

func (s *GRPCServerClient) closeResources() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
	s.closeServices()
}

func (s *GRPCServerClient) closeServices() {
	names := make([]string, 0, len(s.Services))
	for name := range s.Services {
		names = append(names, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names {
		conn := s.Services[name]
		if conn != nil {
			_ = conn.Close()
		}
	}
}

func (s *GRPCServerClient) WaitForShutdown() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	<-stop
	fmt.Println("\n🛑 Received shutdown signal, exiting...")
}
