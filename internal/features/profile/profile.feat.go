package profile

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	userdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
)

func (p *profileService) UpdateProfile(req domain.UpdateProfileRequest) (result domain.Profile, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			result = domain.Profile{}
		}
	}()
	principal, err := sessions.GetUserSession(p.ctx)
	if err != nil {
		return result, err
	}
	if principal == nil {
		return result, common.ErrUnauthorized
	}
	tx, err := p.deps.Transactions.Begin(p.ctx)
	if err != nil {
		return result, err
	}
	owner := featurecommon.NewTransactionOwner(p.ctx, tx)
	defer owner.Finish(&err)
	if err = featurecommon.AcquireLocks(owner, userdictionary.ErrResourceBusy, featurecommon.UserLockKey(principal.UserId)); err != nil {
		return result, err
	}
	repos := p.deps.Bind(tx)
	result, err = repos.Reader.GetByUserId(principal.UserId)
	if err != nil {
		return result, err
	}
	if result.UserId != principal.UserId {
		return result, common.ErrForbidden
	}
	req.Id = result.Id
	if err = repos.Writer.Update(&req); err != nil {
		return result, err
	}
	result.Name, result.Address, result.Phone = req.Name, req.Address, req.Phone
	if err = owner.Commit(); err != nil {
		return result, err
	}
	return result, nil
}
