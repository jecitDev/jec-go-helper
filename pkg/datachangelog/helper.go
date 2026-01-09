package datachangelog

import (
	"strings"
)

func normalizePatchChangeData(reqData map[string]interface{}) map[string]interface{} {
	if reqData == nil {
		return nil
	}

	out := make(map[string]interface{})

	// Copy non-"data" fields first
	for k, v := range reqData {
		if k != "data" {
			out[k] = v
		}
	}

	rawData, ok := reqData["data"]
	if !ok {
		return out
	}

	items, ok := rawData.([]interface{})
	if !ok {
		return out
	}

	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		field, fOk := m["field"].(string)
		value, vOk := m["value"]
		if fOk && vOk {
			out[field] = value
		}
	}

	return out
}

func capitalizeFirstLetter(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
