package mocks

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
)

type SQLStep struct {
	Op      string
	SQL     string
	Args    []driver.Value
	Columns []string
	Rows    [][]driver.Value
	Error   error
	Result  driver.Result
	After   func()
}

// SQL is a per-test connector; no driver registration or external database.
type SQL struct {
	mu      sync.Mutex
	Steps   []SQLStep
	Context context.Context
	Options driver.TxOptions
	pos     int
	Failure error
}

func (s *SQL) Open() *sql.DB                                { return sql.OpenDB(s) }
func (s *SQL) Connect(context.Context) (driver.Conn, error) { return &sqlConnection{s: s}, nil }
func (s *SQL) Driver() driver.Driver                        { return sqlDriver{s: s} }
func (s *SQL) Verify() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != nil {
		return s.Failure
	}
	if s.pos != len(s.Steps) {
		return fmt.Errorf("SQL steps consumed %d, want %d", s.pos, len(s.Steps))
	}
	return nil
}
func (s *SQL) step(ctx context.Context, op, query string, args []driver.NamedValue) (SQLStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pos >= len(s.Steps) {
		s.Failure = fmt.Errorf("unexpected SQL operation %s", op)
		return SQLStep{}, s.Failure
	}
	step := s.Steps[s.pos]
	s.pos++
	values := make([]driver.Value, len(args))
	for i, v := range args {
		values[i] = v.Value
	}
	if step.Op != op || !strings.Contains(query, step.SQL) || (len(values) > 0 || len(step.Args) > 0) && !reflect.DeepEqual(values, step.Args) {
		s.Failure = fmt.Errorf("step %d: got %s %s %#v, want %s %s %#v", s.pos, op, query, values, step.Op, step.SQL, step.Args)
		return step, s.Failure
	}
	if ctx != nil && s.Context != nil && ctx != s.Context {
		s.Failure = fmt.Errorf("SQL context not forwarded")
		return step, s.Failure
	}
	return step, step.Error
}

type sqlDriver struct{ s *SQL }

func (d sqlDriver) Open(string) (driver.Conn, error) { return &sqlConnection{s: d.s}, nil }

type sqlConnection struct{ s *SQL }

func (*sqlConnection) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("unexpected prepare")
}
func (*sqlConnection) Close() error              { return nil }
func (*sqlConnection) Begin() (driver.Tx, error) { return nil, fmt.Errorf("BeginTx required") }
func (c *sqlConnection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.s.Options = opts
	_, err := c.s.step(ctx, "begin", "", nil)
	if err != nil {
		return nil, err
	}
	return &sqlTransaction{s: c.s}, nil
}
func (c *sqlConnection) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.s.step(ctx, "query", q, args)
	if err != nil {
		return nil, err
	}
	return &sqlRows{columns: step.Columns, rows: step.Rows}, nil
}
func (c *sqlConnection) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	step, err := c.s.step(ctx, "exec", q, args)
	if step.Result != nil {
		return step.Result, err
	}
	return driver.RowsAffected(1), err
}

type sqlTransaction struct{ s *SQL }

func (t *sqlTransaction) Commit() error { _, err := t.s.step(nil, "commit", "", nil); return err }
func (t *sqlTransaction) Rollback() error {
	step, err := t.s.step(nil, "rollback", "", nil)
	if step.After != nil {
		step.After()
	}
	return err
}

type sqlRows struct {
	columns []string
	rows    [][]driver.Value
	pos     int
}

func (r *sqlRows) Columns() []string { return r.columns }
func (*sqlRows) Close() error        { return nil }
func (r *sqlRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}
