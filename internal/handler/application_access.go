package handler

import (
	"context"
	"errors"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/application"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ApplicationAccessHandler struct{ service application.Service }

func NewApplicationAccessHandler(service application.Service) (*ApplicationAccessHandler, error) {
	if service == nil {
		return nil, errors.New("application access handler requires service")
	}
	return &ApplicationAccessHandler{service}, nil
}

func (h *ApplicationAccessHandler) ListUserApplicationAccess(ctx context.Context, in *userv1.ListUserApplicationAccessRequest) (*userv1.ListUserApplicationAccessResponse, error) {
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
	q, err = domain.UserApplicationAccessCatalog.Normalize(q)
	if err != nil {
		return nil, err
	}
	res, err := h.service.ListUserApplicationAccess(ctx, q)
	if err != nil {
		return nil, err
	}
	if res.UserID != in.UserId || res.Version < 1 {
		return nil, common.ErrInternalServerError
	}
	page := res.Page
	out := &userv1.ListUserApplicationAccessResponse{UserId: res.UserID, Version: uint64(res.Version), NextCursor: page.NextCursor, Meta: catalogMeta(page.Page, page.Limit, page.Total, page.TotalPages, page.HasNext, page.HasPrev)}
	for _, row := range page.Items {
		out.Items = append(out.Items, accessResponse(row.Project(q)))
	}
	return out, nil
}

func accessResponse(r domain.ApplicationAccessItem) *userv1.UserApplicationAccessItem {
	out := &userv1.UserApplicationAccessItem{ApplicationId: r.ApplicationID, ApplicationCode: r.ApplicationCode, Name: r.Name, ApplicationStatus: r.ApplicationStatus, IsSystem: r.IsSystem, Status: r.Status, GrantedBy: r.GrantedBy, RevokedBy: r.RevokedBy, RevokeReason: r.RevokeReason}
	if r.GrantedAt != nil {
		out.GrantedAt = timestamppb.New(*r.GrantedAt)
	}
	if r.RevokedAt != nil {
		out.RevokedAt = timestamppb.New(*r.RevokedAt)
	}
	return out
}

func accessInput(ctx context.Context, user uint64, code string, roles []uint64, reason string, version *uint64) (domain.ApplicationAccessInput, error) {
	v, err := managementExpectedVersion(version)
	if err != nil {
		return domain.ApplicationAccessInput{}, err
	}
	key, err := managementKey(ctx, true)
	if err != nil {
		return domain.ApplicationAccessInput{}, err
	}
	return domain.ApplicationAccessInput{UserID: user, ApplicationCode: code, RoleIDs: append([]uint64(nil), roles...), Reason: reason, ExpectedVersion: v, Key: key}, nil
}

func (h *ApplicationAccessHandler) GrantApplicationAccess(ctx context.Context, in *applicationv1.GrantApplicationAccessRequest) (*applicationv1.GrantApplicationAccessResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	input, err := accessInput(ctx, in.UserId, in.ApplicationCode, in.RoleIds, in.Reason, in.ExpectedUserVersion)
	if err != nil {
		return nil, err
	}
	if err = input.ValidateGrant(); err != nil {
		return nil, err
	}
	res, err := h.service.GrantApplicationAccess(ctx, input.Normalized())
	if err != nil {
		return nil, err
	}
	if res == nil || res.ValidateApplicationAccessResult(in.UserId, res.ApplicationID) != nil {
		return nil, common.ErrInternalServerError
	}
	return &applicationv1.GrantApplicationAccessResponse{UserId: res.UserID, ApplicationId: res.ApplicationID, AccessStatus: accessStatus(res.Status), ChangedRoleIds: append([]uint64(nil), res.ChangedRoleIDs...), Version: uint64(res.Version), Changed: res.Changed}, nil
}

func (h *ApplicationAccessHandler) RevokeApplicationAccess(ctx context.Context, in *applicationv1.RevokeApplicationAccessRequest) (*applicationv1.RevokeApplicationAccessResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	input, err := accessInput(ctx, in.UserId, in.ApplicationCode, nil, in.Reason, in.ExpectedUserVersion)
	if err != nil {
		return nil, err
	}
	if err = input.ValidateRevoke(); err != nil {
		return nil, err
	}
	res, err := h.service.RevokeApplicationAccess(ctx, input.Normalized())
	if err != nil {
		return nil, err
	}
	if res == nil || res.ValidateApplicationAccessResult(in.UserId, res.ApplicationID) != nil {
		return nil, common.ErrInternalServerError
	}
	return &applicationv1.RevokeApplicationAccessResponse{UserId: res.UserID, ApplicationId: res.ApplicationID, AccessStatus: accessStatus(res.Status), ChangedRoleIds: append([]uint64(nil), res.ChangedRoleIDs...), Version: uint64(res.Version), Changed: res.Changed}, nil
}

func accessStatus(status string) *string {
	if status == "" {
		return nil
	}
	return &status
}
