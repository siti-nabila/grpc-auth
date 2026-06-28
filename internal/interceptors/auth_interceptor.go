package interceptors

import (
	"context"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	jwtPackage "github.com/siti-nabila/grpc-auth/pkg/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TokenInterceptor(secret string) grpc.UnaryServerInterceptor {

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {

		if isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		tokenStr, err := extractTokenFromContext(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "missing token")
		}

		session, err := parseUserSession(tokenStr, secret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		ctx = context.WithValue(ctx, sessions.UserSessionKey, session)

		return handler(ctx, req)
	}
}
func parseUserSession(tokenStr string, secret string) (*sessions.UserSession, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwtPackage.JwtClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*jwtPackage.JwtClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return &sessions.UserSession{
		UserId: claims.UserId,
	}, nil
}

func extractTokenFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("missing metadata")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return "", errors.New("authorization header not found")
	}

	token := authHeaders[0]
	const prefix = "Bearer "

	if !strings.HasPrefix(token, prefix) {
		return "", errors.New("invalid authorization format")
	}

	token = strings.TrimPrefix(token, prefix)
	if token == "" {
		return "", errors.New("empty token")
	}

	return token, nil
}

func isPublicMethod(fullMethod string) bool {
	publicMethods := map[string]bool{
		"/user.UserService/Login":    true,
		"/user.UserService/Register": true,
		"/user.UserService/TesRPC":   true,
	}
	return publicMethods[fullMethod]
}
