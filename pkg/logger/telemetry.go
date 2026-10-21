package logger

import (
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"sync"
	"time"
)

// NewTelemetryWriter snapshots the startup hook set. It returns hook errors
// instead of logrus.Entry.Log's raw-error stderr fallback. No global hooks change.
func NewTelemetryWriter(l *logrus.Logger, now func() time.Time) func(telemetry.Snapshot) error {
	var mu sync.Mutex
	var hooks []logrus.Hook
	if l != nil {
		hooks = append(hooks, l.Hooks[logrus.InfoLevel]...)
	}
	return func(v telemetry.Snapshot) (err error) {
		defer func() {
			if recover() != nil {
				err = errors.New("metrics log write failed")
			}
		}()
		if l == nil || now == nil || len(hooks) == 0 {
			return errors.New("metrics log sink unavailable")
		}
		payload, err := json.Marshal(v)
		if err != nil || len(payload) > telemetry.MaxPayloadBytes {
			return errors.New("metrics payload invalid")
		}
		mu.Lock()
		defer mu.Unlock()
		entry := &logrus.Entry{Logger: l, Time: now(), Level: logrus.InfoLevel, Message: "runtime_metrics", Data: logrus.Fields{"snapshot": string(payload)}}
		for _, hook := range hooks {
			if err = hook.Fire(entry); err != nil {
				return errors.New("metrics log write failed")
			}
		}
		return nil
	}
}
