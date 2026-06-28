package profile

import (
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
)

func (p *profileService) UpdateProfile(req domain.UpdateProfileRequest) error {
	userSession, err := sessions.GetUserSession(p.ctx)
	if err != nil {
		return err
	}

	profileData, err := p.profileReader.GetByUserId(userSession.UserId)
	if err != nil {
		return err
	}

	req.Id = profileData.Id

	tx, err := p.profileWriter.Begin()
	if err != nil {
		return err
	}
	defer common.DeferTransaction(tx, &err)

	return p.profileWriter.Update(&req)
}
