package mocks

// AffectedFailure exercises an execution result whose row count cannot be read.
type AffectedFailure struct{ Err error }

func (r AffectedFailure) LastInsertId() (int64, error) { return 0, r.Err }
func (r AffectedFailure) RowsAffected() (int64, error) { return 0, r.Err }
