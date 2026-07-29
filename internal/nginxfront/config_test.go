package nginxfront

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func TestRenderConfigBuildsDualStackSNIAndPathRoutes(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	certFile, keyFile := writeTestCertificate(t)
	routes := []route{
		testTLSRoute(1, TemplateVlessWSTLS, "shared.example.com", "/ws", "ws", certFile, keyFile),
		testTLSRoute(2, TemplateVlessGRPCTLS, "shared.example.com", "/rpc", "grpc", certFile, keyFile),
		testTLSRoute(3, TemplateVlessTCPTLS, "tcp.example.com", "", "tcp", certFile, keyFile),
		testTLSRoute(4, TemplateVlessXHTTPTLS, "xhttp.example.com", "/xhttp", "xhttp", certFile, keyFile),
	}

	rendered, err := renderConfig(routes)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"listen 0.0.0.0:443;",
		"listen [::]:443;",
		"ssl_preread on;",
		"location = \"/ws\"",
		"location ^~ \"/rpc/\"",
		"location = \"/xhttp\"",
		"location ^~ \"/xhttp/\"",
		"grpc_pass grpc://xray_2;",
		"proxy_protocol on;",
		nginxQuote("unix:"+H1FallbackSocket()) + " proxy_protocol default_server",
		nginxQuote("unix:"+H2FallbackSocket()) + " proxy_protocol default_server",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered config missing %q:\n%s", expected, rendered)
		}
	}
	if count := strings.Count(rendered, `"shared.example.com" "unix:`); count != 1 {
		t.Fatalf("expected one stream map entry for shared SNI, got %d", count)
	}

	if _, err := exec.LookPath(nginxBinary()); err != nil {
		t.Skip("nginx is not installed")
	}
	if err := os.MkdirAll(RunDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ConfigDir(), "render-test.conf")
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := testConfig(path); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRuntimeProjectionForHTTPTLS(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	certFile, keyFile := writeTestCertificate(t)
	inbound := testTLSInbound(4, "ws", "guide.example.com", certFile, keyFile)
	inbound.Fronting = &model.InboundFronting{
		InboundId: 4,
		Template:  TemplateVlessWSTLS,
		DecoyMode: DecoyUnauthorized,
	}
	inbound.Listen, _ = PreferredSocketPath(inbound.Fronting.Template)

	if err := ApplyRuntimeProjection(inbound); err != nil {
		t.Fatal(err)
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		t.Fatal(err)
	}
	if stream["security"] != "none" {
		t.Fatalf("expected Nginx-terminated security, got %#v", stream["security"])
	}
	if _, exists := stream["tlsSettings"]; exists {
		t.Fatal("runtime stream must not retain tlsSettings")
	}
	sockopt, _ := stream["sockopt"].(map[string]any)
	headers, _ := sockopt["trustedXForwardedFor"].([]any)
	if len(headers) != 1 || headers[0] != "X-Real-IP" {
		t.Fatalf("unexpected trusted forwarded headers: %#v", headers)
	}
}

func TestApplyRuntimeProjectionForTCPTLSAddsManagedFallbacks(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	certFile, keyFile := writeTestCertificate(t)
	inbound := testTLSInbound(5, "tcp", "tcp.example.com", certFile, keyFile)
	inbound.Fronting = &model.InboundFronting{
		InboundId: 5,
		Template:  TemplateVlessTCPTLS,
		DecoyMode: DecoyUnauthorized,
	}
	inbound.Listen, _ = PreferredSocketPath(inbound.Fronting.Template)

	if err := ApplyRuntimeProjection(inbound); err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	fallbacks, _ := settings["fallbacks"].([]any)
	if len(fallbacks) != 2 {
		t.Fatalf("managed fallback count = %d", len(fallbacks))
	}
	for _, raw := range fallbacks {
		fallback, _ := raw.(map[string]any)
		if fallback["xver"] != float64(1) {
			t.Fatalf("fallback does not preserve client address: %#v", fallback)
		}
	}
}

func TestValidateTopologyRejectsExclusiveSNIReuse(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	certFile, keyFile := writeTestCertificate(t)
	httpRoute := testTLSRoute(1, TemplateVlessWSTLS, "same.example.com", "/ws", "ws", certFile, keyFile)
	exclusive := testTLSRoute(2, TemplateVlessTCPTLS, "same.example.com", "", "tcp", certFile, keyFile)
	if err := validateTopology([]route{httpRoute, exclusive}); err == nil {
		t.Fatal("expected exclusive SNI conflict")
	}
}

func testTLSRoute(
	id int,
	template string,
	sni string,
	path string,
	network string,
	certFile string,
	keyFile string,
) route {
	inbound := testTLSInbound(id, network, sni, certFile, keyFile)
	inbound.Listen, _ = PreferredSocketPath(template)
	return route{
		Inbound: inbound,
		Fronting: &model.InboundFronting{
			InboundId: id,
			Template:  template,
			DecoyMode: DecoyUnauthorized,
		},
		Network:  network,
		Security: "tls",
		SNIs:     []string{sni},
		Path:     path,
		Socket:   socketPath(inbound.Listen),
		Cert: certificate{
			File:     certFile,
			KeyFile:  keyFile,
			Identity: "test-certificate",
		},
	}
}

func testTLSInbound(id int, network, sni, certFile, keyFile string) *model.Inbound {
	networkKey := network + "Settings"
	networkSettings := map[string]any{}
	switch network {
	case "ws":
		networkSettings["path"] = "/ws"
	case "grpc":
		networkSettings["serviceName"] = "rpc"
	}
	stream := map[string]any{
		"network":  network,
		"security": "tls",
		networkKey: networkSettings,
		"tlsSettings": map[string]any{
			"serverName": sni,
			"certificates": []any{
				map[string]any{
					"certificateFile": certFile,
					"keyFile":         keyFile,
				},
			},
		},
	}
	streamBytes, _ := json.Marshal(stream)
	return &model.Inbound{
		Id:             id,
		Protocol:       model.VLESS,
		Port:           0,
		Settings:       `{"clients":[],"decryption":"none","encryption":"none"}`,
		StreamSettings: string(streamBytes),
	}
}

func writeTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "guide.example.com"},
		DNSNames:     []string{"guide.example.com", "shared.example.com", "same.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
