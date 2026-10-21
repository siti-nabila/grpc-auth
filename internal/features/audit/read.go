package audit

import (
	"context"
	"errors"
	"time"

	auditv1 "github.com/siti-nabila/api-contracts/pb/audit/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/orm/orm"
)

type Service interface {
	ListAuditLogs(context.Context, domain.AuditLogQuery) (orm.PageData[domain.AuditLogItem], error)
}
type Repositories struct {
	State       domain.SessionStateReader
	Permissions domain.PermissionReader
	Audit       domain.AuditLogReader
}
type Dependencies struct {
	Snapshots domain.ReadSnapshotProvider
	Bind      func(domain.Transaction) Repositories
	Now       func() time.Time
}
type service struct{ deps Dependencies }

func NewService(d Dependencies) (Service, error) {
	if d.Snapshots == nil || d.Bind == nil || d.Now == nil {
		return nil, errors.New("incomplete audit read dependencies")
	}
	return &service{d}, nil
}
func BindRepositories(tx domain.Transaction) Repositories {
	return Repositories{reader.NewSessionStateReader(tx), reader.NewPermissionReader(tx), reader.NewAuditReader(tx)}
}
func (s *service) ListAuditLogs(ctx context.Context, input domain.AuditLogQuery) (page orm.PageData[domain.AuditLogItem], err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			page = orm.PageData[domain.AuditLogItem]{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return page, err
	}
	q, err := input.Normalized()
	if err != nil {
		return page, err
	}
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return page, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return page, err
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return page, err
	}
	if err = p.AuthorizeMethod(auditv1.AuditService_ListAuditLogs_FullMethodName); err != nil {
		return page, err
	}
	tx, err := s.deps.Snapshots.BeginSnapshot(ctx)
	if err != nil {
		return page, featurecommon.DependencyError(err)
	}
	if tx == nil {
		return page, common.ErrDependencyUnavailable
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	r := s.deps.Bind(tx)
	if r.State == nil || r.Permissions == nil || r.Audit == nil {
		return page, common.ErrDependencyUnavailable
	}
	if _, err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), auditv1.AuditService_ListAuditLogs_FullMethodName, types.PermissionIAMAuditRead, r.State, r.Permissions, s.deps.Now().UTC()); err != nil {
		return page, err
	}
	page, err = r.Audit.List(ctx, q)
	if err != nil {
		return page, featurecommon.DependencyError(err)
	}
	if err = owner.Commit(); err != nil {
		return page, featurecommon.DependencyError(err)
	}
	return page, nil
}
