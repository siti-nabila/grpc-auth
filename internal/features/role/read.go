package role

import (
	"context"
	"errors"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/orm/orm"
)

func (s *readService) ListRoles(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error) {
	var page orm.PageData[domain.RoleSummary]
	err := s.read(ctx, rolev1.RoleService_ListRoles_FullMethodName, func(r ReadRepositories) error {
		normalized, err := domain.RoleCatalog.Normalize(q)
		if err != nil {
			return err
		}
		page, err = r.Catalog.Roles(ctx, normalized)
		return err
	})
	if err != nil {
		return orm.PageData[domain.RoleSummary]{}, err
	}
	return page, nil
}

func (s *readService) GetRole(ctx context.Context, id uint64) (domain.RoleDetail, error) {
	var detail domain.RoleDetail
	err := s.read(ctx, rolev1.RoleService_GetRole_FullMethodName, func(r ReadRepositories) error {
		if err := domain.ValidateManagementTarget(id); err != nil {
			return err
		}
		var err error
		detail.Role, err = r.Roles.Role(ctx, id)
		if err != nil {
			return err
		}
		detail.Permissions, err = r.Roles.RolePermissions(ctx, id)
		return err
	})
	if err != nil {
		return domain.RoleDetail{}, err
	}
	return detail, nil
}

func (s *readService) read(ctx context.Context, method string, load func(ReadRepositories) error) (err error) {
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
	if r.State == nil || r.Permissions == nil || r.Roles == nil || r.Catalog == nil {
		return common.ErrDependencyUnavailable
	}
	if _, err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), method, types.PermissionIAMRoleRead, r.State, r.Permissions, s.deps.Now().UTC()); err != nil {
		return err
	}
	if err = load(r); err != nil {
		if errors.Is(err, roleerrors.ErrRoleNotFound) || errors.Is(err, common.ErrBadRequest) {
			return err
		}
		return featurecommon.DependencyError(err)
	}
	return featurecommon.DependencyError(owner.Commit())
}
