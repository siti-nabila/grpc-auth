package handler

import (
	"context"
	"errors"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserRoleAssignmentHandler struct{ service user.AssignmentService }

func NewUserRoleAssignmentHandler(service user.AssignmentService) (*UserRoleAssignmentHandler, error) {
	if service == nil {
		return nil, errors.New("user role assignment handler requires service")
	}
	return &UserRoleAssignmentHandler{service}, nil
}

func (h *UserRoleAssignmentHandler) ListUserRoles(ctx context.Context, in *userv1.ListUserRolesRequest) (*userv1.ListUserRolesResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	q, err := catalogQueryFromProto(in.Query)
	if err != nil {
		return nil, err
	}
	q.TargetUserID = in.UserId
	q.ApplicationCode = in.GetFilter().GetApplicationCode()
	q.Statuses = append([]string(nil), in.GetFilter().GetStatuses()...)
	q, err = domain.UserRoleAssignmentCatalog.Normalize(q)
	if err != nil {
		return nil, err
	}
	res, err := h.service.ListUserRoles(ctx, q)
	if err != nil {
		return nil, err
	}
	if res.UserID != in.UserId || res.Version < 1 {
		return nil, common.ErrInternalServerError
	}
	page := res.Page
	out := &userv1.ListUserRolesResponse{UserId: res.UserID, Version: uint64(res.Version), NextCursor: page.NextCursor, Meta: catalogMeta(page.Page, page.Limit, page.Total, page.TotalPages, page.HasNext, page.HasPrev)}
	for _, row := range page.Items {
		out.Items = append(out.Items, assignmentResponse(row.Project(q)))
	}
	return out, nil
}

func assignmentResponse(r domain.UserRoleAssignmentItem) *userv1.UserRoleAssignmentItem {
	out := &userv1.UserRoleAssignmentItem{Id: r.ID, RoleId: r.RoleID, Code: r.Code, RoleCode: r.RoleCode, Name: r.Name, Status: r.Status, RoleStatus: r.RoleStatus, ApplicationId: r.ApplicationID, ApplicationCode: r.ApplicationCode, ApplicationStatus: r.ApplicationStatus, IsSystem: r.IsSystem, ApplicationAccessStatus: r.ApplicationAccessStatus, GrantedBy: r.GrantedBy, RevokedBy: r.RevokedBy, RevokeReason: r.RevokeReason}
	if r.GrantedAt != nil {
		out.GrantedAt = timestamppb.New(*r.GrantedAt)
	}
	if r.RevokedAt != nil {
		out.RevokedAt = timestamppb.New(*r.RevokedAt)
	}
	return out
}

func assignmentInput(ctx context.Context, user uint64, roles []uint64, reason string, version *uint64) (domain.UserRoleAssignmentInput, error) {
	v, err := managementExpectedVersion(version)
	if err != nil {
		return domain.UserRoleAssignmentInput{}, err
	}
	key, err := managementKey(ctx, true)
	if err != nil {
		return domain.UserRoleAssignmentInput{}, err
	}
	input := domain.UserRoleAssignmentInput{UserID: user, RoleIDs: append([]uint64(nil), roles...), Reason: reason, ExpectedVersion: v, Key: key}
	if err = input.Validate(); err != nil {
		return input, err
	}
	return input.Normalized(), nil
}

func (h *UserRoleAssignmentHandler) GrantUserRoles(ctx context.Context, in *userv1.GrantUserRolesRequest) (*userv1.GrantUserRolesResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	input, err := assignmentInput(ctx, in.UserId, in.RoleIds, in.Reason, in.ExpectedUserVersion)
	if err != nil {
		return nil, err
	}
	res, err := h.service.GrantUserRoles(ctx, input)
	if err != nil {
		return nil, err
	}
	if res == nil || res.ValidateAssignmentResult(in.UserId) != nil {
		return nil, common.ErrInternalServerError
	}
	return &userv1.GrantUserRolesResponse{UserId: res.UserID, ChangedRoleIds: append([]uint64(nil), res.ChangedRoleIDs...), Version: uint64(res.Version), Changed: res.Changed}, nil
}

func (h *UserRoleAssignmentHandler) RevokeUserRoles(ctx context.Context, in *userv1.RevokeUserRolesRequest) (*userv1.RevokeUserRolesResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	input, err := assignmentInput(ctx, in.UserId, in.RoleIds, in.Reason, in.ExpectedUserVersion)
	if err != nil {
		return nil, err
	}
	res, err := h.service.RevokeUserRoles(ctx, input)
	if err != nil {
		return nil, err
	}
	if res == nil || res.ValidateAssignmentResult(in.UserId) != nil {
		return nil, common.ErrInternalServerError
	}
	return &userv1.RevokeUserRolesResponse{UserId: res.UserID, ChangedRoleIds: append([]uint64(nil), res.ChangedRoleIDs...), Version: uint64(res.Version), Changed: res.Changed}, nil
}
