package datachangelog

import (
	"crypto/sha256"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
)

// Sanitizer handles sanitization of sensitive fields
type Sanitizer struct {
	sensitiveFields map[string]bool
	redactionChar   string
}

// NewSanitizer creates a new sanitizer instance
func NewSanitizer(sensitiveFields []string) *Sanitizer {
	fieldMap := make(map[string]bool)
	for _, field := range sensitiveFields {
		fieldMap[strings.ToLower(field)] = true
	}

	return &Sanitizer{
		sensitiveFields: fieldMap,
		redactionChar:   "*",
	}
}

// IsSensitive checks if a field is marked as sensitive
func (s *Sanitizer) IsSensitive(fieldName string) bool {
	return s.sensitiveFields[strings.ToLower(fieldName)]
}

// SanitizeValue sanitizes a sensitive value
// func (s *Sanitizer) SanitizeValue(value interface{}) interface{} {
// 	if value == nil {
// 		return nil
// 	}

// 	switch v := value.(type) {
// 	case string:
// 		return s.redactString(v)
// 	case []byte:
// 		return s.redactString(string(v))
// 	default:
// 		return s.hashValue(fmt.Sprintf("%v", v))
// 	}
// }

// SanitizeValue sanitizes a sensitive value [new version]
func (s *Sanitizer) SanitizeValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		return s.redactString(v)
	case []byte:
		return s.redactString(string(v))
	default:
		return "****"
	}
}

// SanitizeMap sanitizes sensitive fields in a map
func (s *Sanitizer) SanitizeMap(data map[string]interface{}, excludedFields []string, sensitiveFields []string) map[string]interface{} {
	if data == nil {
		return nil
	}

	excluded := make(map[string]bool)
	for _, field := range excludedFields {
		excluded[strings.ToLower(field)] = true
	}

	sensitive := make(map[string]bool)
	for _, field := range sensitiveFields {
		sensitive[strings.ToLower(field)] = true
	}

	result := make(map[string]interface{})
	for key, value := range data {
		lowerKey := strings.ToLower(key)

		// Skip excluded fields
		if excluded[lowerKey] {
			log.Println("Skipping sanitization for excluded field:", key, "value:", value)
			continue
		}

		// Sensitive fields → mask value
		if sensitive[lowerKey] {
			result[key] = s.SanitizeValue(value)
			continue
		}

		// Recurse for nested objects
		switch v := value.(type) {
		case map[string]interface{}:
			result[key] = s.SanitizeMap(v, excludedFields, sensitiveFields)
		case []interface{}:
			result[key] = s.sanitizeSlice(v, excludedFields, sensitiveFields)
		default:
			result[key] = value
		}
	}

	return result
}

func (s *Sanitizer) sanitizeSlice(arr []interface{}, excludedFields []string, sensitiveFields []string) []interface{} {
	out := make([]interface{}, len(arr))

	for i, v := range arr {
		switch val := v.(type) {
		case map[string]interface{}:
			out[i] = s.SanitizeMap(val, excludedFields, sensitiveFields)
		default:
			out[i] = val
		}
	}

	return out
}

// redactString redacts a string by showing only the first and last few characters
// func (s *Sanitizer) redactString(value string) string {
// 	if len(value) <= 4 {
// 		return strings.Repeat(s.redactionChar, len(value))
// 	}

// 	// Show first 2 and last 2 characters
// 	prefix := value[:2]
// 	suffix := value[len(value)-2:]
// 	middle := strings.Repeat(s.redactionChar, len(value)-4)

// 	return prefix + middle + suffix
// }

// redactString redacts a string using 80:20 masking or full masking for short strings [new version]
func (s *Sanitizer) redactString(value string) string {
	n := len(value)
	if n == 0 {
		return value
	}

	// Rule 1: short values → fully masked
	if n <= 4 {
		return strings.Repeat(string(s.redactionChar), n)
	}

	// Rule 2: 80:20 masking
	visible := int(math.Ceil(float64(n) * 0.2))
	if visible < 2 {
		visible = 2
	}

	// Split visible chars between start & end
	prefixLen := visible / 2
	suffixLen := visible - prefixLen

	prefix := value[:prefixLen]
	suffix := value[n-suffixLen:]

	middle := strings.Repeat(
		string(s.redactionChar),
		n-prefixLen-suffixLen,
	)

	return prefix + middle + suffix
}

// hashValue creates a hash of a value for comparison without exposing the value
func (s *Sanitizer) hashValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", hash)
}

// MaskEmail masks an email address
func (s *Sanitizer) MaskEmail(email string) string {
	re := regexp.MustCompile(`^(.{2})[^@]*(@.*)$`)
	return re.ReplaceAllString(email, "$1****$2")
}

// MaskPhoneNumber masks a phone number
func (s *Sanitizer) MaskPhoneNumber(phone string) string {
	// Keep first 3 and last 2 digits
	if len(phone) <= 5 {
		return strings.Repeat(s.redactionChar, len(phone))
	}

	prefix := phone[:3]
	suffix := phone[len(phone)-2:]
	middle := strings.Repeat(s.redactionChar, len(phone)-5)

	return prefix + middle + suffix
}

