package stringtools

import "strings"

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
