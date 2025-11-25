package helpers

import (
	"github.com/lib/pq"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
)

func HandleErrorDB(err error) error {
	if err != nil {
		if err, ok := err.(*pq.Error); ok {
			if err.Code == "23505" {
				return dictionary.ErrDuplicateKey
			}
		}
		return err
	}

	return nil
}
