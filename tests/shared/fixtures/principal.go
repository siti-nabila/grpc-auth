package fixtures

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func CallerContext(t *testing.T, ctx context.Context, method string) context.Context {
	t.Helper()
	v, err := serviceauth.NewVerifier(ServiceConfig(), func() time.Time { return ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, ServiceToken(1), "user_id", "999", "x-request-id", "request-test"))
	ctx, err = v.Authenticate(ctx, method)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func PrincipalContext(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	p, err := sessions.PrincipalForState(AccessClaims(t), SessionState(), ServiceTime)
	if err != nil {
		t.Fatal(err)
	}
	return p.Context(ctx)
}
