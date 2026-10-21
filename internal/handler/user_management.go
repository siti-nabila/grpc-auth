package handler

import (
	"context"
	"errors"
	"math"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"google.golang.org/grpc/metadata"
)

type UserManagementHandler struct{ service user.ManagementService }

func NewUserManagementHandler(service user.ManagementService) (*UserManagementHandler, error) {
	if service == nil {
		return nil, errors.New("user management handler requires service")
	}
	return &UserManagementHandler{service}, nil
}

func managementKey(ctx context.Context, required bool) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	keys := md.Get(idempotencyMetadataKey)
	if !required && len(keys) == 0 {
		return "", nil
	}
	if len(keys) != 1 {
		return "", common.ErrBadRequest
	}
	if err := domain.ValidateManagementKey(keys[0], true); err != nil {
		return "", err
	}
	return keys[0], nil
}

func managementExpectedVersion(version *uint64) (int64, error) {
	if version == nil || *version < 1 || *version > math.MaxInt64 {
		return 0, common.ErrBadRequest
	}
	return int64(*version), nil
}

func (h *UserManagementHandler) CreateUser(ctx context.Context, r *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
	if r == nil {
		return nil, common.ErrBadRequest
	}
	key, err := managementKey(ctx, true)
	if err != nil {
		return nil, err
	}
	input := domain.CreateUserInput{Name: r.Name, Email: r.Email, Password: r.Password, Address: r.Address, Phone: r.Phone, Key: key}
	if r.Status != nil {
		status := types.UserStatus(*r.Status)
		input.Status = &status
	}
	result, err := h.service.CreateUser(ctx, input)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, common.ErrInternalServerError
	}
	detail, err := result.UserDetail()
	if err != nil {
		return nil, common.ErrInternalServerError
	}
	return &userv1.CreateUserResponse{User: SafeUserResponse(detail)}, nil
}

func (h *UserManagementHandler) UpdateUser(ctx context.Context, r *userv1.UpdateUserRequest) (*userv1.UpdateUserResponse, error) {
	if r == nil || r.FieldMask == nil {
		return nil, common.ErrBadRequest
	}
	version, err := managementExpectedVersion(r.ExpectedUserVersion)
	if err != nil {
		return nil, err
	}
	key, err := managementKey(ctx, false)
	if err != nil {
		return nil, err
	}
	input := domain.UpdateUserInput{UserID: r.UserId, ExpectedVersion: version, Name: r.Name, Address: r.Address, Phone: r.Phone, Fields: append([]string(nil), r.FieldMask.Paths...), Key: key}
	result, err := h.service.UpdateUser(ctx, input)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, common.ErrInternalServerError
	}
	detail, err := result.UserDetail()
	if err != nil {
		return nil, common.ErrInternalServerError
	}
	return &userv1.UpdateUserResponse{User: SafeUserResponse(detail), Changed: result.Changed}, nil
}

func (h *UserManagementHandler) ChangeUserStatus(ctx context.Context, r *userv1.ChangeUserStatusRequest) (*userv1.ChangeUserStatusResponse, error) {
	if r == nil {
		return nil, common.ErrBadRequest
	}
	version, err := managementExpectedVersion(r.ExpectedUserVersion)
	if err != nil {
		return nil, err
	}
	key, err := managementKey(ctx, true)
	if err != nil {
		return nil, err
	}
	input := domain.ChangeUserStatusInput{UserID: r.UserId, ExpectedVersion: version, Reason: r.Reason, Key: key}
	if r.Status != nil {
		status := types.UserStatus(*r.Status)
		input.Status = &status
	}
	result, err := h.service.ChangeUserStatus(ctx, input)
	if err != nil {
		return nil, err
	}
	if result == nil || result.UserID != r.UserId || result.Version < 1 || !domain.ManagementUserStatus(types.UserStatus(result.Status)) {
		return nil, common.ErrInternalServerError
	}
	return &userv1.ChangeUserStatusResponse{UserId: result.UserID, Status: result.Status, Version: uint64(result.Version), Changed: result.Changed}, nil
}
