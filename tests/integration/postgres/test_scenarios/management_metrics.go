//go:build postgres

package test_scenarios

import (
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"testing"
)

// These assertions run alongside each existing command's exact atomicity checks.
func assertManagementFailureMetrics(t *testing.T, h *managementHarness, stage string) {
	t.Helper()
	v := h.foundation.Login.Metrics.Snapshot(h.now())
	var required, finalization, attempt uint64
	if stage == "audit" || stage == "audit failure" {
		required = 1
	}
	if stage == "commit" {
		finalization = 1
	}
	if stage == "audit failure" {
		attempt = 1
	}
	if v.Audit[telemetry.RequiredWrite] != required || v.Audit[telemetry.FinalizationUnknown] != finalization || v.Audit[telemetry.SecurityAttempt] != attempt || v.Audit[telemetry.LoginAttempt] != 0 {
		t.Fatal("production bind observation counts", stage, v.Audit)
	}
}
