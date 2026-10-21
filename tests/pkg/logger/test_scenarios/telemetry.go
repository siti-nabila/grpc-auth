package test_scenarios

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/pkg/logger"
	"github.com/siti-nabila/grpc-auth/tests/pkg/logger/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"strings"
	"sync"
	"testing"
	"time"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "safe stable worst-case summary uses existing hooks and fixed payload", Run: summary},
		{Name: "formatter and writer failures return safely without recursive logging", Run: logFailures},
	}
}

func summary(t *testing.T) {
	l := logrus.New()
	w := new(mocks.Writer)
	l.AddHook(logger.NewWriterHook(w, &logrus.JSONFormatter{}))
	emit := logger.NewTelemetryWriter(l, func() time.Time { return fixtures.ServiceTime })
	v := fixtures.MaxTelemetrySnapshot()
	payload, e := json.Marshal(v)
	if e != nil || len(payload) > telemetry.MaxPayloadBytes {
		t.Fatal("unbounded numeric schema", len(payload))
	}
	if err := emit(v); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), w.Buffer.Bytes()...)
	if err := emit(v); err != nil || w.Calls != 2 {
		t.Fatal("hook count", err)
	}
	if !bytes.Equal(w.Buffer.Bytes()[:len(first)], w.Buffer.Bytes()[len(first):]) {
		t.Fatal("unstable event")
	}
	if !strings.Contains(string(first), "runtime_metrics") || strings.Contains(string(first), "private") {
		t.Fatal("unsafe event")
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := emit(v); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if w.Calls != 22 {
		t.Fatal("concurrent sink calls")
	}
}

func logFailures(t *testing.T) {
	cause := errors.New("private-log-credential-sentinel")
	for _, kind := range []string{"formatter", "writer", "panic"} {
		t.Run(kind, func(t *testing.T) {
			l := logrus.New()
			w := new(mocks.Writer)
			var f logrus.Formatter = &logrus.JSONFormatter{}
			if kind == "formatter" {
				f = mocks.Formatter{Error: cause}
			} else if kind == "panic" {
				w.Panic = true
			} else {
				w.Error = cause
			}
			l.AddHook(logger.NewFileHook(w, f))
			emit := logger.NewTelemetryWriter(l, func() time.Time { return fixtures.ServiceTime })
			err := emit(telemetry.Snapshot{})
			if err == nil || strings.Contains(err.Error(), cause.Error()) || w.Calls > 1 || w.Buffer.Len() != 0 {
				t.Fatal("unsafe failure", err)
			}
		})
	}
	if logger.NewTelemetryWriter(nil, nil)(telemetry.Snapshot{}) == nil {
		t.Fatal("missing sink false success")
	}
}
