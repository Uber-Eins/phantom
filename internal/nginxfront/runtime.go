package nginxfront

import (
	"encoding/json"
	"fmt"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func H1FallbackSocket() string {
	return RunDir() + "/fallback-h1.sock"
}

func H2FallbackSocket() string {
	return RunDir() + "/fallback-h2.sock"
}

// ApplyRuntimeProjection converts the externally advertised guided config into
// the actual Xray listener behind Nginx. The database row remains untouched.
func ApplyRuntimeProjection(inbound *model.Inbound) error {
	if inbound == nil || inbound.Fronting == nil {
		return nil
	}
	parsed, err := parseRoute(inbound, inbound.Fronting)
	if err != nil {
		return err
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		return err
	}
	if isHTTPTLSTemplate(inbound.Fronting.Template) {
		stream["security"] = "none"
		delete(stream, "tlsSettings")
		sockopt, _ := stream["sockopt"].(map[string]any)
		if sockopt == nil {
			sockopt = map[string]any{}
		}
		sockopt["trustedXForwardedFor"] = []string{"X-Real-IP"}
		stream["sockopt"] = sockopt
	} else if parsed.Network == "tcp" {
		settings, _ := stream["tcpSettings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
		}
		settings["acceptProxyProtocol"] = true
		stream["tcpSettings"] = settings
	} else {
		sockopt, _ := stream["sockopt"].(map[string]any)
		if sockopt == nil {
			sockopt = map[string]any{}
		}
		sockopt["acceptProxyProtocol"] = true
		stream["sockopt"] = sockopt
	}
	streamBytes, err := json.MarshalIndent(stream, "", "  ")
	if err != nil {
		return err
	}
	inbound.StreamSettings = string(streamBytes)

	if inbound.Fronting.Template != TemplateVlessTCPTLS {
		return nil
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return err
	}
	fallbacks, _ := settings["fallbacks"].([]any)
	fallbacks = append(fallbacks,
		map[string]any{"alpn": "h2", "dest": H2FallbackSocket(), "xver": 1},
		map[string]any{"dest": H1FallbackSocket(), "xver": 1},
	)
	settings["fallbacks"] = fallbacks
	settingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode managed fallbacks: %w", err)
	}
	inbound.Settings = string(settingsBytes)
	return nil
}
