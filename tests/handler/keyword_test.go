package handler_test

import (
	"github.com/siti-nabila/grpc-auth/tests/handler/test_scenarios"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
)

func TestKeywordHandlers(t *testing.T) { testutils.Run(t, test_scenarios.KeywordHandlers()) }
