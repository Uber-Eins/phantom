package share

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"maps"
	"net/url"
	"strings"

	"github.com/goccy/go-json"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
)

func (s *LinkService) resolveInboundAddress(inbound *model.Inbound) string {
	if listen := inbound.Listen; listen != "" && listen[0] != '@' && listen[0] != '/' && isRoutableHost(listen) {
		return listen
	}
	return s.address
}

func unmarshalStreamSettings(streamSettings string) map[string]any {
	var stream map[string]any
	_ = json.Unmarshal([]byte(streamSettings), &stream)
	return stream
}

func applyPathAndHostParams(settings map[string]any, params map[string]string) {
	params["path"], _ = settings["path"].(string)
	if host, ok := settings["host"].(string); ok && host != "" {
		params["host"] = host
		return
	}
	params["host"] = searchHost(settings["headers"])
}

func applyPathAndHostObj(settings map[string]any, obj map[string]any) {
	obj["path"], _ = settings["path"].(string)
	if host, ok := settings["host"].(string); ok && host != "" {
		obj["host"] = host
		return
	}
	obj["host"] = searchHost(settings["headers"])
}

func applyShareNetworkParams(stream map[string]any, network string, params map[string]string) {
	switch network {
	case "tcp":
		tcp, _ := stream["tcpSettings"].(map[string]any)
		header, _ := tcp["header"].(map[string]any)
		headerType, _ := header["type"].(string)
		if headerType != "http" {
			return
		}
		request, _ := header["request"].(map[string]any)
		requestPaths, _ := request["path"].([]any)
		if len(requestPaths) > 0 {
			params["path"], _ = requestPaths[0].(string)
		}
		host := ""
		if response, ok := header["response"].(map[string]any); ok {
			host = searchHost(response["headers"])
		}
		if host == "" {
			host = searchHost(request["headers"])
		}
		params["host"] = host
		params["headerType"] = "http"
	case "kcp":
		applyKcpShareParams(stream, params)
	case "ws":
		settings, _ := stream["wsSettings"].(map[string]any)
		applyPathAndHostParams(settings, params)
	case "grpc":
		settings, _ := stream["grpcSettings"].(map[string]any)
		params["serviceName"], _ = settings["serviceName"].(string)
		params["authority"], _ = settings["authority"].(string)
		if multiMode, _ := settings["multiMode"].(bool); multiMode {
			params["mode"] = "multi"
		}
	case "httpupgrade":
		settings, _ := stream["httpupgradeSettings"].(map[string]any)
		applyPathAndHostParams(settings, params)
	case "xhttp":
		settings, _ := stream["xhttpSettings"].(map[string]any)
		applyXhttpExtraParams(settings, params)
	}
}

func applyXhttpExtraObj(xhttp map[string]any, obj map[string]any) {
	if padding, ok := xhttp["xPaddingBytes"].(string); ok && padding != "" {
		obj["x_padding_bytes"] = padding
	}
	maps.Copy(obj, buildXhttpExtra(xhttp))
}

func applyVmessNetworkParams(stream map[string]any, network string, obj map[string]any) {
	obj["net"] = network
	switch network {
	case "tcp":
		tcp, _ := stream["tcpSettings"].(map[string]any)
		header, _ := tcp["header"].(map[string]any)
		headerType, _ := header["type"].(string)
		obj["type"] = headerType
		if headerType != "http" {
			return
		}
		request, _ := header["request"].(map[string]any)
		requestPaths, _ := request["path"].([]any)
		if len(requestPaths) > 0 {
			obj["path"], _ = requestPaths[0].(string)
		}
		host := ""
		if response, ok := header["response"].(map[string]any); ok {
			host = searchHost(response["headers"])
		}
		if host == "" {
			host = searchHost(request["headers"])
		}
		obj["host"] = host
	case "kcp":
		applyKcpShareObj(stream, obj)
	case "ws":
		settings, _ := stream["wsSettings"].(map[string]any)
		applyPathAndHostObj(settings, obj)
	case "grpc":
		settings, _ := stream["grpcSettings"].(map[string]any)
		obj["path"], _ = settings["serviceName"].(string)
		obj["authority"], _ = settings["authority"].(string)
		if multiMode, _ := settings["multiMode"].(bool); multiMode {
			obj["type"] = "multi"
		}
	case "httpupgrade":
		settings, _ := stream["httpupgradeSettings"].(map[string]any)
		applyPathAndHostObj(settings, obj)
	case "xhttp":
		settings, _ := stream["xhttpSettings"].(map[string]any)
		applyPathAndHostObj(settings, obj)
		if mode, ok := settings["mode"].(string); ok {
			obj["mode"] = mode
		}
		applyXhttpExtraObj(settings, obj)
	}
}

