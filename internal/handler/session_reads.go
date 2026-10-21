package handler

import (
	"context"
	"errors"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SessionReadHandler is composed with all UserService methods by the default
// foundation registration. It cannot register a partial UserService.
type SessionReadHandler struct {
	snapshots domain.ReadSnapshotProvider
	now       func() time.Time
}

func NewSessionReadHandler(snapshots domain.ReadSnapshotProvider, now func() time.Time) (*SessionReadHandler, error) {
	if snapshots == nil || now == nil {
		return nil, errors.New("session reads require snapshot provider and clock")
	}
	return &SessionReadHandler{snapshots: snapshots, now: now}, nil
}

func (h *SessionReadHandler) Me(ctx context.Context, _ *emptypb.Empty) (*userv1.UserData, error) {
	principal, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return authfeature.ReadUserDataSnapshot(ctx, principal.Claims(), h.snapshots, h.now())
}

func (h *SessionReadHandler) ListUsers(ctx context.Context, input *userv1.ListUsersRequest) (*userv1.ListUsersResponse, error) {
	principal, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	request, err := UserListRequestFromProto(input)
	if err != nil {
		return nil, err
	}
	page, err := user.ReadUsersSnapshot(ctx, principal.Claims(), request, h.snapshots, h.now())
	if err != nil {
		return nil, err
	}
	return ListUsersResponseFromPage(page), nil
}
