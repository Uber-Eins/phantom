package share

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/goccy/go-json"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	wgutil "github.com/Uber-Eins/phantom/v3/internal/util/wireguard"
)

const testClientID = "11111111-2222-4333-8444-555555555555"

func preparedLinkService(inboundID int, client model.Client) *LinkService {
	svc := NewLinkService()
	svc.PrepareForRequest("panel.example.test")
	svc.primeLinkClients(inboundID, []model.Client{client}, true)
	return svc
}

func parseLink(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse link %q: %v", raw, err)
	}
	return parsed
}

func TestLocalProtocolLinks(t *testing.T) {
	tests := []struct {
		name           string
		protocol       model.Protocol
		settings       string
		streamSettings string
		client         model.Client
		wantScheme     string
		check          func(*testing.T, *url.URL)
	}{
		{
			name:           "vless",
			protocol:       model.VLESS,
			settings:       `{"decryption":"none","encryption":"none"}`,
			streamSettings: `{"network":"ws","security":"tls","wsSettings":{"path":"/ws","host":"edge.example"},"tlsSettings":{"serverName":"tls.example"}}`,
			client:         model.Client{ID: testClientID, Email: "alice", Flow: "xtls-rprx-vision", SubID: "stable"},
			wantScheme:     "vless",
			check: func(t *testing.T, u *url.URL) {
				if u.User.Username() != testClientID {
					t.Errorf("VLESS id = %q", u.User.Username())
				}
				if u.Query().Get("path") != "/ws" || u.Query().Get("sni") != "tls.example" {
					t.Errorf("VLESS transport params = %v", u.Query())
				}
			},
		},
		{
			name:           "trojan",
			protocol:       model.Trojan,
			settings:       `{}`,
			streamSettings: `{"network":"grpc","security":"tls","grpcSettings":{"serviceName":"proxy"},"tlsSettings":{"serverName":"tls.example"}}`,
			client:         model.Client{Email: "alice", Password: "secret/with=padding"},
			wantScheme:     "trojan",
			check: func(t *testing.T, u *url.URL) {
				password, _ := u.User.Password()
				if u.User.Username() != "secret/with=padding" && password != "secret/with=padding" {
					t.Errorf("Trojan password did not round trip: %q", u.User.String())
				}
				if u.Query().Get("serviceName") != "proxy" {
					t.Errorf("Trojan serviceName = %q", u.Query().Get("serviceName"))
				}
			},
		},
		{
			name:           "shadowsocks",
			protocol:       model.Shadowsocks,
			settings:       `{"method":"aes-256-gcm"}`,
			streamSettings: `{"network":"tcp","security":"none"}`,
			client:         model.Client{Email: "alice", Password: "ss-secret"},
			wantScheme:     "ss",
			check: func(t *testing.T, u *url.URL) {
				decoded, err := base64.RawURLEncoding.DecodeString(u.User.Username())
				if err != nil {
					t.Fatalf("decode Shadowsocks userinfo: %v", err)
				}
				if string(decoded) != "aes-256-gcm:ss-secret" {
					t.Errorf("Shadowsocks userinfo = %q", decoded)
				}
			},
		},
		{
			name:           "hysteria2",
			protocol:       model.Hysteria,
			settings:       `{"version":2}`,
			streamSettings: `{"security":"tls","tlsSettings":{"serverName":"tls.example","alpn":["h3"]}}`,
			client:         model.Client{Email: "alice", Auth: "hy-secret"},
			wantScheme:     "hysteria2",
			check: func(t *testing.T, u *url.URL) {
				if u.User.Username() != "hy-secret" || u.Query().Get("alpn") != "h3" {
					t.Errorf("Hysteria fields: user=%q query=%v", u.User.Username(), u.Query())
				}
			},
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inbound := &model.Inbound{
				Id:             i + 1,
				Port:           443,
				Protocol:       test.protocol,
				Remark:         "home",
				Settings:       test.settings,
				StreamSettings: test.streamSettings,
			}
			raw := preparedLinkService(inbound.Id, test.client).GetLink(inbound, test.client.Email)
			u := parseLink(t, raw)
			if u.Scheme != test.wantScheme {
				t.Fatalf("scheme = %q, want %q; link=%s", u.Scheme, test.wantScheme, raw)
			}
			if u.Hostname() != "panel.example.test" {
				t.Errorf("host = %q, want authenticated request host", u.Hostname())
			}
			if u.Fragment != "home-alice" {
				t.Errorf("fragment = %q, want local remark", u.Fragment)
			}
			if strings.Contains(raw, "/sub/") || strings.Contains(raw, "externalProxy") {
				t.Errorf("link contains removed public/remote metadata: %s", raw)
			}
			test.check(t, u)
		})
	}
}

