// Package debug provides request/response capture functionality for debugging.
package debug

import (
	"encoding/json"
	"strings"
	"time"
)

// DebugCapture holds configuration for request/response capture.
type DebugCapture struct {
	Enabled       bool     `json:"enabled"`
	Directory     string   `json:"directory"`
	MaxFiles      int      `json:"max_files"`
	MaxFileSize   int64    `json:"max_file_size"`
	CapturePhases []string `json:"capture_phases"`
	RedactAPIKeys bool     `json:"redact_api_keys"`
}

// Capture phase constants.
const (
	PhaseOriginal         = "original"
	PhaseNormalized       = "normalized"
	PhaseUpstreamRequest  = "upstream-request"
	PhaseUpstreamResponse = "upstream-response"
	PhaseTransformed      = "transformed"
)

// CaptureEntry represents a single captured request/response phase.
type CaptureEntry struct {
	Timestamp time.Time       `json:"timestamp"`
	Phase     string          `json:"phase"`
	Provider  string          `json:"provider"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

// RedactSensitive redacts sensitive fields (api_key, api-key, authorization) from JSON data.
// It replaces values with "[REDACTED]".
func RedactSensitive(data []byte) []byte {
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		// If we can't parse as JSON, return original data
		return data
	}

	redactObject(obj)

	redacted, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return redacted
}

// redactObject recursively redacts sensitive keys in a map.
func redactObject(obj map[string]interface{}) {
	for key, value := range obj {
		lowerKey := strings.ToLower(key)
		if lowerKey == "api_key" || lowerKey == "api-key" || lowerKey == "authorization" {
			obj[key] = "[REDACTED]"
		} else if nested, ok := value.(map[string]interface{}); ok {
			redactObject(nested)
		} else if arr, ok := value.([]interface{}); ok {
			redactArray(arr)
		}
	}
}

// redactArray recursively redacts sensitive keys in array elements.
func redactArray(arr []interface{}) {
	for i, value := range arr {
		if nested, ok := value.(map[string]interface{}); ok {
			redactObject(nested)
		} else if arr2, ok := value.([]interface{}); ok {
			redactArray(arr2)
		}
		arr[i] = value
	}
}
