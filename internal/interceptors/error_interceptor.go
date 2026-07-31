package interceptors

import (
	"context"

	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"google.golang.org/grpc"
)

func ErrorInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	response, err := handler(ctx, req)
	if err != nil {
		return nil, helpers.HandleErrorContext(ctx, err)
	}
	return response, nil
}
