package profile

import (
	"context"
	"errors"
	"math"
	"strconv"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *sessionProfileService) UpdateProfile(ctx context.Context, req domain.ProfileUpdate) (res *UpdateResult, err error) {
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
	if _, err = serviceauth.CallerFromContext(ctx); err != nil {
		return nil, err
	}
	principal, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	fields, err := selectedFields(req)
	if err != nil {
		return nil, err
	}
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, featurecommon.UserLockKey(principal.UserID()), "auth:session:"+principal.SessionID()); err != nil {
		if errors.Is(err, auth.ErrResourceBusy) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	repos := s.deps.Bind(tx)
	if repos.State == nil || repos.Reader == nil || repos.Writer == nil || repos.Version == nil || repos.Audit == nil || repos.Outbox == nil {
		return nil, common.ErrInternalServerError
	}
	principal, current, err := s.lockedProfile(ctx, repos, principal)
	if err != nil {
		return nil, err
	}
	updated := replaceFields(current, req, fields)
	res = &UpdateResult{Profile: updated, Version: principal.UserVersion(), Changed: updated != current}
	if err = s.persistUpdate(ctx, repos, principal, current, res); err != nil {
		return nil, err
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return res, nil
}

func (s *sessionProfileService) persistUpdate(ctx context.Context, repos SessionRepositories, principal sessions.Principal, before domain.Profile, result *UpdateResult) error {
	actor, err := domain.ActorForVerifiedUser(principal.UserID())
	if err != nil {
		return err
	}
	action := types.AuditNoOp
	if result.Changed {
		if err = persistProfileFields(repos, principal, before, result); err != nil {
			return err
		}
		action = types.AuditUserProfileUpdated
	}
	target := strconv.FormatUint(principal.UserID(), 10)
	now := s.deps.Now().UTC()
	after := profileSnapshot(result.Profile, result.Version)
	after.NoOp = !result.Changed
	if err = repos.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetUser, TargetID: target, Before: profileSnapshot(before, principal.UserVersion()), After: after, OccurredAt: now}); err != nil {
		return featurecommon.DependencyError(err)
	}
	if !result.Changed {
		return nil
	}
	id, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	userID := principal.UserID()
	return featurecommon.DependencyError(repos.Outbox.Create(featurecommon.UserEvent(ctx, id, types.EventUserProfileUpdated, userID, result.Version, &userID, types.UserActive, now)))
}

func profileSnapshot(p domain.Profile, version int64) *domain.SafeSnapshot {
	return &domain.SafeSnapshot{Version: version, Name: &p.Name, Address: &p.Address, Phone: &p.Phone}
}

func (s *sessionProfileService) lockedProfile(ctx context.Context, repos SessionRepositories, principal sessions.Principal) (sessions.Principal, domain.Profile, error) {
	verified, err := authfeature.AuthenticateSession(ctx, principal.Claims(), repos.State, s.deps.Now())
	if err != nil {
		return sessions.Principal{}, domain.Profile{}, err
	}
	principal, err = sessions.PrincipalFromContext(verified)
	if err != nil {
		return sessions.Principal{}, domain.Profile{}, err
	}
	if err = principal.AuthorizeMethod(profilev1.ProfileService_UpdateProfile_FullMethodName); err != nil {
		return sessions.Principal{}, domain.Profile{}, err
	}
	current, err := repos.Reader.GetByUserId(principal.UserID())
	if err != nil {
		return sessions.Principal{}, domain.Profile{}, featurecommon.DependencyError(err)
	}
	if current.UserId != principal.UserID() || current.Id == 0 {
		return sessions.Principal{}, domain.Profile{}, common.ErrForbidden
	}
	return principal, current, nil
}

func persistProfileFields(repos SessionRepositories, principal sessions.Principal, before domain.Profile, result *UpdateResult) error {
	if result.Version == math.MaxInt64 {
		return common.ErrInternalServerError
	}
	result.Version++
	if err := repos.Writer.Update(&domain.UpdateProfileRequest{Id: before.Id, Name: result.Profile.Name, Address: result.Profile.Address, Phone: result.Profile.Phone}); err != nil {
		return featurecommon.DependencyError(err)
	}
	if err := repos.Version.SetVersion(principal.UserID(), result.Version); err != nil {
		return featurecommon.DependencyError(err)
	}
	return nil
}
