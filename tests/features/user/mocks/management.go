package mocks

import (
	"context"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type Detail struct {
	Read func(context.Context, uint64) (domain.UserDetail, error)
}

func (r Detail) Detail(ctx context.Context, id uint64) (domain.UserDetail, error) {
	return r.Read(ctx, id)
}

type Policy struct {
	Role  func(context.Context, uint64) (uint64, error)
	Count func(context.Context, uint64) (int64, bool, error)
}

func (r Policy) RoleApplication(ctx context.Context, id uint64) (uint64, error) {
	return r.Role(ctx, id)
}
func (r Policy) AdminCount(ctx context.Context, id uint64) (int64, bool, error) {
	return r.Count(ctx, id)
}

type ProfileMutation struct {
	Write   func(context.Context, uint64, domain.UserDetail) (bool, error)
	Version func(context.Context, uint64, int64, time.Time) (bool, error)
}

func (w ProfileMutation) UpdateProfile(ctx context.Context, id uint64, r domain.UserDetail) (bool, error) {
	return w.Write(ctx, id, r)
}
func (w ProfileMutation) AdvanceProfileVersion(ctx context.Context, id uint64, v int64, at time.Time) (bool, error) {
	return w.Version(ctx, id, v, at)
}

type StatusMutation struct {
	Write func(context.Context, domain.SessionUserState, types.UserStatus, bool, time.Time) (bool, error)
}

func (w StatusMutation) ChangeStatus(ctx context.Context, r domain.SessionUserState, status types.UserStatus, invalidate bool, at time.Time) (bool, error) {
	return w.Write(ctx, r, status, invalidate, at)
}
