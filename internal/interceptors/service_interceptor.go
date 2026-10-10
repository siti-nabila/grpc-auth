package interceptors

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"google.golang.org/grpc"
)

func ServiceInterceptor(verifier *serviceauth.Verifier) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		verified, err := verifier.Authenticate(ctx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(verified, req)
	}
}
