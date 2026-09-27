package test_scenarios

import (
	"errors"
	"fmt"
	"testing"

	authdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{
			Name: "maps duplicate row to shared auth registry error",
			Run:  mapDuplicateRow,
		},
		{
			Name: "maps row not found to shared auth registry error",
			Run:  mapRowNotFound,
		},
		{
			Name: "preserves unknown normalized database error",
			Run:  preserveUnknownDatabaseError,
		},
		{
			Name: "preserves non-database error",
			Run:  preserveNonDatabaseError,
		},
		{
			Name: "keeps nil error nil",
			Run:  keepNilError,
		},
	}
}

func mapDuplicateRow(t *testing.T) {
	databaseError := fmt.Errorf("create auth: %w", &normalizeerr.DBError{
		Kind: normalizeerr.KindDuplicateRow,
	})

	result := helpers.HandleDBError(databaseError)

	if !errors.Is(result, authdictionary.ErrDataExists) {
		t.Fatalf("HandleDBError() = %v, want auth registry ErrDataExists", result)
	}
}

func mapRowNotFound(t *testing.T) {
	databaseError := fmt.Errorf("read auth: %w", &normalizeerr.DBError{
		Kind: normalizeerr.KindRowNotFound,
	})

	result := helpers.HandleDBError(databaseError)

	if !errors.Is(result, authdictionary.ErrNotFound) {
		t.Fatalf("HandleDBError() = %v, want auth registry ErrNotFound", result)
	}
}

func preserveUnknownDatabaseError(t *testing.T) {
	databaseError := &normalizeerr.DBError{
		Kind: normalizeerr.KindUnknown,
		Raw:  errors.New("database unavailable"),
	}

	result := helpers.HandleDBError(databaseError)

	if result != databaseError {
		t.Fatalf("HandleDBError() = %v, want original normalized error", result)
	}
}

func preserveNonDatabaseError(t *testing.T) {
	original := errors.New("dependency failed")

	result := helpers.HandleDBError(original)

	if result != original {
		t.Fatalf("HandleDBError() = %v, want original error", result)
	}
}

func keepNilError(t *testing.T) {
	if result := helpers.HandleDBError(nil); result != nil {
		t.Fatalf("HandleDBError(nil) = %v, want nil", result)
	}
}
