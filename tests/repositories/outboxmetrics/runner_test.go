package outboxmetrics_test

import (
	"github.com/siti-nabila/grpc-auth/tests/repositories/outboxmetrics/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestOutboxMetrics(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
