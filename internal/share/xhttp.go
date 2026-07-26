package share

import (
	"strings"

	"github.com/goccy/go-json"
)

// buildXhttpExtra returns the client-visible, bidirectional portion of
// Xray's SplitHTTP configuration. Server-only fields are deliberately omitted.
func buildXhttpExtra(xhttp map[string]any) map[string]any {
	if xhttp == nil {
		return nil
	}
	extra := map[string]any{}

	if mode, ok := xhttp["mode"].(string); ok && mode != "" {
		extra["mode"] = mode
	}
	if padding, ok := xhttp["xPaddingBytes"].(string); ok && padding != "" {
		extra["xPaddingBytes"] = padding
	}
	if obfs, ok := xhttp["xPaddingObfsMode"].(bool); ok && obfs {
		extra["xPaddingObfsMode"] = true
		for _, field := range []string{"xPaddingKey", "xPaddingHeader", "xPaddingPlacement", "xPaddingMethod"} {
			if value, ok := xhttp[field].(string); ok && value != "" {
				extra[field] = value
			}
		}
	}

	stringFields := []string{
		"uplinkHTTPMethod",
		"sessionIDPlacement", "sessionIDKey", "sessionIDTable", "sessionIDLength",
		"seqPlacement", "seqKey",
		"uplinkDataPlacement", "uplinkDataKey",
		"scMaxEachPostBytes", "scMinPostsIntervalMs",
	}
	coreDefaults := map[string]string{
		"scMaxEachPostBytes":   "1000000",
		"scMinPostsIntervalMs": "30",
	}
	for _, field := range stringFields {
		if value, ok := xhttp[field].(string); ok && value != "" && value != coreDefaults[field] {
			extra[field] = value
		}
	}

	for legacy, renamed := range map[string]string{
		"sessionPlacement": "sessionIDPlacement",
		"sessionKey":       "sessionIDKey",
	} {
		if _, exists := extra[renamed]; !exists {
			if value, ok := xhttp[legacy].(string); ok && value != "" {
				extra[renamed] = value
			}
		}
	}

	if value, ok := nonZeroShareValue(xhttp["uplinkChunkSize"]); ok {
		extra["uplinkChunkSize"] = value
	}
	if value, ok := xhttp["noGRPCHeader"].(bool); ok && value {
		extra["noGRPCHeader"] = value
	}
	for _, field := range []string{"xmux", "downloadSettings"} {
		if value, ok := nonEmptyShareObject(xhttp[field]); ok {
			extra[field] = value
		}
	}

	if rawHeaders, ok := xhttp["headers"].(map[string]any); ok {
		headers := map[string]any{}
		for key, value := range rawHeaders {
			if !strings.EqualFold(key, "host") {
				headers[key] = value
			}
		}
		if len(headers) > 0 {
			extra["headers"] = headers
		}
	}

	if len(extra) == 0 {
		return nil
	}
	return extra
}

func nonZeroShareValue(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		return typed, typed != ""
	case int:
		return typed, typed != 0
	case int32:
		return typed, typed != 0
	case int64:
		return typed, typed != 0
	case float32:
		return typed, typed != 0
	case float64:
		return typed, typed != 0
	default:
		return nil, false
	}
}

func nonEmptyShareObject(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, len(typed) > 0
	case map[string]string:
		return typed, len(typed) > 0
	case []any:
		return typed, len(typed) > 0
	default:
		return nil, false
	}
}

// applyXhttpExtraParams emits top-level compatibility fields plus the full
// client-visible config in the URL's extra JSON parameter.
func applyXhttpExtraParams(xhttp map[string]any, params map[string]string) {
	if xhttp == nil {
		return
	}
	applyPathAndHostParams(xhttp, params)
	if mode, ok := xhttp["mode"].(string); ok {
		params["mode"] = mode
	}
	if padding, ok := xhttp["xPaddingBytes"].(string); ok && padding != "" {
		params["x_padding_bytes"] = padding
	}
	if extra := buildXhttpExtra(xhttp); extra != nil {
		if data, err := json.Marshal(extra); err == nil {
			params["extra"] = string(data)
		}
	}
}
