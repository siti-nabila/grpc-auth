package helpers

import (
	"fmt"

	errorpackage "github.com/siti-nabila/error-package"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func GrpcBadRequest(err error) error {
	errs, ok := err.(errorpackage.Errors)
	if !ok {
		return err
	}

	// base status
	st := status.New(codes.InvalidArgument, dictionary.ErrBadRequest.Error())

	// flattening all field errors
	var fields []*errdetails.BadRequest_FieldViolation
	for field, list := range errs {
		fields = append(fields, convertErrorsToViolations(field, list)...)
	}

	detail := &errdetails.BadRequest{
		FieldViolations: fields,
	}

	stWithDetails, e := st.WithDetails(detail)
	if e != nil {
		return st.Err()
	}

	return stWithDetails.Err()
}

func convertErrorsToViolations(field string, list []error) []*errdetails.BadRequest_FieldViolation {
	var result []*errdetails.BadRequest_FieldViolation

	for _, err := range list {

		switch e := err.(type) {

		case errorpackage.Errors:
			// nested: field → nested key
			for subField, subList := range e {
				nestedKey := fmt.Sprintf("%s.%s", field, subField)
				result = append(result, convertErrorsToViolations(nestedKey, subList)...)
			}

		case errorpackage.Error:
			// localized per current Language variable
			msg := e.Error()
			result = append(result, &errdetails.BadRequest_FieldViolation{
				Field:       field,
				Description: msg,
			})

		default:
			// plain error
			result = append(result, &errdetails.BadRequest_FieldViolation{
				Field:       field,
				Description: err.Error(),
			})
		}
	}

	return result
}
