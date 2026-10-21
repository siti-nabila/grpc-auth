package configs

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"time"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/grpc-auth/internal/features/catalog"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"google.golang.org/grpc"
)

type roleServer struct {
	rolev1.UnimplementedRoleServiceServer
	reads      *handler.RoleReadHandler
	management *handler.RoleManagementHandler
}
type catalogServer struct {
	permissionv1.UnimplementedPermissionServiceServer
	applicationv1.UnimplementedApplicationServiceServer
	handler *handler.CatalogHandler
	access  *handler.ApplicationAccessHandler
}

func (s *catalogServer) GrantApplicationAccess(ctx context.Context, r *applicationv1.GrantApplicationAccessRequest) (*applicationv1.GrantApplicationAccessResponse, error) {
	return s.access.GrantApplicationAccess(ctx, r)
}
func (s *catalogServer) RevokeApplicationAccess(ctx context.Context, r *applicationv1.RevokeApplicationAccessRequest) (*applicationv1.RevokeApplicationAccessResponse, error) {
	return s.access.RevokeApplicationAccess(ctx, r)
}

func prepareRoles(provider *database.TransactionProvider, metrics *telemetry.Store, now func() time.Time, eventID func() (string, error)) (*roleServer, *catalogServer, error) {
	reads, err := role.NewReadService(role.ReadDependencies{Snapshots: provider, Bind: role.BindReadRepositories, Now: now})
	if err != nil {
		return nil, nil, err
	}
	readHandler, err := handler.NewRoleReadHandler(reads)
	if err != nil {
		return nil, nil, err
	}
	commands, err := role.NewManagementService(role.ManagementDependencies{Transactions: provider, Bind: func(tx domain.Transaction) role.ManagementRepositories {
		r := role.BindManagementRepositories(tx)
		r.Audit = observeRequiredAudit(r.Audit, metrics)
		return r
	}, Now: now, EventID: eventID})
	if err != nil {
		return nil, nil, err
	}
	management, err := handler.NewRoleManagementHandler(commands)
	if err != nil {
		return nil, nil, err
	}
	catalogs, err := catalog.NewService(catalog.Dependencies{Snapshots: provider, Bind: catalog.BindRepositories, Now: now})
	if err != nil {
		return nil, nil, err
	}
	catalogHandler, err := handler.NewCatalogHandler(catalogs)
	if err != nil {
		return nil, nil, err
	}
	return &roleServer{reads: readHandler, management: management}, &catalogServer{handler: catalogHandler}, nil
}

func registerRoles(s *grpc.Server, roles *roleServer, catalogs *catalogServer) {
	rolev1.RegisterRoleServiceServer(s, roles)
	permissionv1.RegisterPermissionServiceServer(s, catalogs)
	applicationv1.RegisterApplicationServiceServer(s, catalogs)
}
func (s *roleServer) ListRoles(ctx context.Context, r *rolev1.ListRolesRequest) (*rolev1.ListRolesResponse, error) {
	return s.reads.ListRoles(ctx, r)
}
func (s *roleServer) GetRole(ctx context.Context, r *rolev1.GetRoleRequest) (*rolev1.GetRoleResponse, error) {
	return s.reads.GetRole(ctx, r)
}
func (s *roleServer) CreateRole(ctx context.Context, r *rolev1.CreateRoleRequest) (*rolev1.CreateRoleResponse, error) {
	return s.management.CreateRole(ctx, r)
}
func (s *roleServer) UpdateRole(ctx context.Context, r *rolev1.UpdateRoleRequest) (*rolev1.UpdateRoleResponse, error) {
	return s.management.UpdateRole(ctx, r)
}
func (s *roleServer) ChangeRoleStatus(ctx context.Context, r *rolev1.ChangeRoleStatusRequest) (*rolev1.ChangeRoleStatusResponse, error) {
	return s.management.ChangeRoleStatus(ctx, r)
}
func (s *roleServer) ReplaceRolePermissions(ctx context.Context, r *rolev1.ReplaceRolePermissionsRequest) (*rolev1.ReplaceRolePermissionsResponse, error) {
	return s.management.ReplaceRolePermissions(ctx, r)
}
func (s *catalogServer) ListPermissions(ctx context.Context, r *permissionv1.ListPermissionsRequest) (*permissionv1.ListPermissionsResponse, error) {
	return s.handler.ListPermissions(ctx, r)
}
func (s *catalogServer) ListApplications(ctx context.Context, r *applicationv1.ListApplicationsRequest) (*applicationv1.ListApplicationsResponse, error) {
	return s.handler.ListApplications(ctx, r)
}
