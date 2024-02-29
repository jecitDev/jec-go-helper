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
