package interceptors

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/locale"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func LanguageInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		langs := md.Get(locale.MetadataKey)
		if len(langs) > 0 {
			ctx = locale.NewContext(ctx, langs[0])
		}
	}
	return handler(ctx, req)
}
