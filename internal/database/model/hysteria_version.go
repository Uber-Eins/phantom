package model

import "encoding/json"

// hysteriaConfigVersion is the only Hysteria version xray-core builds.
const hysteriaConfigVersion = 2

// HealHysteriaVersion pins a Hysteria inbound's settings.version to the
// version xray-core accepts.
func HealHysteriaVersion(settings string) (string, bool) {
	return healHysteriaVersionField(settings, nil)
}

// HealHysteriaStreamVersion pins streamSettings.hysteriaSettings.version.
func HealHysteriaStreamVersion(streamSettings string) (string, bool) {
	return healHysteriaVersionField(streamSettings, []string{"hysteriaSettings"})
}

func healHysteriaVersionField(raw string, path []string) (string, bool) {
	if raw == "" {
		return raw, false
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw, false
	}
	target := parsed
	for _, key := range path {
		next, ok := target[key].(map[string]any)
		if !ok {
			return raw, false
		}
		target = next
	}
	if version, ok := target["version"].(float64); ok && version == hysteriaConfigVersion {
		return raw, false
	}
	target["version"] = hysteriaConfigVersion
	out, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return raw, false
	}
	return string(out), true
}
