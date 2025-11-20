package writer

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
)

type (
	DbSource string

	AuthWriter struct {
		Db  *database.DBLogger
		Tx  *sql.Tx
		ctx context.Context
	}
)

const (
	UserDbSource DbSource = "user"
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
	query := `
		INSERT INTO users(email, password) VALUES (?, ?) RETURNING id
	`

	err = a.Db.QueryRowContext(a.ctx, query, request.Email, request.Password).Scan(&request.Id)
	if err != nil {
		return err
	}

	return nil
}

func (a *AuthWriter) RegisterTx(request *domain.AuthRequest) (err error) {
	query := `INSERT INTO auth(email, password) VALUES ($1, $2) RETURNING id`

	err = a.Db.QueryRowTxContext(a.ctx, query, request.Email, request.Password).Scan(&request.Id)
	if err != nil {
		return err
	}

	return nil
}
