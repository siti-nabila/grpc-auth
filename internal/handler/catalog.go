package handler

import (
	"context"
	"errors"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/catalog"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/orm/orm"
)

type CatalogHandler struct{ service catalog.Service }

func NewCatalogHandler(service catalog.Service) (*CatalogHandler, error) {
	if service == nil {
		return nil, errors.New("catalog handler requires service")
	}
	return &CatalogHandler{service}, nil
}

func catalogQueryFromProto(in *paginatorv1.PageQuery) (domain.CatalogQuery, error) {
	if in == nil {
		return domain.CatalogQuery{}, nil
	}
	q := domain.CatalogQuery{Page: int(in.Page), Limit: int(in.Limit), Fields: append([]string(nil), in.Fields...), LastID: in.LastId}
	for _, s := range in.Sort {
		if s == nil {
			return q, common.ErrBadRequest
		}
		q.Sort = append(q.Sort, orm.SortField{Field: s.Field, Desc: s.Desc})
	}
	if in.Search != nil {
		mode, err := searchModeFromProto(in.Search.Mode)
		if err != nil {
			return q, err
		}
		q.Search = &orm.SearchQuery{Fields: append([]string(nil), in.Search.Fields...), Keyword: in.Search.Keyword, Mode: mode}
	}
	return q, nil
}

func (h *CatalogHandler) ListPermissions(ctx context.Context, in *permissionv1.ListPermissionsRequest) (*permissionv1.ListPermissionsResponse, error) {
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
	q.ApplicationCode = in.ApplicationCode
	q, err = domain.PermissionCatalog.Normalize(q)
	if err != nil {
		return nil, err
	}
	page, err := h.service.ListPermissions(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*permissionv1.PermissionItem, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, permissionResponse(row.Project(q)))
	}
	return &permissionv1.ListPermissionsResponse{Items: items, Meta: catalogMeta(page.Page, page.Limit, page.Total, page.TotalPages, page.HasNext, page.HasPrev), NextCursor: page.NextCursor}, nil
}

func (h *CatalogHandler) ListApplications(ctx context.Context, in *applicationv1.ListApplicationsRequest) (*applicationv1.ListApplicationsResponse, error) {
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
	q.Statuses = append([]string(nil), in.GetFilter().GetStatuses()...)
	q, err = domain.ApplicationCatalog.Normalize(q)
	if err != nil {
		return nil, err
	}
	page, err := h.service.ListApplications(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*applicationv1.ApplicationItem, 0, len(page.Items))
	for _, row := range page.Items {
		r := row.Project(q)
		items = append(items, &applicationv1.ApplicationItem{Id: r.ID, Code: string(r.Code), Name: r.Name, Status: string(r.Status), IsSystem: r.IsSystem})
	}
	return &applicationv1.ListApplicationsResponse{Items: items, Meta: catalogMeta(page.Page, page.Limit, page.Total, page.TotalPages, page.HasNext, page.HasPrev), NextCursor: page.NextCursor}, nil
}

func permissionResponse(r domain.PermissionItem) *permissionv1.PermissionItem {
	return &permissionv1.PermissionItem{Id: r.ID, ApplicationId: r.ApplicationID, ApplicationCode: string(r.ApplicationCode), Code: r.Code, Description: r.Description}
}
func permissionResponses(rows []domain.PermissionItem) []*permissionv1.PermissionItem {
	out := make([]*permissionv1.PermissionItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, permissionResponse(r))
	}
	return out
}
func catalogMeta(page, limit int, total int64, pages int, next, prev bool) *paginatorv1.PageMeta {
	return &paginatorv1.PageMeta{Page: int32(page), Limit: int32(limit), Total: total, TotalPages: int32(pages), HasNext: next, HasPrev: prev}
}
