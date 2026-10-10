package interceptors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"strconv"
	"strings"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	clientIPMetadataKey        = "x-client-ip"
	rateLimitReasonMetadataKey = "rate_limit_reason"
	rateIPKeyPrefix            = "ip:"
	rateServiceKeyPrefix       = "service:"
)

func NewAuthRateInterceptor(store *ratelimit.Store, accountSecret []byte, encoder ErrorEncoder) (grpc.UnaryServerInterceptor, error) {
	if store == nil || len(accountSecret) == 0 || encoder == nil {
		return nil, errors.New("rate interceptor requires store account secret and error encoder")
	}
	secret := append([]byte(nil), accountSecret...)
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		caller, err := serviceauth.CallerFromContext(ctx)
		if err != nil {
			return nil, err
		}
		keys, err := rateKeys(ctx, request, info.FullMethod, caller, secret)
		if err != nil {
			return nil, err
		}
		if len(keys) == 0 {
			return next(ctx, request)
		}
		decision, err := store.Admit(keys...)
		if err != nil {
			return nil, common.ErrInternalServerError
		}
		if !decision.Allowed {
			return nil, rateError(ctx, encoder, decision)
		}
		return next(ctx, request)
	}, nil
}

func rateKeys(ctx context.Context, request any, method string, caller serviceauth.Caller, secret []byte) ([]ratelimit.Key, error) {
	switch method {
	case userv1.UserService_Login_FullMethodName:
		email := ""
		if req, ok := request.(*userv1.AuthRequest); ok {
			email = req.GetEmail()
		}
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
		return []ratelimit.Key{{Group: ratelimit.LoginIP, Value: trustedIPKey(ctx, caller)}, {Group: ratelimit.LoginAccount, Value: hex.EncodeToString(mac.Sum(nil))}}, nil
	case userv1.UserService_Register_FullMethodName:
		return []ratelimit.Key{{Group: ratelimit.RegisterIP, Value: trustedIPKey(ctx, caller)}}, nil
	case userv1.UserService_Me_FullMethodName, userv1.UserService_ListUsers_FullMethodName:
		principal, err := sessions.PrincipalFromContext(ctx)
		if err != nil {
			return nil, err
		}
		return []ratelimit.Key{{Group: ratelimit.ProtectedRead, Value: strconv.FormatUint(principal.UserID(), 10)}}, nil
	}
	return nil, nil
}

func trustedIPKey(ctx context.Context, caller serviceauth.Caller) string {
	if caller.ID() == serviceauth.RestORCServiceID {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get(clientIPMetadataKey)
		if len(values) == 1 {
			if addr, err := netip.ParseAddr(values[0]); err == nil && addr.Zone() == "" {
				return rateIPKeyPrefix + addr.Unmap().String()
			}
		}
	}
	return rateServiceKeyPrefix + caller.ID()
}

func rateError(ctx context.Context, encoder ErrorEncoder, decision ratelimit.Decision) error {
	base := status.Convert(encoder.Encode(common.ErrRateLimited, locale.FromContext(ctx)))
	for _, detail := range base.Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok {
			if info.Metadata == nil {
				info.Metadata = map[string]string{}
			}
			info.Metadata[rateLimitReasonMetadataKey] = string(decision.Reason)
			// Rebuild only this detail so there is one authoritative ErrorInfo.
			with, err := status.New(base.Code(), base.Message()).WithDetails(info, &errdetails.RetryInfo{RetryDelay: durationpb.New(decision.RetryAfter)})
			if err != nil {
				return common.ErrInternalServerError
			}
			return with.Err()
		}
	}
	return common.ErrInternalServerError
}
