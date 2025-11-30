package inteceptors

import (
	"context"

	errorpackage "github.com/siti-nabila/error-package"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type (
	ctxKey string
)

const (
	ContextKeyLanguage ctxKey = "X-Language"
)

func LanguageInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		langs := md.Get(string(ContextKeyLanguage))
		if len(langs) > 0 {
			ctx = context.WithValue(ctx, ContextKeyLanguage, langs[0])
			errorpackage.SetLanguage(langs[0])

		}
	}
	return handler(ctx, req)
}
