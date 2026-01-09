package datachangelog

import (
	"fmt"
	"reflect"
	"strings"
)

// DiffCalculator computes differences between before and after data
type DiffCalculator struct {
	excludedFields  []string
	sensitiveFields []string
}

// NewDiffCalculator creates a new DiffCalculator instance
func NewDiffCalculator(excludedFields, sensitiveFields []string) *DiffCalculator {
	return &DiffCalculator{
		excludedFields:  excludedFields,
		sensitiveFields: sensitiveFields,
	}
}

// CalculateDiff computes the differences between before and after maps
// Returns a slice of FieldDiff representing all changes
func (dc *DiffCalculator) CalculateDiff(before, after map[string]interface{}) []FieldDiff {
	var diffs []FieldDiff

	// Track which keys we've processed
	processedKeys := make(map[string]bool)

	// Check all keys in after (new/modified fields)
	if after != nil {
		for key, newValue := range after {
			if dc.isFieldExcluded(key) {
				continue
			}

			processedKeys[key] = true

			if before == nil {
				// Field created
				diffs = append(diffs, FieldDiff{
					FieldName: key,
					FieldType: dc.getFieldType(newValue),
					OldValue:  nil,
					NewValue:  newValue,
					Sanitized: false,
				})
			} else if oldValue, exists := before[key]; exists {
				// Field might have been modified
				if !dc.valuesEqual(oldValue, newValue) {
					diffs = append(diffs, FieldDiff{
						FieldName: key,
						FieldType: dc.getFieldType(newValue),
						OldValue:  oldValue,
						NewValue:  newValue,
						Sanitized: false,
					})
				}
			} else {
				// Field created (not in before)
				diffs = append(diffs, FieldDiff{
					FieldName: key,
					FieldType: dc.getFieldType(newValue),
					OldValue:  nil,
					NewValue:  newValue,
					Sanitized: false,
				})
			}
		}
	}

	// Check for deleted fields (in before but not in after)
	if before != nil {
		for key, oldValue := range before {
			if dc.isFieldExcluded(key) {
				continue
			}

			if !processedKeys[key] {
				diffs = append(diffs, FieldDiff{
					FieldName: key,
					FieldType: dc.getFieldType(oldValue),
					OldValue:  oldValue,
					NewValue:  nil,
					Sanitized: false,
				})
			}
		}
	}

	return diffs
}

// valuesEqual checks if two values are equal, handling various types
func (dc *DiffCalculator) valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	// Try direct comparison first
	if a == b {
		return true
	}

	// String comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)

	return aStr == bStr
}

// getFieldType returns a string representation of the field's type
func (dc *DiffCalculator) getFieldType(value interface{}) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case bool:
		return "boolean"
	case float64:
		if v == float64(int64(v)) {
			return "integer"
		}
		return "number"
	case float32:
		return "number"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return reflect.TypeOf(value).String()
	}
}

// isFieldExcluded checks if a field is in the excluded list
func (dc *DiffCalculator) isFieldExcluded(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	for _, excluded := range dc.excludedFields {
		if strings.ToLower(excluded) == lowerField {
			return true
		}
	}
	return false
}

// IsSensitiveField checks if a field is sensitive
func (dc *DiffCalculator) IsSensitiveField(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	for _, sensitive := range dc.sensitiveFields {
		if strings.ToLower(sensitive) == lowerField {
			return true
		}
	}
	return false
}

// CalculateDiffStats calculates statistics about the differences
func (dc *DiffCalculator) CalculateDiffStats(diffs []FieldDiff) DiffStats {
	stats := DiffStats{
		TotalFields:   len(diffs),
		ChangedFields: 0,
		AddedFields:   0,
		RemovedFields: 0,
	}

	for _, diff := range diffs {
		if dc.IsSensitiveField(diff.FieldName) {
			stats.SanitizedCount++
		}

		if diff.OldValue == nil && diff.NewValue != nil {
			stats.AddedFields++
		} else if diff.OldValue != nil && diff.NewValue == nil {
			stats.RemovedFields++
		} else {
			stats.ChangedFields++
		}
	}

	return stats
}

// FilterDiffs filters diffs based on provided criteria
func (dc *DiffCalculator) FilterDiffs(diffs []FieldDiff, fieldNames []string) []FieldDiff {
	if len(fieldNames) == 0 {
		return diffs
	}

	fieldMap := make(map[string]bool)
	for _, field := range fieldNames {
		fieldMap[strings.ToLower(field)] = true
	}

	var filtered []FieldDiff
	for _, diff := range diffs {
		if fieldMap[strings.ToLower(diff.FieldName)] {
			filtered = append(filtered, diff)
		}
	}

	return filtered
}

// CompactDiff creates a compact representation showing only changed fields
func (dc *DiffCalculator) CompactDiff(diffs []FieldDiff) map[string]interface{} {
	compact := make(map[string]interface{})

	for _, diff := range diffs {
		compact[diff.FieldName] = map[string]interface{}{
			"old": diff.OldValue,
			"new": diff.NewValue,
		}
	}

	return compact
}

// DiffStats represents statistics about field differences
type DiffStats struct {
	TotalFields    int
	ChangedFields  int
	AddedFields    int
	RemovedFields  int
	SanitizedCount int
}

// CalculateChangePercentage calculates the percentage of fields that changed
func (ds DiffStats) CalculateChangePercentage() float64 {
	if ds.TotalFields == 0 {
		return 0
	}
	return float64((ds.AddedFields + ds.ChangedFields + ds.RemovedFields)) / float64(ds.TotalFields) * 100
}

// HasSignificantChanges returns true if there are meaningful changes
func (ds DiffStats) HasSignificantChanges() bool {
	return ds.AddedFields > 0 || ds.ChangedFields > 0 || ds.RemovedFields > 0
}

// MergeFieldDiffs merges multiple FieldDiff slices, removing duplicates
func MergeFieldDiffs(diffs ...[]FieldDiff) []FieldDiff {
	seen := make(map[string]bool)
	var merged []FieldDiff

	for _, diffSlice := range diffs {
		for _, diff := range diffSlice {
			key := diff.FieldName
			if !seen[key] {
				seen[key] = true
				merged = append(merged, diff)
			}
		}
	}

	return merged
}

// FieldDiffComparator provides advanced comparison capabilities
type FieldDiffComparator struct {
	ignoreCase bool
	trimSpaces bool
	precision  int // For floating point comparison
}

// NewFieldDiffComparator creates a new comparator with default settings
func NewFieldDiffComparator() *FieldDiffComparator {
	return &FieldDiffComparator{
		ignoreCase: false,
		trimSpaces: false,
		precision:  2,
	}
}

// SetIgnoreCase sets whether string comparisons should be case-insensitive
func (fdc *FieldDiffComparator) SetIgnoreCase(ignore bool) {
	fdc.ignoreCase = ignore
}

// SetTrimSpaces sets whether to trim spaces before comparison
func (fdc *FieldDiffComparator) SetTrimSpaces(trim bool) {
	fdc.trimSpaces = trim
}

// SetPrecision sets the precision for floating point comparisons
func (fdc *FieldDiffComparator) SetPrecision(p int) {
	fdc.precision = p
}

// AreValuesEqual performs advanced comparison of two values
func (fdc *FieldDiffComparator) AreValuesEqual(a, b interface{}) bool {
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)

	if fdc.trimSpaces {
		aStr = strings.TrimSpace(aStr)
		bStr = strings.TrimSpace(bStr)
	}

	if fdc.ignoreCase {
		aStr = strings.ToLower(aStr)
		bStr = strings.ToLower(bStr)
	}

	return aStr == bStr
}
