package mocks

import (
	"context"

	"google.golang.org/grpc"
)

func ErrorHandler(err error) grpc.UnaryHandler {
	return func(context.Context, any) (any, error) {
		return nil, err
	}
}
