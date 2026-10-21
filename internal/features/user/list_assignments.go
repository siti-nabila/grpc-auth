package user

import (
	"context"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *assignmentService) ListUserRoles(ctx context.Context, q domain.CatalogQuery) (res domain.UserRoleAssignmentPage, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = domain.UserRoleAssignmentPage{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return res, err
	}
	q, err = domain.UserRoleAssignmentCatalog.Normalize(q)
	if err != nil {
		return res, err
	}
	p, err := assignmentPrincipal(ctx, userv1.UserService_ListUserRoles_FullMethodName)
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
	_, err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), userv1.UserService_ListUserRoles_FullMethodName, types.PermissionIAMUserRead, r.State, r.Permissions, s.deps.Now().UTC())
	if err != nil {
		return res, err
	}
	before, err := assignmentTarget(ctx, r, q.TargetUserID)
	if err != nil {
		return res, err
	}
	res.UserID = before.ID
	res.Version = before.Version
	res.Page, err = r.Assignments.ListAssignments(ctx, q)
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}
