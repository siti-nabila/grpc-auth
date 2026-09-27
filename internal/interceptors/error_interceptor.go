package interceptors

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/locale"
	"google.golang.org/grpc"
)

type ErrorEncoder interface {
	Encode(err error, language locale.Language) error
}

func NewErrorInterceptor(encoder ErrorEncoder) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		response, err := handler(ctx, req)
		if err != nil {
			return nil, encoder.Encode(err, locale.FromContext(ctx))
		}
		return response, nil
	}
}
