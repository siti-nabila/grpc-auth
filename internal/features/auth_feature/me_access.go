package authfeature

import (
	"context"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
)

func readMeAccess(ctx context.Context, tx domain.Transaction, userID uint64, caller serviceauth.Caller, res *userv1.UserData) error {
	if err := readMeRoles(ctx, tx, userID, caller, res); err != nil {
		return err
	}
	if err := readMeApplications(ctx, tx, userID, caller, res); err != nil {
		return err
	}
	return readMePermissions(ctx, tx, userID, caller, res)
}

func readMeRoles(ctx context.Context, tx domain.Transaction, userID uint64, caller serviceauth.Caller, res *userv1.UserData) error {
	legacy, err := reader.NewUserIdentityReader(tx).ActiveRoles(ctx, userID)
	if err != nil {
		return err
	}
	for _, role := range legacy {
		res.RoleCodes = append(res.RoleCodes, int32(role.Code))
		res.RoleNames = append(res.RoleNames, role.Name)
	}
	roles, err := reader.NewSelfAccessReader(tx).Roles(ctx, userID)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if caller.AuthorizeNamespace(string(role.ApplicationCode)) == nil {
			res.Roles = append(res.Roles, &userv1.UserRoleAssignment{RoleId: role.ID, Code: role.Code, Name: role.Name, Status: string(role.Status), RoleStatus: string(role.RoleStatus)})
		}
	}
	return nil
}

func readMeApplications(ctx context.Context, tx domain.Transaction, userID uint64, caller serviceauth.Caller, res *userv1.UserData) error {
	apps, err := reader.NewSelfAccessReader(tx).Applications(ctx, userID)
	if err != nil {
		return err
	}
	for _, app := range apps {
		if caller.AuthorizeNamespace(string(app.Code)) == nil {
			res.ApplicationAccess = append(res.ApplicationAccess, &userv1.UserApplicationAccess{ApplicationId: app.ID, Code: string(app.Code), Name: app.Name, Status: string(app.Status), ApplicationStatus: string(app.ApplicationStatus)})
		}
	}
	return nil
}

func readMePermissions(ctx context.Context, tx domain.Transaction, userID uint64, caller serviceauth.Caller, res *userv1.UserData) error {
	rows, err := reader.NewPermissionReader(tx).Permissions(ctx, userID)
	if err != nil {
		return err
	}
	visible := make([]domain.PermissionContribution, 0, len(rows))
	for _, row := range rows {
		if caller.AuthorizeNamespace(string(row.ApplicationCode)) == nil {
			visible = append(visible, row)
		}
	}
	res.Permissions = make(map[string]bool)
	for _, permission := range featurecommon.ResolvePermissions(visible) {
		res.Permissions[permission.Code] = true
	}
	return nil
}
