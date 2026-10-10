package jwt_test

import (
	"github.com/siti-nabila/grpc-auth/tests/pkg/jwt/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestAccessVerifier(t *testing.T) { testutils.Run(t, test_scenarios.All()) }
func TestAccessIssuer(t *testing.T)   { testutils.Run(t, test_scenarios.Issuer()) }
