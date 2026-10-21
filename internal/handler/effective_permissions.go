package handler

import (
	"context"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
)

func (h *SessionReadHandler) GetMyEffectivePermissions(ctx context.Context, in *userv1.GetMyEffectivePermissionsRequest) (*userv1.GetEffectivePermissionsResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	input := domain.MyEffectivePermissionsInput{ApplicationCode: in.ApplicationCode}
	res, err := user.ReadMyEffectivePermissionsSnapshot(ctx, p.Claims(), input, h.snapshots, h.now())
	if err != nil {
		return nil, err
	}
	return effectivePermissionsResponse(res), nil
}

func (h *SessionReadHandler) GetUserEffectivePermissions(ctx context.Context, in *userv1.GetUserEffectivePermissionsRequest) (*userv1.GetEffectivePermissionsResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	input := domain.UserEffectivePermissionsInput{UserID: in.UserId, ApplicationCode: in.ApplicationCode}
	res, err := user.ReadUserEffectivePermissionsSnapshot(ctx, p.Claims(), input, h.snapshots, h.now())
	if err != nil {
		return nil, err
	}
	return effectivePermissionsResponse(res), nil
}

func effectivePermissionsResponse(res domain.EffectivePermissionsResult) *userv1.GetEffectivePermissionsResponse {
	out := &userv1.GetEffectivePermissionsResponse{UserId: res.UserID, UserStatus: string(res.UserStatus), Version: uint64(res.Version), Permissions: make([]*userv1.EffectivePermission, 0, len(res.Permissions))}
	for _, permission := range res.Permissions {
		item := &userv1.EffectivePermission{Code: permission.Code, Roles: make([]*userv1.EffectivePermissionRole, 0, len(permission.Roles))}
		for _, role := range permission.Roles {
			item.Roles = append(item.Roles, &userv1.EffectivePermissionRole{RoleId: role.ID, Code: role.Code})
		}
		out.Permissions = append(out.Permissions, item)
	}
	return out
}
