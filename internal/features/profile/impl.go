package profile

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
)

type (
	ProfileService interface {
		UpdateProfile(req domain.UpdateProfileRequest) (domain.Profile, error)
	}

	profileService struct {
		ctx  context.Context
		deps Dependencies
	}
)

func NewProfileService(ctx context.Context) ProfileService {
	return NewProfileServiceWithDependencies(ctx, Dependencies{
		Transactions: database.NewTransactionProvider(database.DBGetNativePool(database.UserDbSource), database.GetDialect(database.UserDbSource), database.GetORMConfig()),
		Bind:         BindRepositories,
	})
}

type Repositories struct {
	Reader domain.ProfileReader
	Writer domain.ProfileWriter
}
type Dependencies struct {
	Transactions domain.TransactionProvider
	Bind         func(domain.Transaction) Repositories
}

func BindRepositories(tx domain.Transaction) Repositories {
	return Repositories{Reader: reader.NewProfileReaderInTransaction(tx), Writer: writer.NewProfileWriterInTransaction(tx)}
}

func NewProfileServiceWithDependencies(ctx context.Context, deps Dependencies) ProfileService {
	return &profileService{ctx: ctx, deps: deps}
}
