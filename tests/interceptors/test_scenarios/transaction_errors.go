package test_scenarios

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"google.golang.org/grpc/codes"
)

func encodeTransactionErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		code   codes.Code
		reason string
		en     string
		id     string
	}{
		{"auth busy", auth.ErrResourceBusy, codes.Aborted, "auth.resource_busy", "Resource is busy. Retry the request.", "Resource sedang digunakan. Ulangi permintaan."},
		{"user busy", user.ErrResourceBusy, codes.Aborted, "user.resource_busy", "Resource is busy. Retry the request.", "Resource sedang digunakan. Ulangi permintaan."},
		{"user idempotency", user.ErrIdempotencyConflict, codes.Aborted, "user.idempotency_conflict", "Idempotency key was already used with a different request.", "Kunci idempotensi sudah digunakan untuk permintaan yang berbeda."},
		{"dependency", common.ErrDependencyUnavailable, codes.Unavailable, "common.dependency_unavailable", "Dependency unavailable.", "Layanan pendukung tidak tersedia."},
		{"cancellation", context.Canceled, codes.Canceled, "common.request_canceled", "Request canceled.", "Permintaan dibatalkan."},
		{"deadline", context.DeadlineExceeded, codes.DeadlineExceeded, "common.deadline_exceeded", common.ErrDeadlineExceeded.Message(locale.English), common.ErrDeadlineExceeded.Message(locale.Indonesian)},
		{"legacy unavailable", common.ErrServiceUnavailable, codes.Unimplemented, "common.service_unavailable", common.ErrServiceUnavailable.Message(locale.English), common.ErrServiceUnavailable.Message(locale.Indonesian)},
		{"legacy duplicate", auth.ErrDataExists, codes.AlreadyExists, "auth.already_exists", auth.ErrDataExists.Message(locale.English), auth.ErrDataExists.Message(locale.Indonesian)},
		{"unknown", errors.New("private connection credentials"), codes.Internal, "", "Internal server error.", common.ErrInternalServerError.Message(locale.Indonesian)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("private operation: %w", tc.err)
			for _, language := range []locale.Language{locale.English, locale.Indonesian, locale.Language("id-ID"), locale.Language("fr")} {
				want := tc.en
				if language == locale.Indonesian || language == locale.Language("id-ID") {
					want = tc.id
				}
				ctx := locale.NewContext(context.Background(), string(language))
				results := []error{invokeChain(string(language), wrapped), helpers.HandleErrorContext(ctx, wrapped)}
				if language == locale.English {
					results = append(results, helpers.HandleError(wrapped))
				}
				for _, result := range results {
					decoded, ok := grpcerror.Decode(result)
					if !ok || decoded.Code != tc.code || decoded.Reason != tc.reason || decoded.Message != want {
						t.Fatalf("language %s: got %+v, want %s/%s/%s", language, decoded, tc.code, tc.reason, want)
					}
					if tc.reason != "" && helpers.HandleErrorContext(ctx, result) != result {
						t.Fatal("already encoded details changed")
					}
				}
			}
			if !errors.Is(wrapped, tc.err) {
				t.Fatal("original error cause changed")
			}
		})
	}
}
