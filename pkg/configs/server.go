package configs

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type (
	GRPCServerClient struct {
		Server   *grpc.Server
		Config   *AppConfig
		Services map[string]*grpc.ClientConn
	}
)

func NewGRPCServer(cfg *AppConfig, register func(*grpc.Server)) *GRPCServerClient {
	// Implementation for creating a new gRPC server using the provided AppConfig{
	opts := []grpc.ServerOption{
		grpc.ConnectionTimeout(cfg.Timeout),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: cfg.KeepAliveIdle,
			Time:              cfg.KeepAlive,
			Timeout:           cfg.KeepAliveTimeout,
		}),
	}

	server := grpc.NewServer(opts...)
	if register != nil {
		register(server)
	}
	svr := &GRPCServerClient{
		Server:   server,
		Config:   cfg,
		Services: make(map[string]*grpc.ClientConn),
	}

	if len(cfg.Services) != 0 {
		for svcName, v := range cfg.Services {
			conn, err := dialService(v)
			if err != nil {
				fmt.Println("failed to connect to service: ", err)
			}
			svr.Services[svcName] = conn
		}
	}

	return svr

}

func dialService(serviceConfig ServiceConfig) (*grpc.ClientConn, error) {
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
		log.Println("err listen: ", err)
		return err
	}
	fmt.Printf("[gRPC] server listening on %s\n", addrs)

	// Jalankan server dalam goroutine agar tidak blocking
	go func() {
		if err := s.Server.Serve(lis); err != nil {
			fmt.Println("[gRPC] server stopped with error: ", err)
		}
	}()

	return nil
}

func (s *GRPCServerClient) GracefulStop() {
	fmt.Println("[gRPC] stopping server...")

	if len(s.Services) != 0 {
		for name, conn := range s.Services {
			fmt.Printf("[gRPC] closing client connection: %s\n", name)
			conn.Close()
		}
	}

	s.Server.GracefulStop()
	fmt.Println("[gRPC] server stopped gracefully")
}

func (s *GRPCServerClient) WaitForShutdown() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	fmt.Println("\n🛑 Received shutdown signal, exiting...")
}
