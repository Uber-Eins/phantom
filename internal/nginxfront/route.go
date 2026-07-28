package nginxfront

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

var (
	dnsNamePattern  = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	httpPathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~%/-]+$`)
)

func parseRoute(inbound *model.Inbound, fronting *model.InboundFronting) (route, error) {
	if inbound == nil || fronting == nil {
		return route{}, errors.New("fronting route is incomplete")
	}
	if inbound.Protocol != model.VLESS {
		return route{}, errors.New("Nginx guide currently supports VLESS only")
	}
	if inbound.Port != 0 {
		return route{}, errors.New("guided Xray inbound must use port 0 with its Unix socket")
	}
	if !IsGuidedTemplate(fronting.Template) {
		return route{}, fmt.Errorf("unsupported guided template %q", fronting.Template)
	}
	expectedNetwork, expectedSecurity, _ := expectedTransport(fronting.Template)
	var stream map[string]any
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		return route{}, fmt.Errorf("invalid stream settings: %w", err)
	}
	network, _ := stream["network"].(string)
	security, _ := stream["security"].(string)
	if network != expectedNetwork || security != expectedSecurity {
		return route{}, fmt.Errorf(
			"template %s requires %s+%s",
			fronting.Template,
			expectedNetwork,
			expectedSecurity,
		)
	}
	socket := socketPath(inbound.Listen)
	if !IsTemplateSocket(inbound.Listen, fronting.Template) {
		return route{}, errors.New("guided inbound must use its template-named managed Unix socket")
	}

	result := route{
		Inbound:  inbound,
		Fronting: fronting,
		Network:  network,
		Security: security,
		Socket:   socket,
	}
	if security == "tls" {
		tlsSettings, _ := stream["tlsSettings"].(map[string]any)
		sni, _ := tlsSettings["serverName"].(string)
		sni = strings.ToLower(strings.TrimSpace(sni))
		if !validSNI(sni) {
			return route{}, errors.New("TLS SNI must be a fully-qualified DNS name")
		}
		result.SNIs = []string{sni}
		cert, err := parseCertificate(tlsSettings, inbound.Id)
		if err != nil {
			return route{}, err
		}
		result.Cert = cert
	} else {
		realitySettings, _ := stream["realitySettings"].(map[string]any)
		names, _ := realitySettings["serverNames"].([]any)
		for _, item := range names {
			name, _ := item.(string)
			name = strings.ToLower(strings.TrimSpace(name))
			if !validSNI(name) {
				return route{}, errors.New("REALITY SNI values must be fully-qualified DNS names")
			}
			if !slices.Contains(result.SNIs, name) {
				result.SNIs = append(result.SNIs, name)
			}
		}
		if len(result.SNIs) == 0 {
			return route{}, errors.New("REALITY requires at least one SNI")
		}
		target, _ := realitySettings["target"].(string)
		if strings.TrimSpace(target) == "" {
			return route{}, errors.New("REALITY target is required")
		}
		privateKey, _ := realitySettings["privateKey"].(string)
		clientSettings, _ := realitySettings["settings"].(map[string]any)
		publicKey, _ := clientSettings["publicKey"].(string)
		if strings.TrimSpace(privateKey) == "" || strings.TrimSpace(publicKey) == "" {
			return route{}, errors.New("REALITY key pair is required")
		}
	}
	result.Path = transportPath(stream, network)
	if isHTTPTLSTemplate(fronting.Template) &&
		(result.Path == "/" || !httpPathPattern.MatchString(result.Path)) {
		return route{}, errors.New("guided HTTP transport path must be a non-root URL path")
	}
	return result, nil
}

func validSNI(value string) bool {
	if value == "" || net.ParseIP(value) != nil || len(value) > 253 {
		return false
	}
	return dnsNamePattern.MatchString(value)
}

func parseCertificate(tlsSettings map[string]any, inboundID int) (certificate, error) {
	rows, _ := tlsSettings["certificates"].([]any)
	if len(rows) == 0 {
		return certificate{}, errors.New("TLS certificate and private key are required")
	}
	first, _ := rows[0].(map[string]any)
	if first == nil {
		return certificate{}, errors.New("invalid TLS certificate")
	}
	certFile, _ := first["certificateFile"].(string)
	keyFile, _ := first["keyFile"].(string)
	if certFile != "" || keyFile != "" {
		if !filepath.IsAbs(certFile) || !filepath.IsAbs(keyFile) {
			return certificate{}, errors.New("TLS certificate paths must be absolute")
		}
		if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
			return certificate{}, fmt.Errorf("invalid TLS certificate files: %w", err)
		}
		return certificate{
			File:     certFile,
			KeyFile:  keyFile,
			Identity: "file:" + certFile + "\x00" + keyFile,
		}, nil
	}
	certPEM := stringLines(first["certificate"])
	keyPEM := stringLines(first["key"])
	if certPEM == "" || keyPEM == "" {
		return certificate{}, errors.New("TLS certificate and private key are required")
	}
	if _, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM)); err != nil {
		return certificate{}, fmt.Errorf("invalid inline TLS certificate: %w", err)
	}
	certDir := filepath.Join(ConfigDir(), "certs")
	return certificate{
		File:     filepath.Join(certDir, fmt.Sprintf("inbound-%d.crt", inboundID)),
		KeyFile:  filepath.Join(certDir, fmt.Sprintf("inbound-%d.key", inboundID)),
		PEM:      certPEM,
		KeyPEM:   keyPEM,
		Identity: "inline:" + certPEM + "\x00" + keyPEM,
	}, nil
}

func stringLines(value any) string {
	switch typed := value.(type) {
	case []any:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			if line, ok := item.(string); ok {
				lines = append(lines, line)
			}
		}
		return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
	case []string:
		return strings.TrimSpace(strings.Join(typed, "\n")) + "\n"
	case string:
		return strings.TrimSpace(typed) + "\n"
	default:
		return ""
	}
}

func transportPath(stream map[string]any, network string) string {
	switch network {
	case "ws":
		settings, _ := stream["wsSettings"].(map[string]any)
		path, _ := settings["path"].(string)
		return normalizeHTTPPath(path)
	case "grpc":
		settings, _ := stream["grpcSettings"].(map[string]any)
		service, _ := settings["serviceName"].(string)
		return normalizeHTTPPath(service)
	case "xhttp":
		settings, _ := stream["xhttpSettings"].(map[string]any)
		path, _ := settings["path"].(string)
		return normalizeHTTPPath(path)
	default:
		return ""
	}
}

func normalizeHTTPPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}
