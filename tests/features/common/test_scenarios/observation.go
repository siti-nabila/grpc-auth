package test_scenarios

import (
	"context"
	"errors"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func Observation() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "audit decorator preserves request error and panic without observer effects", Run: auditObservation},
		{Name: "finalization uncertainty counted once without retry or altered cause", Run: finalizationObservation},
	}
}

func auditObservation(t *testing.T) {
	cause := errors.New("private-write-failure")
	for _, kind := range []string{"success", "failure", "panic", "observer panic"} {
		t.Run(kind, func(t *testing.T) {
			calls, failures := 0, 0
			req := domain.AuditRequest{TargetID: "safe-target"}
			w := common.ObserveAudit(mocks.AuditWriter{Write: func(v domain.AuditRequest) error {
				calls++
				if v.TargetID != req.TargetID {
					t.Fatal("argument changed")
				}
				if kind == "panic" {
					panic(cause)
				}
				if kind == "success" {
					return nil
				}
				return cause
			}}, func() {
				failures++
				if kind == "observer panic" {
					panic("observer")
				}
			})
			func() {
				defer func() {
					p := recover()
					if kind == "panic" && p != cause {
						t.Error("original panic lost")
					}
					if kind != "panic" && p != nil {
						t.Error("observer panic escaped")
					}
				}()
				err := w.Create(req)
				if kind == "success" && err != nil || kind != "success" && !errors.Is(err, cause) {
					t.Error("original error changed", err)
				}
			}()
			want := 1
			if kind == "success" {
				want = 0
			}
			if calls != 1 || failures != want {
				t.Fatal("duplicate/lost operation", calls, failures)
			}
		})
	}
	if common.ObserveAudit(nil, nil) != nil {
		t.Fatal("missing dependency hidden")
	}
}

func finalizationObservation(t *testing.T) {
	for _, kind := range []string{"success", "commit error", "commit panic", "cancel", "observer panic"} {
		t.Run(kind, func(t *testing.T) {
			n := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = common.WithFinalizationObserver(ctx, func() {
				n++
				if kind == "observer panic" {
					panic("observation")
				}
			})
			cause := errors.New("unknown commit outcome")
			tx := &mocks.Transaction{Context: ctx}
			if kind == "commit error" || kind == "observer panic" {
				tx.CommitError = cause
			}
			if kind == "commit panic" {
				tx.Hook = func(op string) {
					if op == "commit" {
						panic("commit")
					}
				}
			}
			owner := common.NewTransactionOwner(ctx, tx)
			err := func() (err error) {
				defer owner.Finish(&err)
				if kind == "cancel" {
					cancel()
				}
				return owner.Commit()
			}()
			want := 1
			if kind == "success" || kind == "cancel" {
				want = 0
			}
			if n != want {
				t.Fatal("finalization metric", n, want)
			}
			if (kind == "commit error" || kind == "observer panic") && !errors.Is(err, cause) {
				t.Fatal("commit cause changed", err)
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			before := len(tx.Calls)
			_ = owner.Commit()
			_ = owner.Rollback()
			if len(tx.Calls) != before || n != want {
				t.Fatal("duplicate finalization")
			}
		})
	}
}
