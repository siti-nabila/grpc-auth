package configs_test

import (
	"github.com/siti-nabila/grpc-auth/tests/configs/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestConfigs(t *testing.T)            { testutils.Run(t, test_scenarios.All()) }
func TestLoginSecurity(t *testing.T)      { testutils.Run(t, test_scenarios.Login()) }
func TestCommandFingerprint(t *testing.T) { testutils.Run(t, test_scenarios.CommandFingerprint()) }
func TestTelemetryLifecycle(t *testing.T) { testutils.Run(t, test_scenarios.TelemetryLifecycle()) }
