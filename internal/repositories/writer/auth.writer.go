package writer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/siti-nabila/grpc-auth/internal/repositories"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type (
	AuthWriter struct {
		Db  *database.DBLogger
		Tx  *sql.Tx
		ctx context.Context
	}
)

func NewAuthWriter(ctx context.Context) domain.AuthWriter {
	dbLogger := database.NewDBLogger()
	conn := database.DBGetNativePool(database.UserDbSource)
	dbLogger.Adapter(conn)
	return &AuthWriter{
		Db:  dbLogger,
		ctx: ctx,
	}
}

func (a *AuthWriter) UseTransaction(tx *sql.Tx) {
	a.Db.UseTransaction(tx)
}

func (a *AuthWriter) Begin() (*sql.Tx, error) {
	return a.Db.Db.Begin()
}

func (a *AuthWriter) Register(request *domain.AuthRequest) (err error) {
	query := fmt.Sprintf(`
		INSERT INTO %s(email, password) VALUES (?, ?) RETURNING id
	`, repositories.AuthTable)

	err = a.Db.QueryRowContext(a.ctx, query, request.Email, request.Password).Scan(&request.Id)
	if err != nil {
		return err
	}

	return nil
}

func (a *AuthWriter) RegisterTx(request *domain.AuthRequest) (err error) {
	query := fmt.Sprintf(`INSERT INTO %s(email, password) VALUES ($1, $2) RETURNING id`, repositories.AuthTable)
	err = a.Db.QueryRowTxContext(a.ctx, query, request.Email, request.Password).Scan(&request.Id)
	return helpers.HandleErrorDB(err)
}
