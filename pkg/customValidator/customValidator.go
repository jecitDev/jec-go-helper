package customvalidator

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"time"

	"github.com/go-playground/validator/v10"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CustomValidator struct {
	Validator *validator.Validate
}

func NewCustomValidator() *CustomValidator {
	valCustom := validator.New()

	valCustom.RegisterCustomTypeFunc(validateTime, time.Time{})
	valCustom.RegisterValidation("ISO8601date", validateDateTimeIso8601)
	valCustom.RegisterValidation("daterange", validateDateRange)

	return &CustomValidator{Validator: valCustom}
}

func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.Validator.Struct(i)
}

func validateTime(field reflect.Value) interface{} {
	if timeVal, ok := field.Interface().(time.Time); ok {
		minTime := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
		if timeVal.After(minTime) {
			return field
		}
	}

	return nil
}

func validateDateTimeIso8601(fl validator.FieldLevel) bool {
	ISO8601DateRegexString := `^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})([+-])(\d{2}):(\d{2})$`
	ISO8601DateRegex := regexp.MustCompile(ISO8601DateRegexString)

	date := reflect.ValueOf(fl.Field()).Interface()
	datestr := fmt.Sprintf("%v", date)

	if len(datestr) > 0 {
		return ISO8601DateRegex.MatchString(datestr)
	}

	return true
}

func validateDateRange(fl validator.FieldLevel) bool {
	return fl.Field().String() == "daterange"
}

func GrpcErrorHandler() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)

		var message []string
		if err == nil {
			return resp, err
		}
		if castedObject, ok := err.(validator.ValidationErrors); ok {
			for _, err := range castedObject {
				switch err.Tag() {
				case "required":
					message = append(message, fmt.Sprintf("%s is required",
						err.Field()))
				case "email":
					message = append(message, fmt.Sprintf("%s is not valid email",
						err.Field()))
				case "gte":
					message = append(message, fmt.Sprintf("%s value must be greater than %s",
						err.Field(), err.Param()))
				case "lte":
					message = append(message, fmt.Sprintf("%s value must be lower than %s",
						err.Field(), err.Param()))
				case "ISO8601date":
					message = append(message, fmt.Sprintf("%s value must be ISO8601 date (YYYY-MM-DDTHH:mm:ssZ)",
						err.Field()))
				}
			}
		}
		if len(message) > 0 {
			err = status.Errorf(codes.InvalidArgument, "%+v", message)
		}

		return resp, err
	}
}
