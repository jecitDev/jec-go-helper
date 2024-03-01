package stringtools

import (
	"fmt"
	"reflect"
	"strings"
)

func RightValue(input string, length int) string {
	if len(input) <= length {
		return input // Return the entire input string if its length is less than or equal to the specified length.
	}
	startIndex := len(input) - length

	return input[startIndex:]
}

func RightValueWithFormat(format string, input string, length int) string {
	if len(input) >= length {
		return input[len(input)-length:]
	}

	return strings.Repeat(format, length-len(input)) + input
}

func ParseStringToBoolPtr(s string) *bool {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	var result bool
	if s == "true" {
		result = true
	} else if s == "false" {
		result = false
	} else {
		// Handle invalid input string that is neither "true" nor "false".
		// This could be an error return depending on your needs.
		return nil
	}
	return &result
}

func StructToString(s interface{}, delimiter string) string {
	v := reflect.ValueOf(s)
	t := v.Type()

	var sb strings.Builder

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		tagName := strings.Split(tag, ",")[0] // Get the JSON field name, ignore options like omitempty

		// Skip if the field has no json tag or is omitted
		if tagName == "" || tagName == "-" {
			continue
		}

		value := v.Field(i).Interface()

		// Special handling for *bool to avoid dereferencing nil pointers
		if v.Field(i).Kind() == reflect.Ptr && v.Field(i).IsNil() {
			continue // Skip nil pointers, or handle them differently if needed
		}

		// Append to string builder
		if sb.Len() > 0 {
			sb.WriteString(delimiter)
		}
		sb.WriteString(fmt.Sprintf("%s:%v", tagName, value))
	}

	return sb.String()
}
