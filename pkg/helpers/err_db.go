package helpers

import (
	"github.com/lib/pq"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
)

func HandleErrorDB(err error) error {
	if err != nil {
		if err, ok := err.(*pq.Error); ok {
			switch err.Code {
			case "23505":
				return dictionary.ErrDuplicateKey
			default:
				return err
			}

		}
		return err
	}

	return nil
}
