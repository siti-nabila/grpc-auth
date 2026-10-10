package interceptors

import (
	"context"
	"errors"
	"strings"
	"time"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The default foundation server composes this boundary with real Login and
// every authoritative consumer, never with the legacy token-only producer.
func NewSessionInterceptor(verifier *jwt.AccessVerifier, reader domain.SessionStateReader, now func() time.Time) (grpc.UnaryServerInterceptor, error) {
	if verifier == nil || reader == nil || now == nil {
		return nil, errors.New("session interceptor requires verifier state reader and clock")
	}
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if _, err := serviceauth.CallerFromContext(ctx); err != nil {
			return nil, err
		}
		switch info.FullMethod {
		case userv1.UserService_Login_FullMethodName, userv1.UserService_Register_FullMethodName, userv1.UserService_TesRPC_FullMethodName:
			return next(ctx, request)
		case userv1.UserService_Me_FullMethodName, userv1.UserService_ListUsers_FullMethodName, profilev1.ProfileService_UpdateProfile_FullMethodName:
		default:
			return nil, common.ErrForbidden
		}
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("authorization")
		if len(values) != 1 {
			return nil, auth.ErrInvalidSession
		}
		parts := strings.Split(values[0], " ")
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			return nil, auth.ErrInvalidSession
		}
		claims, err := verifier.Verify(parts[1])
		if err != nil {
			return nil, err
		}
		verified, err := authfeature.AuthenticateSession(ctx, claims, reader, now())
		if err != nil {
			return nil, err
		}
		principal, err := sessions.PrincipalFromContext(verified)
		if err != nil {
			return nil, err
		}
		if err = principal.AuthorizeMethod(info.FullMethod); err != nil {
			return nil, err
		}
		return next(verified, request)
	}, nil
}
