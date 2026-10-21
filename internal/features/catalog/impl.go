package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/orm/orm"
)

type Service interface {
	ListPermissions(context.Context, domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error)
	ListApplications(context.Context, domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error)
}
type Repositories struct {
	State       domain.SessionStateReader
	Permissions domain.PermissionReader
	Catalog     domain.CatalogReader
}
type Dependencies struct {
	Snapshots domain.ReadSnapshotProvider
	Bind      func(domain.Transaction) Repositories
	Now       func() time.Time
}
type service struct{ deps Dependencies }

func NewService(d Dependencies) (Service, error) {
	if d.Snapshots == nil || d.Bind == nil || d.Now == nil {
		return nil, errors.New("incomplete catalog read dependencies")
	}
	return &service{d}, nil
}
func BindRepositories(tx domain.Transaction) Repositories {
	return Repositories{reader.NewSessionStateReader(tx), reader.NewPermissionReader(tx), reader.NewCatalogReader(tx)}
}
