package helpers

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	"github.com/siti-nabila/api-contracts/pkg/locale"
)

var sharedErrorEncoder = newSharedErrorEncoder()

func HandleError(err error) error {
	return sharedErrorEncoder.Encode(err, locale.DefaultLanguage)
}

func HandleErrorContext(ctx context.Context, err error) error {
	return sharedErrorEncoder.Encode(err, locale.FromContext(ctx))
}

func newSharedErrorEncoder() *grpcerror.Encoder {
	mappings := append(
		grpcmapping.CommonCodeMappings(),
		grpcmapping.AuthCodeMappings()...,
	)
	return grpcerror.NewEncoder(mappings...)
}
