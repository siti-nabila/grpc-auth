package test_scenarios

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/tests/interceptors/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/interceptors/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{
			Name: "encodes error with request-scoped language",
			Run:  encodeRequestLanguage,
		},
		{
			Name: "does not leak language between concurrent requests",
			Run:  isolateConcurrentLanguages,
		},
		{
			Name: "does not expose unknown internal error",
			Run:  hideUnknownError,
		},
	}
}

func encodeRequestLanguage(t *testing.T) {
	encoded := invokeChain("id", auth.ErrPasswordMismatch)
	decoded, ok := grpcerror.Decode(encoded)
	if !ok {
		t.Fatal("Decode() ok = false, want true")
	}
	if decoded.Message != fixtures.IndonesianPasswordMismatch {
		t.Errorf(
			"decoded message = %q, want %q",
			decoded.Message,
			fixtures.IndonesianPasswordMismatch,
		)
	}
}

func isolateConcurrentLanguages(t *testing.T) {
	type expectation struct {
		language string
		message  string
	}
	expectations := []expectation{
		{language: "en", message: fixtures.EnglishPasswordMismatch},
		{language: "id", message: fixtures.IndonesianPasswordMismatch},
	}

	var waitGroup sync.WaitGroup
	failures := make(chan string, 200)
	for index := 0; index < 100; index++ {
		for _, expected := range expectations {
			expected := expected
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				decoded, ok := grpcerror.Decode(
					invokeChain(expected.language, auth.ErrPasswordMismatch),
				)
				if !ok || decoded.Message != expected.message {
					failures <- decoded.Message
				}
			}()
		}
	}
	waitGroup.Wait()
	close(failures)

	for message := range failures {
		t.Errorf("concurrent request message = %q", message)
	}
}

func hideUnknownError(t *testing.T) {
	encoded := invokeChain("en", errors.New("database password leaked"))
	decoded, ok := grpcerror.Decode(encoded)
	if !ok {
		t.Fatal("Decode() ok = false, want true")
	}
	if decoded.Message != fixtures.InternalMessage {
		t.Errorf("decoded message = %q, want generic internal message", decoded.Message)
	}
}

func invokeChain(language string, handlerError error) error {
	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(locale.MetadataKey, language),
	)
	_, err := interceptors.LanguageInterceptor(
		ctx,
		nil,
		nil,
		func(ctx context.Context, request any) (any, error) {
			return interceptors.ErrorInterceptor(
				ctx,
				request,
				nil,
				mocks.ErrorHandler(handlerError),
			)
		},
	)
	return err
}
