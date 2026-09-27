package test_scenarios

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/tests/interceptors/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/interceptors/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{
			Name: "encodes error with request-scoped language",
			Run:  encodeRequestLanguage,
		},
		{
			Name: "encodes business error with arbitrary request language",
			Run:  encodeArbitraryRequestLanguage,
		},
		{
			Name: "encodes common deadline exceeded error",
			Run:  encodeCommonDeadlineExceeded,
		},
		{
			Name: "encodes multiple field errors with arbitrary request language",
			Run:  encodeArbitraryLanguageFieldErrors,
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
	grpcStatus, ok := status.FromError(encoded)
	if !ok {
		t.Fatal("status.FromError() ok = false, want true")
	}
	if grpcStatus.Code() != codes.InvalidArgument {
		t.Errorf("grpc status code = %s, want %s", grpcStatus.Code(), codes.InvalidArgument)
	}

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

func encodeArbitraryRequestLanguage(t *testing.T) {
	encoded := invokeChain("zh-CN", auth.ErrDataExists)
	grpcStatus, ok := status.FromError(encoded)
	if !ok {
		t.Fatal("status.FromError() ok = false, want true")
	}
	if grpcStatus.Code() != codes.AlreadyExists {
		t.Errorf("grpc status code = %s, want %s", grpcStatus.Code(), codes.AlreadyExists)
	}

	decoded, ok := grpcerror.Decode(encoded)
	if !ok {
		t.Fatal("Decode() ok = false, want true")
	}
	if decoded.Message != fixtures.ChineseAlreadyExists {
		t.Errorf(
			"decoded message = %q, want %q",
			decoded.Message,
			fixtures.ChineseAlreadyExists,
		)
	}
}

func encodeCommonDeadlineExceeded(t *testing.T) {
	encoded := invokeChain("id", common.ErrDeadlineExceeded)
	grpcStatus, ok := status.FromError(encoded)
	if !ok {
		t.Fatal("status.FromError() ok = false, want true")
	}
	if grpcStatus.Code() != codes.DeadlineExceeded {
		t.Errorf(
			"grpc status code = %s, want %s",
			grpcStatus.Code(),
			codes.DeadlineExceeded,
		)
	}
	if grpcStatus.Message() != common.ErrDeadlineExceeded.Message(locale.Indonesian) {
		t.Errorf(
			"grpc status message = %q, want %q",
			grpcStatus.Message(),
			common.ErrDeadlineExceeded.Message(locale.Indonesian),
		)
	}
}

func encodeArbitraryLanguageFieldErrors(t *testing.T) {
	fieldErrors := dictionary.FieldErrors{}
	fieldErrors.Add("email", auth.ErrRequired)
	fieldErrors.Add("email", auth.ErrMinLength(6))

	encoded := invokeChain("zh-CN", fieldErrors)
	decoded, ok := grpcerror.Decode(encoded)
	if !ok {
		t.Fatal("Decode() ok = false, want true")
	}
	want := []string{
		fixtures.ChineseRequired,
		fixtures.ChineseMinimumLength,
	}
	got := decoded.FieldErrors["email"]
	if len(got) != len(want) {
		t.Fatalf("field errors = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("field error[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func isolateConcurrentLanguages(t *testing.T) {
	type expectation struct {
		language     string
		handlerError error
		message      string
	}
	expectations := []expectation{
		{
			language:     "en",
			handlerError: auth.ErrPasswordMismatch,
			message:      fixtures.EnglishPasswordMismatch,
		},
		{
			language:     "id",
			handlerError: auth.ErrPasswordMismatch,
			message:      fixtures.IndonesianPasswordMismatch,
		},
		{
			language:     "zh-CN",
			handlerError: auth.ErrDataExists,
			message:      fixtures.ChineseAlreadyExists,
		},
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
					invokeChain(expected.language, expected.handlerError),
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
	mappings := append(
		grpcmapping.CommonCodeMappings(),
		grpcmapping.AuthCodeMappings()...,
	)
	errorEncoder := grpcerror.NewEncoder(mappings...)
	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(locale.MetadataKey, language),
	)
	_, err := interceptors.LanguageInterceptor(
		ctx,
		nil,
		nil,
		func(ctx context.Context, request any) (any, error) {
			return interceptors.NewErrorInterceptor(errorEncoder)(
				ctx,
				request,
				nil,
				mocks.ErrorHandler(handlerError),
			)
		},
	)
	return err
}
