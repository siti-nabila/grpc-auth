package helpers

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
)

func HandleError(err error) error {
	return grpcerror.Encode(err, locale.DefaultLanguage)
}

func HandleErrorContext(ctx context.Context, err error) error {
	return grpcerror.Encode(err, locale.FromContext(ctx))
}
