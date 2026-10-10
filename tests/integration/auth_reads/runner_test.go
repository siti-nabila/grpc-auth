package auth_reads_test

import (
	"github.com/siti-nabila/grpc-auth/tests/integration/auth_reads/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestReadSnapshots(t *testing.T) { testutils.Run(t, test_scenarios.All()) }

func TestLoginProducer(t *testing.T) { testutils.Run(t, test_scenarios.Producer()) }

func TestRegisteredServer(t *testing.T) { testutils.Run(t, test_scenarios.Registered()) }