// MaskSSN masks a social security number
func (s *Sanitizer) MaskSSN(ssn string) string {
	// Keep last 4 digits
	clean := strings.ReplaceAll(strings.ReplaceAll(ssn, "-", ""), " ", "")
	if len(clean) <= 4 {
		return strings.Repeat(s.redactionChar, len(clean))
	}

	suffix := clean[len(clean)-4:]
	return strings.Repeat(s.redactionChar, len(clean)-4) + suffix
}

// RedactionLevel defines the level of redaction to apply
type RedactionLevel int

const (
	// RedactionLevelNone means no redaction
	RedactionLevelNone RedactionLevel = iota
	// RedactionLevelPartial means partial redaction (e.g., show first and last chars)
	RedactionLevelPartial
	// RedactionLevelFull means complete redaction
	RedactionLevelFull
	// RedactionLevelHash means hash the value
	RedactionLevelHash
)

// RedactionConfig defines redaction rules for specific fields
type RedactionConfig struct {
	FieldName string
	Level     RedactionLevel
	Pattern   *regexp.Regexp // Optional: regex pattern to match field types
}

// AdvancedSanitizer provides advanced sanitization with custom rules
type AdvancedSanitizer struct {
	rules map[string]RedactionConfig
}

// NewAdvancedSanitizer creates a new advanced sanitizer
func NewAdvancedSanitizer() *AdvancedSanitizer {
	return &AdvancedSanitizer{
		rules: make(map[string]RedactionConfig),
	}
}

// AddRule adds a redaction rule for a field
func (as *AdvancedSanitizer) AddRule(fieldName string, level RedactionLevel) {
	as.rules[strings.ToLower(fieldName)] = RedactionConfig{
		FieldName: fieldName,
		Level:     level,
	}
}

// SanitizeField sanitizes a field value according to configured rules
func (as *AdvancedSanitizer) SanitizeField(fieldName string, value interface{}) interface{} {
	rule, exists := as.rules[strings.ToLower(fieldName)]
	if !exists {
		return value
	}

	strValue := fmt.Sprintf("%v", value)

	switch rule.Level {
	case RedactionLevelNone:
		return value
	case RedactionLevelPartial:
		return as.partialRedaction(strValue)
	case RedactionLevelFull:
		return strings.Repeat("*", len(strValue))
	case RedactionLevelHash:
		hash := sha256.Sum256([]byte(strValue))
		return fmt.Sprintf("sha256:%x", hash)
	default:
		return value
	}
}

// partialRedaction performs partial redaction of a string
func (as *AdvancedSanitizer) partialRedaction(value string) string {
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}

	prefix := value[:2]
	suffix := value[len(value)-2:]
	middle := strings.Repeat("*", len(value)-4)

	return prefix + middle + suffix
}

// SanitizeFieldDiff sanitizes the values in a FieldDiff
func (s *Sanitizer) SanitizeFieldDiff(diff *FieldDiff) *FieldDiff {
	if diff == nil {
		return nil
	}

	sanitized := *diff

	if s.IsSensitive(diff.FieldName) {
		sanitized.OldValue = s.SanitizeValue(diff.OldValue)
		sanitized.NewValue = s.SanitizeValue(diff.NewValue)
		sanitized.Sanitized = true
	}

	return &sanitized
}

// SanitizeFieldDiffs sanitizes a slice of FieldDiff
func (s *Sanitizer) SanitizeFieldDiffs(diffs []FieldDiff) []FieldDiff {
	if diffs == nil {
		return nil
	}

	result := make([]FieldDiff, 0, len(diffs))
	for _, diff := range diffs {
		sanitized := s.SanitizeFieldDiff(&diff)
		if sanitized != nil {
			result = append(result, *sanitized)
		}
	}

	return result
}

// IsFieldLikeEmail checks if a field name or value looks like an email
func IsFieldLikeEmail(fieldName string) bool {
	lowerName := strings.ToLower(fieldName)
	return strings.Contains(lowerName, "email") ||
		strings.Contains(lowerName, "mail") ||
		strings.Contains(lowerName, "address")
}

// IsFieldLikePhoneNumber checks if a field name or value looks like a phone number
func IsFieldLikePhoneNumber(fieldName string) bool {
	lowerName := strings.ToLower(fieldName)
	return strings.Contains(lowerName, "phone") ||
		strings.Contains(lowerName, "mobile") ||
		strings.Contains(lowerName, "telephone") ||
		strings.Contains(lowerName, "tel")
}

// IsFieldLikeSSN checks if a field name looks like an SSN
func IsFieldLikeSSN(fieldName string) bool {
	lowerName := strings.ToLower(fieldName)
	return strings.Contains(lowerName, "ssn") ||
		strings.Contains(lowerName, "social") ||
		strings.Contains(lowerName, "security")
}

// IsFieldLikePassword checks if a field name looks like a password
func IsFieldLikePassword(fieldName string) bool {
	lowerName := strings.ToLower(fieldName)
	return strings.Contains(lowerName, "password") ||
		strings.Contains(lowerName, "passwd") ||
		strings.Contains(lowerName, "pwd") ||
		strings.Contains(lowerName, "secret")
}

// AutoDetectSensitiveFields automatically detects sensitive fields by name
func AutoDetectSensitiveFields(fieldNames []string) []string {
	var result []string

	for _, name := range fieldNames {
		if IsFieldLikeEmail(name) ||
			IsFieldLikePhoneNumber(name) ||
			IsFieldLikeSSN(name) ||
			IsFieldLikePassword(name) {
			result = append(result, name)
		}
	}

	return result
}
