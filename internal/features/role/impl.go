package role

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/orm/orm"
)

type ReadService interface {
	ListRoles(context.Context, domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error)
	GetRole(context.Context, uint64) (domain.RoleDetail, error)
}
type ReadRepositories struct {
	State       domain.SessionStateReader
	Permissions domain.PermissionReader
	Roles       domain.RoleManagementReader
	Catalog     domain.CatalogReader
}
type ReadDependencies struct {
	Snapshots domain.ReadSnapshotProvider
	Bind      func(domain.Transaction) ReadRepositories
	Now       func() time.Time
}
type readService struct{ deps ReadDependencies }

func NewReadService(d ReadDependencies) (ReadService, error) {
	if d.Snapshots == nil || d.Bind == nil || d.Now == nil {
		return nil, errors.New("incomplete role read dependencies")
	}
	return &readService{d}, nil
}
func BindReadRepositories(tx domain.Transaction) ReadRepositories {
	return ReadRepositories{reader.NewSessionStateReader(tx), reader.NewPermissionReader(tx), reader.NewRoleManagementReader(tx), reader.NewCatalogReader(tx)}
}
