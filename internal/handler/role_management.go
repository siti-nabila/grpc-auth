package handler

import (
	"context"
	"errors"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type RoleManagementHandler struct{ service role.ManagementService }

func NewRoleManagementHandler(service role.ManagementService) (*RoleManagementHandler, error) {
	if service == nil {
		return nil, errors.New("role management handler requires service")
	}
	return &RoleManagementHandler{service}, nil
}

func roleResult(result *domain.SafeCommandResult, target uint64) (domain.RoleDetail, error) {
	if result == nil || result.Role == nil || target != 0 && result.RoleID != target {
		return domain.RoleDetail{}, common.ErrInternalServerError
	}
	detail, err := result.RoleDetail()
	if err != nil {
		return domain.RoleDetail{}, common.ErrInternalServerError
	}
	return detail, nil
}

func (h *RoleManagementHandler) CreateRole(ctx context.Context, in *rolev1.CreateRoleRequest) (*rolev1.CreateRoleResponse, error) {
	if _, err := sessions.PrincipalFromContext(ctx); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, common.ErrBadRequest
	}
	key, err := managementKey(ctx, true)
	if err != nil {
		return nil, err
	}
	input := (domain.CreateRoleInput{ApplicationCode: in.ApplicationCode, Code: in.Code, Name: in.Name, Description: in.Description, PermissionCodes: in.PermissionCodes, Key: key}).Normalized()
	if err = input.Validate(); err != nil {
		return nil, err
	}
	result, err := h.service.CreateRole(ctx, input)
	if err != nil {
		return nil, err
	}
	detail, err := roleResult(result, 0)
	if err != nil {
		return nil, err
	}
	if detail.Role.Code != input.Code || string(detail.Role.ApplicationCode) != input.ApplicationCode {
		return nil, common.ErrInternalServerError
	}
	return &rolev1.CreateRoleResponse{Role: roleResponse(detail.Role), Permissions: permissionResponses(detail.Permissions)}, nil
}

func (h *RoleManagementHandler) UpdateRole(ctx context.Context, in *rolev1.UpdateRoleRequest) (*rolev1.UpdateRoleResponse, error) {
	if _, err := sessions.PrincipalFromContext(ctx); err != nil {
		return nil, err
	}
	if in == nil || in.FieldMask == nil {
		return nil, common.ErrBadRequest
	}
	version, err := managementExpectedVersion(in.ExpectedRoleVersion)
	if err != nil {
		return nil, err
	}
	key, err := managementKey(ctx, false)
	if err != nil {
		return nil, err
	}
	input := (domain.UpdateRoleInput{RoleID: in.RoleId, ExpectedVersion: version, Name: in.Name, Description: in.Description, Fields: append([]string(nil), in.FieldMask.Paths...), Key: key}).Normalized()
	if err = input.Validate(); err != nil {
		return nil, err
	}
	result, err := h.service.UpdateRole(ctx, input)
	if err != nil {
		return nil, err
	}
	detail, err := roleResult(result, in.RoleId)
	if err != nil {
		return nil, err
	}
	return &rolev1.UpdateRoleResponse{Role: roleResponse(detail.Role), Changed: result.Changed}, nil
}

func (h *RoleManagementHandler) ChangeRoleStatus(ctx context.Context, in *rolev1.ChangeRoleStatusRequest) (*rolev1.ChangeRoleStatusResponse, error) {
	if _, err := sessions.PrincipalFromContext(ctx); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, common.ErrBadRequest
	}
	version, err := managementExpectedVersion(in.ExpectedRoleVersion)
	if err != nil {
		return nil, err
	}
	key, err := managementKey(ctx, true)
	if err != nil {
		return nil, err
	}
	input := domain.ChangeRoleStatusInput{RoleID: in.RoleId, ExpectedVersion: version, Reason: in.Reason, Key: key}
	if in.Status != nil {
		status := types.RoleStatus(*in.Status)
		input.Status = &status
	}
	input = input.Normalized()
	if err = input.Validate(); err != nil {
		return nil, err
	}
	result, err := h.service.ChangeRoleStatus(ctx, input)
	if err != nil {
		return nil, err
	}
	if _, err = roleResult(result, in.RoleId); err != nil {
		return nil, err
	}
	return &rolev1.ChangeRoleStatusResponse{RoleId: result.RoleID, Status: result.Status, Version: uint64(result.Version), Changed: result.Changed}, nil
}

func (h *RoleManagementHandler) ReplaceRolePermissions(ctx context.Context, in *rolev1.ReplaceRolePermissionsRequest) (*rolev1.ReplaceRolePermissionsResponse, error) {
	if _, err := sessions.PrincipalFromContext(ctx); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, common.ErrBadRequest
	}
	version, err := managementExpectedVersion(in.ExpectedRoleVersion)
	if err != nil {
		return nil, err
	}
	key, err := managementKey(ctx, true)
	if err != nil {
		return nil, err
	}
	input := domain.ReplaceRolePermissionsInput{RoleID: in.RoleId, ExpectedVersion: version, Reason: in.Reason, Key: key}
	if in.Permissions != nil {
		input.Permissions = &domain.PermissionSet{Codes: in.Permissions.Codes}
	}
	input = input.Normalized()
	if err = input.Validate(); err != nil {
		return nil, err
	}
	result, err := h.service.ReplaceRolePermissions(ctx, input)
	if err != nil {
		return nil, err
	}
	detail, err := roleResult(result, in.RoleId)
	if err != nil {
		return nil, err
	}
	return &rolev1.ReplaceRolePermissionsResponse{Role: roleResponse(detail.Role), Permissions: permissionResponses(detail.Permissions), Changed: result.Changed}, nil
}
