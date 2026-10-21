package application

import (
	"context"
	"errors"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func principal(ctx context.Context, method string) (sessions.Principal, error) {
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return sessions.Principal{}, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return sessions.Principal{}, err
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return p, err
	}
	return p, p.AuthorizeMethod(method)
}

func target(ctx context.Context, r Repositories, id uint64) (domain.SessionUserState, error) {
	before, err := r.Users.GetByID(ctx, id)
	if errors.Is(err, auth.ErrNotFound) {
		return before, auth.ErrNotFound
	}
	if err != nil {
		return before, featurecommon.DependencyError(err)
	}
	if before.ID != id || before.DeletedAt.Valid {
		return before, auth.ErrNotFound
	}
	if before.Version < 1 || !domain.ManagementUserStatus(before.Status) {
		return before, common.ErrDependencyUnavailable
	}
	return before, nil
}

func (s *applicationService) ListUserApplicationAccess(ctx context.Context, q domain.CatalogQuery) (res domain.ApplicationAccessPage, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = domain.ApplicationAccessPage{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return res, err
	}
	q, err = domain.UserApplicationAccessCatalog.Normalize(q)
	if err != nil {
		return res, err
	}
	p, err := principal(ctx, userv1.UserService_ListUserApplicationAccess_FullMethodName)
	if err != nil {
		return res, err
	}
	tx, err := s.deps.Snapshots.BeginSnapshot(ctx)
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	r := s.deps.Bind(tx)
	if !r.complete() {
		return res, common.ErrInternalServerError
	}
	_, err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), userv1.UserService_ListUserApplicationAccess_FullMethodName, types.PermissionIAMUserRead, r.State, r.Permissions, s.deps.Now().UTC())
	if err != nil {
		return res, err
	}
	before, err := target(ctx, r, q.TargetUserID)
	if err != nil {
		return res, err
	}
	res.UserID = before.ID
	res.Version = before.Version
	res.Page, err = r.Access.ListAccess(ctx, q)
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}
