package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/siti-nabila/grpc-auth/pkg/logger"
)

type (
	DBLogger struct {
		Db *sql.DB
		Tx *sql.Tx
	}
	DBRow struct {
		row         *sql.Row
		query       string
		interpolate string
		args        []any
		start       time.Time
	}
)

func NewDBLogger() *DBLogger {
	return &DBLogger{}
}

func (d *DBLogger) Adapter(dbConn *sql.DB) {
	d.Db = dbConn
}

func (d *DBLogger) UseTransaction(tx *sql.Tx) {

	d.Tx = tx
}

func deferLog(query string, err *error, start time.Time) {

	if *err != nil {
		logger.Logs.DB.Errorf("[SQL-ERROR] %v | err: %v | time: %v", query, *err, time.Since(start))
	} else {
		logger.Logs.DB.Infof("[SQL] %v | time: %v", query, time.Since(start))
	}
}

func (d *DBLogger) ExecContext(ctx context.Context, query string, args ...any) (res sql.Result, err error) {
	start := time.Now()
	interpolated := interpolate(query, args...)
	defer deferLog(interpolated, &err, start)

	res, err = d.Db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil

}

func (d *DBLogger) QueryContext(ctx context.Context, query string, args ...any) (res *sql.Rows, err error) {
	start := time.Now()
	interpolated := interpolate(query, args...)

	defer deferLog(interpolated, &err, start)
	res, err = d.Db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (d *DBLogger) QueryRowContext(ctx context.Context, query string, args ...any) *DBRow {
	start := time.Now()
	interpolated := interpolate(query, args...)
	row := d.Db.QueryRowContext(ctx, query, args...)

	return &DBRow{
		row:         row,
		query:       query,
		args:        args,
		start:       start,
		interpolate: interpolated,
	}
}

func (d *DBLogger) ExecTxContext(ctx context.Context, query string, args ...any) (res sql.Result, err error) {
	start := time.Now()
	interpolated := interpolate(query, args...)
	defer deferLog(interpolated, &err, start)

	res, err = d.Tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil

}
func (d *DBLogger) QueryTxContext(ctx context.Context, query string, args ...any) (res *sql.Rows, err error) {
	start := time.Now()
	interpolated := interpolate(query, args...)

	defer deferLog(interpolated, &err, start)
	res, err = d.Tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (d *DBLogger) QueryRowTxContext(ctx context.Context, query string, args ...any) *DBRow {
	start := time.Now()
	interpolated := interpolate(query, args...)
	row := d.Tx.QueryRowContext(ctx, query, args...)

	return &DBRow{
		row:         row,
		query:       query,
		args:        args,
		start:       start,
		interpolate: interpolated,
	}
}

func (r *DBRow) Scan(dest ...any) (err error) {
	defer deferLog(r.interpolate, &err, r.start)
	err = r.row.Scan(dest...) // <-- eksekusi sebenarnya
	if err != nil {
		return err
	}
	return nil
}
