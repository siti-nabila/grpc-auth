package logger_test

import (
	"github.com/siti-nabila/grpc-auth/tests/pkg/logger/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestTelemetryLogging(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
