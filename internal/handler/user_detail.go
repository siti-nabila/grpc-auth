package handler

import (
	"context"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *SessionReadHandler) GetUser(ctx context.Context, input *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if input == nil || domain.ValidateManagementTarget(input.GetId()) != nil {
		return nil, common.ErrBadRequest
	}
	detail, err := user.ReadUserDetailSnapshot(ctx, p.Claims(), input.GetId(), h.snapshots, h.now())
	if err != nil {
		return nil, err
	}
	return &userv1.GetUserResponse{User: SafeUserResponse(detail)}, nil
}

// SafeUserResponse never copies a credential-bearing database or proto model.
func SafeUserResponse(r domain.UserDetail) *userv1.User {
	u := &userv1.User{Id: r.ID, Email: r.Email, Name: r.Name, Address: r.Address, PhoneNo: r.Phone, Status: string(r.Status), Version: uint64(r.Version), MustChangePassword: r.MustChangePassword}
	if !r.CreatedAt.IsZero() {
		u.CreatedAt = timestamppb.New(r.CreatedAt)
	}
	for _, role := range r.Roles {
		u.Roles = append(u.Roles, &userv1.UserRoleAssignment{RoleId: role.ID, Code: role.Code, Name: role.Name, Status: string(role.Status), RoleStatus: string(role.RoleStatus)})
	}
	for _, app := range r.ApplicationAccess {
		u.ApplicationAccess = append(u.ApplicationAccess, &userv1.UserApplicationAccess{ApplicationId: app.ID, Code: app.Code, Name: app.Name, Status: string(app.Status), ApplicationStatus: string(app.ApplicationStatus)})
	}
	return u
}
