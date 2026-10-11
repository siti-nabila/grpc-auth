package session

import (
	"context"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *listService) ListMySessions(ctx context.Context, r ListInput) (*ListResult, error) {
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.list(ctx, p.UserID(), r, sessionv1.SessionService_ListMySessions_FullMethodName)
}
func (s *listService) ListUserSessions(ctx context.Context, target uint64, r ListInput) (*ListResult, error) {
	return s.list(ctx, target, r, sessionv1.SessionService_ListUserSessions_FullMethodName)
}

func (s *listService) list(ctx context.Context, target uint64, input ListInput, method string) (res *ListResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = nil
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return nil, err
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = p.AuthorizeMethod(method); err != nil {
		return nil, err
	}
	input, q, err := normalizeList(target, input)
	if err != nil {
		return nil, err
	}
	tx, err := s.deps.Snapshots.BeginSnapshot(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	r := s.deps.Bind(tx)
	if r.State == nil || r.Sessions == nil || r.Permissions == nil {
		return nil, common.ErrInternalServerError
	}
	verified, err := authfeature.AuthenticateSession(ctx, p.Claims(), r.State, s.deps.Now())
	if err != nil {
		return nil, err
	}
	p, err = sessions.PrincipalFromContext(verified)
	if err != nil {
		return nil, err
	}
	if err = p.AuthorizeMethod(method); err != nil {
		return nil, err
	}
	if method == sessionv1.SessionService_ListUserSessions_FullMethodName {
		rows, e := r.Permissions.Permissions(ctx, p.UserID())
		if e != nil {
			return nil, featurecommon.DependencyError(e)
		}
		if !featurecommon.HasPermission(featurecommon.ResolvePermissions(rows), string(types.PermissionIAMSessionRead)) {
			return nil, common.ErrForbidden
		}
	} else if target != p.UserID() {
		return nil, common.ErrForbidden
	}
	page, err := r.Sessions.ListPage(ctx, q)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	res = &ListResult{Items: make([]ListItem, 0, len(page.Items)), Page: input.Page, Limit: q.Limit, Total: page.Total, HasNext: page.HasNext, HasPrev: q.After != nil}
	for _, row := range page.Items {
		if row.UserID != target {
			return nil, common.ErrInternalServerError
		}
		res.Items = append(res.Items, ListItem{Session: row, Current: row.ID == p.SessionID()})
	}
	if len(page.Items) > 0 {
		res.NextCursor, err = nextSessionCursor(q, page.Items[len(page.Items)-1])
		if err != nil {
			return nil, err
		}
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return res, nil
}
