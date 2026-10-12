package catalog

import (
	"context"
	"errors"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/orm/orm"
)

func (s *service) ListPermissions(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error) {
	var page orm.PageData[domain.PermissionItem]
	err := s.read(ctx, permissionv1.PermissionService_ListPermissions_FullMethodName, func(r Repositories) error {
		normalized, err := domain.PermissionCatalog.Normalize(q)
		if err != nil {
			return err
		}
		page, err = r.Catalog.Permissions(ctx, normalized)
		return err
	})
	if err != nil {
		return orm.PageData[domain.PermissionItem]{}, err
	}
	return page, nil
}

func (s *service) ListApplications(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error) {
	var page orm.PageData[domain.ApplicationItem]
	err := s.read(ctx, applicationv1.ApplicationService_ListApplications_FullMethodName, func(r Repositories) error {
		normalized, err := domain.ApplicationCatalog.Normalize(q)
		if err != nil {
			return err
		}
		page, err = r.Catalog.Applications(ctx, normalized)
		return err
	})
	if err != nil {
		return orm.PageData[domain.ApplicationItem]{}, err
	}
	return page, nil
}

func (s *service) read(ctx context.Context, method string, load func(Repositories) error) (err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return err
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return err
	}
	if err = p.AuthorizeMethod(method); err != nil {
		return err
	}
	tx, err := s.deps.Snapshots.BeginSnapshot(ctx)
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if tx == nil {
		return common.ErrDependencyUnavailable
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	r := s.deps.Bind(tx)
	if r.State == nil || r.Permissions == nil || r.Catalog == nil {
		return common.ErrDependencyUnavailable
	}
	if err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), method, types.PermissionIAMPermissionRead, r.State, r.Permissions, s.deps.Now().UTC()); err != nil {
		return err
	}
	if err = load(r); err != nil {
		if errors.Is(err, common.ErrBadRequest) {
			return err
		}
		return featurecommon.DependencyError(err)
	}
	return featurecommon.DependencyError(owner.Commit())
}
