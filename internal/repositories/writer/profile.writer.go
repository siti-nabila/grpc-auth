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
	profileWriter struct {
		Db  *database.DBLogger
		Tx  *sql.Tx
		ctx context.Context
	}
)

func NewProfileWriter(ctx context.Context) domain.ProfileWriter {
	dbLogger := database.NewDBLogger()
	conn := database.DBGetNativePool(database.UserDbSource)
	dbLogger.Adapter(conn)
	return &profileWriter{
		Db:  dbLogger,
		ctx: ctx,
	}

}

func (p *profileWriter) Begin() (*sql.Tx, error) {
	return p.Db.Db.Begin()
}

func (p *profileWriter) UseTransaction(tx *sql.Tx) {
	p.Db.UseTransaction(tx)
}

func (p *profileWriter) CreateProfileTx(request *domain.ProfileRequest) (err error) {
	query := fmt.Sprintf(`INSERT INTO %s(user_id) VALUES ($1)`, repositories.ProfileTable)
	_, err = p.Db.ExecTxContext(p.ctx, query, request.UserId)
	return helpers.HandleErrorDB(err)

}
