package fixtures

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/configs"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// FoundationServer uses the production assembly and pipeline over controlled
// in-memory TLS transport. The caller owns the injected database and entropy.
func FoundationServer(t *testing.T, db *sql.DB, now func() time.Time, entropy io.Reader, methods ...string) (*grpc.ClientConn, *configs.Foundation) {
	t.Helper()
	private, public := AccessPEM(t)
	cfg := &configs.AppConfig{ApplicationName: AccessIssuer,
		AccessIssuer:       config.AccessIssuerConfig{PrivateKeyFile: "private", PublicKeyFile: "public", KeyVersion: "test-v1"},
		AuthLimiter:        config.AuthLimiterConfig{AccountHMACSecret: "test-only-account-secret"},
		CommandFingerprint: config.CommandFingerprintConfig{ActiveVersion: "test-v1", Keys: map[string]string{"test-v1": strings.Repeat("m", 32)}},
		ServiceAuth:        ServiceConfig(), Timeout: time.Second}
	cfg.ServiceAuth.Callers[0].Methods = append(cfg.ServiceAuth.Callers[0].Methods, methods...)
	security, err := configs.LoadLoginSecurity(cfg, func(path string) ([]byte, error) {
		if path == "private" {
			return private, nil
		}
		return public, nil
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	fingerprinter, err := configs.LoadCommandFingerprinter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	foundation, err := configs.PrepareFoundation(context.Background(), security, db, dialect.NewPostgres(), database.GetORMConfig(), now, entropy, fingerprinter)
	if err != nil {
		t.Fatal(err)
	}
	var cert []byte
	cfg.TLS, cert = ServiceCertificate(t, 1, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	server, err := configs.NewGRPCServer(cfg, foundation.Pipeline)
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	done := make(chan error, 1)
	go func() { done <- server.Server.Serve(listener) }()
	t.Cleanup(func() {
		server.GracefulStop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(cert)
	conn, err := grpc.NewClient("passthrough:///auth.test", grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "auth.test", Time: now,
	})), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, foundation
}

func RPCContext(ctx context.Context, token string) context.Context {
	values := []string{serviceauth.MetadataKey, ServiceToken(1), locale.MetadataKey, "id"}
	if token != "" {
		values = append(values, "authorization", "Bearer "+token)
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(values...))
}
