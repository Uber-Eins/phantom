package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"

	"github.com/xtls/xray-core/infra/conf"
)

// The frontend golden fixtures are the panel's model of an Xray config.
// Building them through xray-core also proves the generated wire config is
// accepted by the version pinned in go.mod.

func goldenFixtureDir(t *testing.T, category string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "frontend", "src", "test", "golden", "fixtures", category))
	if err != nil {
		t.Fatalf("resolve fixture dir: %v", err)
	}
	return dir
}

func goldenFixtures(t *testing.T, category string) map[string]map[string]any {
	t.Helper()
	dir := goldenFixtureDir(t, category)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("no fixtures under %s", dir)
	}

	fixtures := make(map[string]map[string]any, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var fixture map[string]any
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		fixtures[strings.TrimSuffix(name, ".json")] = fixture
	}
	return fixtures
}

func writeTestCertificate(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "golden-fixture.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"golden-fixture.test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "fixture.crt")
	keyPath = filepath.Join(dir, "fixture.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

func repointCertificateFiles(node any, certPath, keyPath string) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			switch key {
			case "certificateFile":
				if _, ok := child.(string); ok {
					value[key] = certPath
					continue
				}
			case "keyFile":
				if _, ok := child.(string); ok {
					value[key] = keyPath
					continue
				}
			}
			repointCertificateFiles(child, certPath, keyPath)
		}
	case []any:
		for _, child := range value {
			repointCertificateFiles(child, certPath, keyPath)
		}
	}
}

func assertXrayAccepts(t *testing.T, subject string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if isMissingGeoAssetErr(err) {
		t.Skipf("geo data files not available, cannot judge %s: %v", subject, err)
	}
	t.Fatalf("xray-core refuses %s: %v", subject, err)
}

func buildGoldenInbound(t *testing.T, inbound map[string]any) error {
	t.Helper()
	certPath, keyPath := writeTestCertificate(t)
	repointCertificateFiles(inbound, certPath, keyPath)

	raw, err := json.Marshal(inbound)
	if err != nil {
		t.Fatalf("marshal inbound: %v", err)
	}
	detour := new(conf.InboundDetourConfig)
	if err := json.Unmarshal(raw, detour); err != nil {
		return err
	}
	_, err = detour.Build()
	return err
}

func TestGoldenInboundFixturesBuildInXray(t *testing.T) {
	for name, fixture := range goldenFixtures(t, "inbound") {
		if protocol, _ := fixture["protocol"].(string); protocol == string(model.MTProto) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			inbound := map[string]any{
				"tag":      "golden-in",
				"listen":   "127.0.0.1",
				"port":     8443,
				"protocol": fixture["protocol"],
				"settings": fixture["settings"],
			}
			assertXrayAccepts(t, "this fixture", buildGoldenInbound(t, inbound))
		})
	}
}

func TestGoldenInboundFullFixturesBuildInXray(t *testing.T) {
	for name, fixture := range goldenFixtures(t, "inbound-full") {
		protocol, _ := fixture["protocol"].(string)
		if protocol == string(model.MTProto) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			section := func(key string) string {
				value, ok := fixture[key]
				if !ok || value == nil {
					return ""
				}
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatalf("marshal %s: %v", key, err)
				}
				return string(raw)
			}
			port := 0
			if value, ok := fixture["port"].(float64); ok {
				port = int(value)
			}
			listen, _ := fixture["listen"].(string)
			tag, _ := fixture["tag"].(string)

			inbound := &model.Inbound{
				Protocol:       model.Protocol(protocol),
				Port:           port,
				Listen:         listen,
				Tag:            tag,
				Settings:       section("settings"),
				StreamSettings: section("streamSettings"),
				Sniffing:       section("sniffing"),
			}
			raw, err := json.Marshal(inbound.GenXrayInboundConfig())
			if err != nil {
				t.Fatalf("marshal generated inbound: %v", err)
			}
			var generated map[string]any
			if err := json.Unmarshal(raw, &generated); err != nil {
				t.Fatalf("decode generated inbound: %v", err)
			}
			assertXrayAccepts(t, "the generated inbound", buildGoldenInbound(t, generated))
		})
	}
}

func TestGoldenStreamFixturesBuildInXray(t *testing.T) {
	for _, category := range []string{"stream", "security", "sockopt", "finalmask"} {
		for name, fixture := range goldenFixtures(t, category) {
			t.Run(category+"/"+name, func(t *testing.T) {
				stream := map[string]any{"network": "tcp"}
				switch category {
				case "stream":
					stream = fixture
				case "security":
					for key, value := range fixture {
						stream[key] = value
					}
				case "sockopt":
					stream["sockopt"] = fixture
				case "finalmask":
					stream["finalmask"] = fixture
				}
				inbound := map[string]any{
					"tag": "golden-in", "listen": "127.0.0.1", "port": 8443, "protocol": "vless",
					"settings": map[string]any{
						"clients":    []any{map[string]any{"id": "b831381d-6324-4d53-ad4f-8cda48b30811", "email": "golden"}},
						"decryption": "none",
					},
					"streamSettings": stream,
				}
				assertXrayAccepts(t, "this fixture", buildGoldenInbound(t, inbound))
			})
		}
	}
}

func buildGoldenRouting(routing map[string]any) error {
	raw, err := json.Marshal(routing)
	if err != nil {
		return err
	}
	router := new(conf.RouterConfig)
	if err := json.Unmarshal(raw, router); err != nil {
		return err
	}
	_, err = router.Build()
	return err
}

func TestGoldenRoutingFixturesBuildInXray(t *testing.T) {
	for name, rule := range goldenFixtures(t, "rule") {
		t.Run("rule/"+name, func(t *testing.T) {
			assertXrayAccepts(t, "this rule", buildGoldenRouting(map[string]any{
				"domainStrategy": "AsIs",
				"rules":          []any{rule},
				"balancers":      []any{map[string]any{"tag": "balancer-load", "selector": []any{"proxy-"}}},
			}))
		})
	}

	for name, balancer := range goldenFixtures(t, "balancer") {
		t.Run("balancer/"+name, func(t *testing.T) {
			assertXrayAccepts(t, "this balancer", buildGoldenRouting(map[string]any{
				"domainStrategy": "AsIs",
				"balancers":      []any{balancer},
				"rules": []any{map[string]any{
					"type": "field", "port": "443", "balancerTag": balancer["tag"],
				}},
			}))
		})
	}
}

func TestGoldenDNSFixturesBuildInXray(t *testing.T) {
	build := func(dns map[string]any) error {
		raw, err := json.Marshal(dns)
		if err != nil {
			return err
		}
		dnsConf := new(conf.DNSConfig)
		if err := json.Unmarshal(raw, dnsConf); err != nil {
			return err
		}
		_, err = dnsConf.Build()
		return err
	}

	for name, fixture := range goldenFixtures(t, "dns") {
		t.Run("dns/"+name, func(t *testing.T) {
			assertXrayAccepts(t, "this dns section", build(fixture))
		})
	}
	for name, server := range goldenFixtures(t, "dns-server") {
		t.Run("dns-server/"+name, func(t *testing.T) {
			assertXrayAccepts(t, "this dns server", build(map[string]any{"servers": []any{server}}))
		})
	}
}

func isMissingGeoAssetErr(err error) bool {
	message := err.Error()
	return strings.Contains(message, "geoip.dat") || strings.Contains(message, "geosite.dat")
}
