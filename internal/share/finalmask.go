package share

import (
	"fmt"
	"slices"

	"github.com/goccy/go-json"
)

var kcpMaskToHeaderType = map[string]string{
	"dns":       "dns",
	"dtls":      "dtls",
	"srtp":      "srtp",
	"utp":       "utp",
	"wechat":    "wechat-video",
	"wireguard": "wireguard",
}

var validFinalMaskUDPTypes = map[string]struct{}{
	"salamander":    {},
	"mkcp-legacy":   {},
	"xdns":          {},
	"xicmp":         {},
	"noise":         {},
	"header-custom": {},
	"realm":         {},
}

var validFinalMaskTCPTypes = map[string]struct{}{
	"header-custom": {},
	"fragment":      {},
	"sudoku":        {},
	"xmc":           {},
}

func applyKcpShareParams(stream map[string]any, params map[string]string) {
	extractKcpShareFields(stream).applyToParams(params)
}

func applyKcpShareObj(stream map[string]any, obj map[string]any) {
	extractKcpShareFields(stream).applyToObj(obj)
}

type kcpShareFields struct {
	headerType string
	seed       string
	mtu        int
	tti        int
}

func (f kcpShareFields) applyToParams(params map[string]string) {
	if f.headerType != "" && f.headerType != "none" {
		params["headerType"] = f.headerType
	}
	setStringParam(params, "seed", f.seed)
	setIntParam(params, "mtu", f.mtu)
	setIntParam(params, "tti", f.tti)
}

func (f kcpShareFields) applyToObj(obj map[string]any) {
	if f.headerType != "" && f.headerType != "none" {
		obj["type"] = f.headerType
	}
	setStringField(obj, "path", f.seed)
	setIntField(obj, "mtu", f.mtu)
	setIntField(obj, "tti", f.tti)
}

func extractKcpShareFields(stream map[string]any) kcpShareFields {
	fields := kcpShareFields{headerType: "none"}

	if kcp, ok := stream["kcpSettings"].(map[string]any); ok {
		if header, ok := kcp["header"].(map[string]any); ok {
			if value, ok := header["type"].(string); ok && value != "" {
				fields.headerType = value
			}
		}
		if value, ok := kcp["seed"].(string); ok && value != "" {
			fields.seed = value
		}
		if value, ok := readPositiveInt(kcp["mtu"]); ok {
			fields.mtu = value
		}
		if value, ok := readPositiveInt(kcp["tti"]); ok {
			fields.tti = value
		}
	}

	for _, rawMask := range normalizedFinalMaskUDPMasks(stream["finalmask"]) {
		mask, _ := rawMask.(map[string]any)
		if mask == nil {
			continue
		}
		if maskType, _ := mask["type"].(string); maskType != "mkcp-legacy" {
			continue
		}

		settings, _ := mask["settings"].(map[string]any)
		header, _ := settings["header"].(string)
		value, _ := settings["value"].(string)
		if header == "" {
			fields.seed = value
			continue
		}
		if mapped, ok := kcpMaskToHeaderType[header]; ok {
			fields.headerType = mapped
		}
	}

	return fields
}

func readPositiveInt(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, number > 0
	case int32:
		return int(number), number > 0
	case int64:
		return int(number), number > 0
	case float32:
		parsed := int(number)
		return parsed, parsed > 0
	case float64:
		parsed := int(number)
		return parsed, parsed > 0
	default:
		return 0, false
	}
}

func setStringParam(params map[string]string, key, value string) {
	if value == "" {
		delete(params, key)
		return
	}
	params[key] = value
}

func setIntParam(params map[string]string, key string, value int) {
	if value <= 0 {
		delete(params, key)
		return
	}
	params[key] = fmt.Sprintf("%d", value)
}

func setStringField(obj map[string]any, key, value string) {
	if value == "" {
		delete(obj, key)
		return
	}
	obj[key] = value
}

func setIntField(obj map[string]any, key string, value int) {
	if value <= 0 {
		delete(obj, key)
		return
	}
	obj[key] = value
}

func applyFinalMaskParams(finalmask map[string]any, params map[string]string) {
	if value, ok := marshalFinalMask(finalmask); ok {
		params["fm"] = value
	}
}

func applyFinalMaskObj(finalmask map[string]any, obj map[string]any) {
	if value, ok := marshalFinalMask(finalmask); ok {
		obj["fm"] = value
	}
}

func marshalFinalMask(finalmask map[string]any) (string, bool) {
	normalized := normalizeFinalMask(finalmask)
	if !hasFinalMaskContent(normalized) {
		return "", false
	}
	data, err := json.Marshal(normalized)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return "", false
	}
	return string(data), true
}

func normalizeFinalMask(finalmask map[string]any) map[string]any {
	tcpMasks := normalizedFinalMaskTCPMasks(finalmask)
	udpMasks := normalizedFinalMaskUDPMasks(finalmask)
	quicParams, hasQuicParams := finalmask["quicParams"].(map[string]any)
	if len(tcpMasks) == 0 && len(udpMasks) == 0 && !hasQuicParams {
		return nil
	}

	result := map[string]any{}
	if len(tcpMasks) > 0 {
		result["tcp"] = tcpMasks
	}
	if len(udpMasks) > 0 {
		result["udp"] = udpMasks
	}
	if hasQuicParams && len(quicParams) > 0 {
		result["quicParams"] = quicParams
	}
	return result
}

func normalizedFinalMaskTCPMasks(value any) []any {
	finalmask, _ := value.(map[string]any)
	if finalmask == nil {
		return nil
	}
	rawMasks, _ := finalmask["tcp"].([]any)
	normalized := make([]any, 0, len(rawMasks))
	for _, rawMask := range rawMasks {
		mask, _ := rawMask.(map[string]any)
		if mask == nil {
			continue
		}
		maskType, _ := mask["type"].(string)
		if _, ok := validFinalMaskTCPTypes[maskType]; !ok || maskType == "" {
			continue
		}
		normalizedMask := map[string]any{"type": maskType}
		if settings, ok := mask["settings"].(map[string]any); ok && len(settings) > 0 {
			normalizedMask["settings"] = settings
		}
		normalized = append(normalized, normalizedMask)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizedFinalMaskUDPMasks(value any) []any {
	finalmask, _ := value.(map[string]any)
	if finalmask == nil {
		return nil
	}
	rawMasks, _ := finalmask["udp"].([]any)
	normalized := make([]any, 0, len(rawMasks))
	for _, rawMask := range rawMasks {
		mask, _ := rawMask.(map[string]any)
		if mask == nil {
			continue
		}
		maskType, _ := mask["type"].(string)
		if _, ok := validFinalMaskUDPTypes[maskType]; !ok || maskType == "" {
			continue
		}
		normalizedMask := map[string]any{"type": maskType}
		if settings, ok := mask["settings"].(map[string]any); ok && len(settings) > 0 {
			normalizedMask["settings"] = settings
		}
		normalized = append(normalized, normalizedMask)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func hasFinalMaskContent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return typed != ""
	case map[string]any:
		for _, item := range typed {
			if hasFinalMaskContent(item) {
				return true
			}
		}
		return false
	case []any:
		return slices.ContainsFunc(typed, hasFinalMaskContent)
	default:
		return true
	}
}
