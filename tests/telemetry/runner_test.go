package telemetry_test

import (
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/grpc-auth/tests/telemetry/test_scenarios"
	"testing"
)

func TestTelemetry(t *testing.T)          { testutils.Run(t, test_scenarios.Core()) }
func TestMetricsInterceptor(t *testing.T) { testutils.Run(t, test_scenarios.Observer()) }
func TestRuntime(t *testing.T)            { testutils.Run(t, test_scenarios.Runtime()) }
