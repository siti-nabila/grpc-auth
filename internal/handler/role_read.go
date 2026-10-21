package handler

import (
	"context"
	"errors"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type RoleReadHandler struct{ service role.ReadService }

func NewRoleReadHandler(service role.ReadService) (*RoleReadHandler, error) {
	if service == nil {
		return nil, errors.New("role read handler requires service")
	}
	return &RoleReadHandler{service}, nil
}

func (h *RoleReadHandler) ListRoles(ctx context.Context, in *rolev1.ListRolesRequest) (*rolev1.ListRolesResponse, error) {
	if _, err := sessions.PrincipalFromContext(ctx); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, common.ErrBadRequest
	}
	q, err := catalogQueryFromProto(in.Query)
	if err != nil {
		return nil, err
	}
	q.ApplicationCode = in.GetFilter().GetApplicationCode()
	q.Statuses = append([]string(nil), in.GetFilter().GetStatuses()...)
	q, err = domain.RoleCatalog.Normalize(q)
	if err != nil {
		return nil, err
	}
	page, err := h.service.ListRoles(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*rolev1.RoleSummary, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, roleResponse(row.Project(q)))
	}
	return &rolev1.ListRolesResponse{Items: items, Meta: catalogMeta(page.Page, page.Limit, page.Total, page.TotalPages, page.HasNext, page.HasPrev), NextCursor: page.NextCursor}, nil
}

func (h *RoleReadHandler) GetRole(ctx context.Context, in *rolev1.GetRoleRequest) (*rolev1.GetRoleResponse, error) {
	if _, err := sessions.PrincipalFromContext(ctx); err != nil {
		return nil, err
	}
	if in == nil || domain.ValidateManagementTarget(in.RoleId) != nil {
		return nil, common.ErrBadRequest
	}
	detail, err := h.service.GetRole(ctx, in.RoleId)
	if err != nil {
		return nil, err
	}
	return &rolev1.GetRoleResponse{Role: roleResponse(detail.Role), Permissions: permissionResponses(detail.Permissions)}, nil
}

func roleResponse(r domain.RoleSummary) *rolev1.RoleSummary {
	out := &rolev1.RoleSummary{Id: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, ApplicationId: r.ApplicationID, ApplicationCode: string(r.ApplicationCode), Status: string(r.Status), IsSystem: r.IsSystem, Version: uint64(r.Version), RoleCode: r.NumericCode, AffectedUserCount: r.AffectedUserCount}
	if r.CreatedAt != nil {
		out.CreatedAt = timestamppb.New(*r.CreatedAt)
	}
	if r.UpdatedAt != nil {
		out.UpdatedAt = timestamppb.New(*r.UpdatedAt)
	}
	return out
}
