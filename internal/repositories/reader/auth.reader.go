package reader

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
)

type (
	DbSource   string
	authReader struct {
		Db  *database.DBLogger
		ctx context.Context
	}
)

const (
	UserDbSource DbSource = "user"
)

func NewAuthReader(ctx context.Context) domain.AuthReader {
	dbLogger := database.NewDBLogger()
	conn := database.DBGetNativePool(database.UserDbSource)
	dbLogger.Adapter(conn)
	return &authReader{
		Db:  dbLogger,
		ctx: ctx,
	}

}

func (a *authReader) GetByEmail(email string) (result domain.AuthResponse, err error) {
	query := `SELECT id, email, password, created_at, updated_at, deleted_at FROM auth WHERE email = $1 LIMIT 1`
	err = a.Db.QueryRowContext(a.ctx, query, email).Scan(
		&result.Id,
		&result.Email,
		&result.Password,
		&result.CreatedAt,
		&result.UpdatedAt,
		&result.DeletedAt,
	)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (a *authReader) GetById(id uint64) (result domain.AuthResponse, err error) {
	query := `SELECT id, email, password, created_at, updated_at,  deleted_at FROM auth WHERE id = $1 LIMIT 1`
	err = a.Db.QueryRowContext(a.ctx, query, id).Scan(
		&result.Id,
		&result.Email,
		&result.Password,
		&result.CreatedAt,
		&result.UpdatedAt,
		&result.DeletedAt,
	)
	if err != nil {
		return result, err
	}
	return result, nil
}
