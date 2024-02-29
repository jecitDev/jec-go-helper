package customvalidator

import (
	"context"
	"fmt"
	"log"
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
	// valCustom.RegisterCustomTypeFunc(validateDateTimeIso8601, iso8601date.ISO8601date{})
	valCustom.RegisterValidation("daterange", validateDateRange)
	valCustom.RegisterValidation("ISO8601date", validateDateTimeIso8601)
	return &CustomValidator{Validator: valCustom}
}

func validateDateTimeIso8601(fl validator.FieldLevel) bool {
	ISO8601DateRegexString := `^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})([+-])(\d{2}):(\d{2})$`
	ISO8601DateRegex := regexp.MustCompile(ISO8601DateRegexString)
	date := reflect.ValueOf(fl.Field()).Interface()
	log.Printf("ISO8601DateRegexOK: %+v", reflect.ValueOf(fl.Field()).Interface())
	datestr := fmt.Sprintf("%+v", date)
	log.Printf("ISO8601DateRegex: %s", datestr)
	return ISO8601DateRegex.MatchString(datestr)
}

// func validateDateTimeIso8601(field reflect.Value) interface{} {

// 	if timeVal, ok := field.Interface().(iso8601date.ISO8601date); ok {
// 		ISO8601DateRegexString := `^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})([+-])(\d{2}):(\d{2})$`
// 		ISO8601DateRegex := regexp.MustCompile(ISO8601DateRegexString)

// 		if ISO8601DateRegex.MatchString(timeVal.String()) {
// 			return field
// 		}
// 	}
// 	return nil

// }

func validateTime(field reflect.Value) interface{} {

	if timeVal, ok := field.Interface().(time.Time); ok {

		minTime := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)

		if timeVal.After(minTime) {
			return field
		}
	}
	return nil
}

func validateDateRange(fl validator.FieldLevel) bool {
	return fl.Field().String() == "daterange"
}

func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.Validator.Struct(i)
}

func GrpcErrorHandler() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, err
		}
		var message []string
		// fmt.Println("error validation")
		if castedObject, ok := err.(validator.ValidationErrors); ok {

			for _, err := range castedObject {
				switch err.Tag() {
				case "required":
					// log.Printf("asd:%v", err.Type())
					// log.Printf("zxc:%v", reflect.TypeOf(iso8601date.ISO8601date{}))
					// switch err.Type() {
					// case reflect.TypeOf(iso8601date.ISO8601date{}):
					// 	message = append(message, fmt.Sprintf("%s is required or must be ISO8601 date",
					// 		err.Field()))
					// default:
					message = append(message, fmt.Sprintf("%s is required",
						err.Field()))
					// }
					// message = append(message, fmt.Sprintf("%s is required",
					// 	err.Field()))
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

// func GrpcErrorHandler(err error) error {
// 	var reportMessage interface{}
// 	// fmt.Println("error validation")
// 	if castedObject, ok := err.(validator.ValidationErrors); ok {
// 		var message []string
// 		for _, err := range castedObject {
// 			switch err.Tag() {
// 			case "required":
// 				message = append(message, fmt.Sprintf("%s is required",
// 					err.Field()))
// 			case "email":
// 				message = append(message, fmt.Sprintf("%s is not valid email",
// 					err.Field()))
// 			case "gte":
// 				message = append(message, fmt.Sprintf("%s value must be greater than %s",
// 					err.Field(), err.Param()))
// 			case "lte":
// 				message = append(message, fmt.Sprintf("%s value must be lower than %s",
// 					err.Field(), err.Param()))
// 			}
// 		}
// 		// report.Message = message
// 		reportMessage = message
// 	}

// 	return status.Errorf(codes.InvalidArgument, "%+v", reportMessage)

// }
