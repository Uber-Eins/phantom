package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func hysteriaSettingsVersion(t *testing.T, settings string) any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	return parsed["version"]
}

func TestHealHysteriaVersion(t *testing.T) {
	tests := []struct {
		name        string
		settings    string
		wantChanged bool
	}{
		{name: "legacy v1 row", settings: `{"version":1,"clients":[]}`, wantChanged: true},
		{name: "missing version", settings: `{"clients":[]}`, wantChanged: true},
		{name: "already v2", settings: `{"version":2,"clients":[]}`, wantChanged: false},
		{name: "string version", settings: `{"version":"1","clients":[]}`, wantChanged: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healed, changed := HealHysteriaVersion(tt.settings)
			if changed != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if got := hysteriaSettingsVersion(t, healed); got != float64(2) {
				t.Fatalf("version = %#v, want 2", got)
			}
		})
	}
}

func TestHealHysteriaVersionKeepsClients(t *testing.T) {
	healed, changed := HealHysteriaVersion(`{"version":1,"clients":[{"auth":"tok","email":"a@x"}]}`)
	if !changed {
		t.Fatal("a v1 row must be healed")
	}
	if !strings.Contains(healed, `"auth": "tok"`) || !strings.Contains(healed, `"email": "a@x"`) {
		t.Fatalf("healing dropped client data: %s", healed)
	}
}

func TestHealHysteriaVersionLeavesInvalidInputAlone(t *testing.T) {
	const broken = `{"version":1,`
	if healed, changed := HealHysteriaVersion(broken); changed || healed != broken {
		t.Fatalf("invalid settings changed: changed=%v value=%q", changed, healed)
	}
	if healed, changed := HealHysteriaVersion(""); changed || healed != "" {
		t.Fatalf("empty settings changed: changed=%v value=%q", changed, healed)
	}
}

func TestHealHysteriaStreamVersion(t *testing.T) {
	healed, changed := HealHysteriaStreamVersion(
		`{"network":"hysteria","hysteriaSettings":{"version":1,"udpIdleTimeout":60}}`,
	)
	if !changed {
		t.Fatal("a v1 transport must be healed")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(healed), &parsed); err != nil {
		t.Fatalf("unmarshal stream settings: %v", err)
	}
	hysteria, _ := parsed["hysteriaSettings"].(map[string]any)
	if hysteria["version"] != float64(2) || hysteria["udpIdleTimeout"] != float64(60) {
		t.Fatalf("unexpected healed transport: %#v", hysteria)
	}
}

func TestHealHysteriaStreamVersionWithoutHysteriaSettings(t *testing.T) {
	const stream = `{"network":"tcp","tcpSettings":{}}`
	if healed, changed := HealHysteriaStreamVersion(stream); changed || healed != stream {
		t.Fatalf("unrelated stream changed: changed=%v value=%q", changed, healed)
	}
}

func TestGenXrayInboundConfigHealsHysteriaVersion(t *testing.T) {
	in := Inbound{
		Protocol:       Hysteria,
		Port:           36715,
		Listen:         "127.0.0.1",
		Tag:            "in-hysteria",
		Settings:       `{"version":1,"clients":[{"auth":"tok","email":"a@x"}]}`,
		StreamSettings: `{"network":"hysteria","hysteriaSettings":{"version":1,"udpIdleTimeout":60}}`,
	}
	cfg := in.GenXrayInboundConfig()
	if got := hysteriaSettingsVersion(t, string(cfg.Settings)); got != float64(2) {
		t.Fatalf("generated settings.version = %#v, want 2", got)
	}
	var stream map[string]any
	if err := json.Unmarshal(cfg.StreamSettings, &stream); err != nil {
		t.Fatalf("unmarshal generated stream settings: %v", err)
	}
	hysteria, _ := stream["hysteriaSettings"].(map[string]any)
	if hysteria["version"] != float64(2) {
		t.Fatalf("generated hysteriaSettings.version = %#v, want 2", hysteria["version"])
	}
	if !strings.Contains(in.Settings, `"version":1`) {
		t.Fatal("healing must not mutate the stored row")
	}
}

func TestGenXrayInboundConfigLeavesOtherProtocolsAlone(t *testing.T) {
	in := Inbound{
		Protocol: VLESS,
		Port:     443,
		Tag:      "in-vless",
		Settings: `{"clients":[],"decryption":"none"}`,
	}
	if got := hysteriaSettingsVersion(t, string(in.GenXrayInboundConfig().Settings)); got != nil {
		t.Fatalf("non-Hysteria inbound gained version %#v", got)
	}
}
