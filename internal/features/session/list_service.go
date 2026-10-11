package session

import (
	"context"
	"errors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"time"
)

type ListInput struct {
	Page, Limit int
	Field       domain.SessionSortField
	Desc        bool
	Cursor      string
}
type ListItem struct {
	Session domain.SessionSummary
	Current bool
}
type ListResult struct {
	Items            []ListItem
	Page, Limit      int
	Total            int64
	HasNext, HasPrev bool
	NextCursor       string
}
type ListService interface {
	ListMySessions(context.Context, ListInput) (*ListResult, error)
	ListUserSessions(context.Context, uint64, ListInput) (*ListResult, error)
}
type ListRepositories struct {
	State       domain.SessionStateReader
	Permissions domain.PermissionReader
	Sessions    domain.SessionLifecycleReader
}
type ListDependencies struct {
	Snapshots domain.ReadSnapshotProvider
	Bind      func(domain.Transaction) ListRepositories
	Now       func() time.Time
}
type listService struct{ deps ListDependencies }

func NewListService(d ListDependencies) (ListService, error) {
	if d.Snapshots == nil || d.Bind == nil || d.Now == nil {
		return nil, errors.New("incomplete session list dependencies")
	}
	return &listService{deps: d}, nil
}
func BindListRepositories(tx domain.Transaction) ListRepositories {
	return ListRepositories{State: reader.NewSessionStateReader(tx), Permissions: reader.NewPermissionReader(tx), Sessions: reader.NewSessionLifecycleReader(tx)}
}