func applyShareTLSParams(stream map[string]any, params map[string]string) {
	params["security"] = "tls"
	tlsSetting, _ := stream["tlsSettings"].(map[string]any)
	if alpn := stringSlice(tlsSetting["alpn"]); len(alpn) > 0 {
		params["alpn"] = strings.Join(alpn, ",")
	}
	if sni, ok := searchKey(tlsSetting, "serverName"); ok {
		params["sni"], _ = sni.(string)
	}
	applyTLSClientParams(tlsSetting, params)
}

func applyVmessTLSParams(stream map[string]any, obj map[string]any) {
	tlsSetting, _ := stream["tlsSettings"].(map[string]any)
	if alpn := stringSlice(tlsSetting["alpn"]); len(alpn) > 0 {
		obj["alpn"] = strings.Join(alpn, ",")
	}
	if sni, ok := searchKey(tlsSetting, "serverName"); ok {
		obj["sni"], _ = sni.(string)
	}
	applyTLSClientFields(tlsSetting, obj)
}

func stringSlice(value any) []string {
	raw, _ := value.([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func applyTLSClientParams(tlsSetting map[string]any, params map[string]string) {
	settings, _ := searchKey(tlsSetting, "settings")
	if tlsSetting == nil {
		return
	}
	if fingerprint, ok := searchKey(settings, "fingerprint"); ok {
		params["fp"], _ = fingerprint.(string)
	}
	if echConfig, ok := searchKey(settings, "echConfigList"); ok {
		if value, _ := echConfig.(string); value != "" {
			params["ech"] = value
		}
	}
	if name, ok := verifyPeerCertByNameValue(settings); ok {
		params["vcn"] = name
	}
	if pins, ok := pinnedSha256List(settings); ok {
		params["pcs"] = strings.Join(pins, ",")
	}
}

func applyTLSClientFields(tlsSetting map[string]any, obj map[string]any) {
	settings, _ := searchKey(tlsSetting, "settings")
	if tlsSetting == nil {
		return
	}
	if fingerprint, ok := searchKey(settings, "fingerprint"); ok {
		obj["fp"], _ = fingerprint.(string)
	}
	if echConfig, ok := searchKey(settings, "echConfigList"); ok {
		if value, _ := echConfig.(string); value != "" {
			obj["ech"] = value
		}
	}
	if name, ok := verifyPeerCertByNameValue(settings); ok {
		obj["vcn"] = name
	}
	if pins, ok := pinnedSha256List(settings); ok {
		obj["pcs"] = strings.Join(pins, ",")
	}
}

func verifyPeerCertByNameValue(tlsClientSettings any) (string, bool) {
	raw, ok := searchKey(tlsClientSettings, "verifyPeerCertByName")
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func pinnedSha256List(tlsClientSettings any) ([]string, bool) {
	raw, ok := searchKey(tlsClientSettings, "pinnedPeerCertSha256")
	if !ok {
		return nil, false
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			out = append(out, text)
		}
	}
	return out, len(out) > 0
}

func hysteriaPinHex(pin string) string {
	pin = strings.TrimSpace(pin)
	if value := strings.ReplaceAll(pin, ":", ""); len(value) == hex.EncodedLen(sha256.Size) {
		if _, err := hex.DecodeString(value); err == nil {
			return strings.ToLower(value)
		}
	}
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if value, err := encoding.DecodeString(pin); err == nil && len(value) == sha256.Size {
			return hex.EncodeToString(value)
		}
	}
	return pin
}

func applyShareRealityParams(stream map[string]any, params map[string]string, clientKey string) {
	params["security"] = "reality"
	realitySetting, _ := stream["realitySettings"].(map[string]any)
	realitySettings, _ := searchKey(realitySetting, "settings")
	if realitySetting == nil {
		return
	}
	if value, ok := searchKey(realitySetting, "serverNames"); ok {
		if names, _ := value.([]any); len(names) > 0 {
			params["sni"], _ = names[random.Num(len(names))].(string)
		}
	}
	if value, ok := searchKey(realitySettings, "publicKey"); ok {
		params["pbk"], _ = value.(string)
	}
	if value, ok := searchKey(realitySetting, "shortIds"); ok {
		if shortIDs, _ := value.([]any); len(shortIDs) > 0 {
			params["sid"], _ = shortIDs[random.Num(len(shortIDs))].(string)
		}
	}
	for queryKey, settingKey := range map[string]string{
		"fp":  "fingerprint",
		"pqv": "mldsa65Verify",
	} {
		if value, ok := searchKey(realitySettings, settingKey); ok {
			if text, _ := value.(string); text != "" {
				params[queryKey] = text
			}
		}
	}
	seed := ""
	if value, ok := searchKey(realitySettings, "spiderX"); ok {
		seed, _ = value.(string)
	}
	params["spx"] = deriveSpiderX(seed, clientKey)
}

func subKey(client model.Client) string {
	if client.SubID != "" {
		return client.SubID
	}
	return client.Email
}

func deriveSpiderX(seed, clientKey string) string {
	if seed == "" && clientKey == "" {
		return "/" + random.Seq(15)
	}
	sum := sha256.Sum256([]byte(seed + "|" + clientKey))
	return "/" + hex.EncodeToString(sum[:])[:15]
}

func buildVmessLink(obj map[string]any) string {
	data, _ := json.MarshalIndent(obj, "", "  ")
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}

func buildLinkWithParams(link string, params map[string]string, fragment string) string {
	return appendQueryAndFragment(link, params, fragment, "", false)
}

func appendQueryAndFragment(link string, params map[string]string, fragment, securityOverride string, omitTLSFields bool) string {
	var builder strings.Builder
	builder.WriteString(link)

	if len(params) > 0 {
		query := url.Values{}
		for key, value := range params {
			if securityOverride != "" && key == "security" {
				value = securityOverride
			}
			if omitTLSFields && (key == "alpn" || key == "sni" || key == "fp" || key == "pcs") {
				continue
			}
			query.Set(key, value)
		}
		if encoded := query.Encode(); encoded != "" {
			if strings.Contains(link, "?") {
				builder.WriteByte('&')
			} else {
				builder.WriteByte('?')
			}
			builder.WriteString(encoded)
		}
	}

	if fragment != "" {
		builder.WriteByte('#')
		builder.WriteString(strings.ReplaceAll(url.QueryEscape(fragment), "+", "%20"))
	}
	return builder.String()
}

func (s *LinkService) genRemark(inbound *model.Inbound, email string) string {
	return fallbackRemark(inbound.Remark, email)
}

func fallbackRemark(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "-")
}

func searchKey(data any, key string) (any, bool) {
	switch value := data.(type) {
	case map[string]any:
		for candidate, child := range value {
			if candidate == key {
				return child, true
			}
			if result, ok := searchKey(child, key); ok {
				return result, true
			}
		}
	case []any:
		for _, child := range value {
			if result, ok := searchKey(child, key); ok {
				return result, true
			}
		}
	}
	return nil, false
}

func searchHost(headers any) string {
	data, _ := headers.(map[string]any)
	for key, value := range data {
		if !strings.EqualFold(key, "host") {
			continue
		}
		switch typed := value.(type) {
		case []any:
			if len(typed) > 0 {
				host, _ := typed[0].(string)
				return host
			}
		case string:
			return typed
		}
	}
	return ""
}