func TestVmessLinkUsesRequestHost(t *testing.T) {
	client := model.Client{ID: testClientID, Email: "alice", Security: "auto"}
	inbound := &model.Inbound{
		Id:             10,
		Port:           8443,
		Protocol:       model.VMESS,
		Remark:         "home",
		Settings:       `{}`,
		StreamSettings: `{"network":"ws","security":"tls","wsSettings":{"path":"/vmess"}}`,
	}
	raw := preparedLinkService(inbound.Id, client).GetLink(inbound, client.Email)
	encoded := strings.TrimPrefix(raw, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode VMess link: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatalf("decode VMess JSON: %v", err)
	}
	if payload["add"] != "panel.example.test" || payload["id"] != testClientID {
		t.Errorf("VMess identity/address = %#v", payload)
	}
	if payload["ps"] != "home-alice" {
		t.Errorf("VMess remark = %q", payload["ps"])
	}
}

func TestGuidedInboundLinkUsesNginxPublicPort(t *testing.T) {
	inbound := &model.Inbound{
		Port:           0,
		Listen:         "/run/xray/VLESS-TCP-REALITY",
		Protocol:       model.VLESS,
		Remark:         "guided",
		Settings:       `{"clients":[{"id":"` + testClientID + `","email":"alice"}],"decryption":"none","encryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"reality","tcpSettings":{"header":{"type":"none"}},"realitySettings":{"serverNames":["example.com"],"settings":{"publicKey":"public"}}}`,
		Fronting: &model.InboundFronting{
			Template:  "vless-tcp-reality",
			DecoyMode: "reality-target",
		},
	}

	links := NewLinkProvider().LinksForClient("panel.example.test", inbound, "alice")
	if len(links) != 1 {
		t.Fatalf("guided links = %#v", links)
	}
	parsed := parseLink(t, links[0])
	if parsed.Hostname() != "panel.example.test" || parsed.Port() != "443" {
		t.Fatalf("guided endpoint = %s", parsed.Host)
	}
	if inbound.Port != 0 {
		t.Fatalf("link projection mutated stored port to %d", inbound.Port)
	}
}

func TestMtprotoLinkUsesClientSecret(t *testing.T) {
	const secret = "ee8196fe6ed8b637d001f91d6952cfcdf07777772e636c6f7564666c6172652e636f6d"
	client := model.Client{Email: "alice", Secret: secret}
	inbound := &model.Inbound{
		Id:       11,
		Port:     8443,
		Protocol: model.MTProto,
		Remark:   "home",
		Settings: `{}`,
	}
	u := parseLink(t, preparedLinkService(inbound.Id, client).GetLink(inbound, client.Email))
	if u.Scheme != "tg" || u.Host != "proxy" {
		t.Fatalf("MTProto link = %s", u)
	}
	if u.Query().Get("server") != "panel.example.test" || u.Query().Get("secret") != secret {
		t.Errorf("MTProto params = %v", u.Query())
	}
	if u.Fragment != "" {
		t.Errorf("MTProto link must not have a fragment: %q", u.Fragment)
	}
}

func TestWireguardLinkUsesLocalKeys(t *testing.T) {
	serverPrivate, serverPublic, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	clientPrivate, _, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatalf("client keypair: %v", err)
	}
	client := model.Client{
		Email:      "alice",
		PrivateKey: clientPrivate,
		AllowedIPs: []string{"10.0.0.2/32", "fd00::2/128"},
		KeepAlive:  25,
	}
	inbound := &model.Inbound{
		Id:       12,
		Port:     51820,
		Protocol: model.WireGuard,
		Remark:   "home",
		Settings: `{"secretKey":"` + serverPrivate + `","mtu":1420}`,
	}
	u := parseLink(t, preparedLinkService(inbound.Id, client).GetLink(inbound, client.Email))
	if u.Scheme != "wireguard" || u.Hostname() != "panel.example.test" {
		t.Fatalf("WireGuard link = %s", u)
	}
	if u.User.Username() != clientPrivate {
		t.Errorf("WireGuard client key = %q", u.User.Username())
	}
	if u.Query().Get("publickey") != serverPublic ||
		u.Query().Get("address") != "10.0.0.2/32,fd00::2/128" ||
		u.Query().Get("keepalive") != "25" {
		t.Errorf("WireGuard params = %v", u.Query())
	}
}

func TestBuildXhttpExtraEmitsSessionCompatibilityAliases(t *testing.T) {
	extra := buildXhttpExtra(map[string]any{
		"sessionIDPlacement": "header",
		"sessionIDKey":       "X-Session",
	})

	for key, want := range map[string]string{
		"sessionIDPlacement": "header",
		"sessionPlacement":   "header",
		"sessionIDKey":       "X-Session",
		"sessionKey":         "X-Session",
	} {
		if got := extra[key]; got != want {
			t.Errorf("%s = %v, want %q", key, got, want)
		}
	}
}
