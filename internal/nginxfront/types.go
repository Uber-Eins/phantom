// Package nginxfront manages the Nginx TCP/443 entrypoint used by guided
// VLESS inbounds.
package nginxfront

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	TemplateVlessTCPTLS       = "vless-tcp-tls"
	TemplateVlessTCPReality   = "vless-tcp-reality"
	TemplateVlessWSTLS        = "vless-ws-tls"
	TemplateVlessGRPCTLS      = "vless-grpc-tls"
	TemplateVlessGRPCReality  = "vless-grpc-reality"
	TemplateVlessXHTTPTLS     = "vless-xhttp-tls"
	TemplateVlessXHTTPReality = "vless-xhttp-reality"

	DecoyUnauthorized = "unauthorized"
	DecoyProxy        = "proxy"
	DecoyStatic       = "static"
	DecoyReality      = "reality-target"

	PublicPort = 443
)

var supportedTemplates = []string{
	TemplateVlessTCPTLS,
	TemplateVlessTCPReality,
	TemplateVlessWSTLS,
	TemplateVlessGRPCTLS,
	TemplateVlessGRPCReality,
	TemplateVlessXHTTPTLS,
	TemplateVlessXHTTPReality,
}

type route struct {
	Inbound  *model.Inbound
	Fronting *model.InboundFronting
	Network  string
	Security string
	SNIs     []string
	Path     string
	Socket   string
	Cert     certificate
}

type certificate struct {
	File     string
	KeyFile  string
	PEM      string
	KeyPEM   string
	Identity string
}

func ConfigDir() string {
	return filepath.Join(config.GetDBFolderPath(), "nginx")
}

func RunDir() string {
	return filepath.Join(ConfigDir(), "run")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "nginx.conf")
}

func socketPath(listen string) string {
	value, _, _ := strings.Cut(strings.TrimSpace(listen), ",")
	return value
}

func IsGuidedTemplate(template string) bool {
	return slices.Contains(supportedTemplates, template)
}

func isHTTPTLSTemplate(template string) bool {
	switch template {
	case TemplateVlessWSTLS, TemplateVlessGRPCTLS, TemplateVlessXHTTPTLS:
		return true
	default:
		return false
	}
}

func expectedTransport(template string) (network, security string, ok bool) {
	switch template {
	case TemplateVlessTCPTLS:
		return "tcp", "tls", true
	case TemplateVlessTCPReality:
		return "tcp", "reality", true
	case TemplateVlessWSTLS:
		return "ws", "tls", true
	case TemplateVlessGRPCTLS:
		return "grpc", "tls", true
	case TemplateVlessGRPCReality:
		return "grpc", "reality", true
	case TemplateVlessXHTTPTLS:
		return "xhttp", "tls", true
	case TemplateVlessXHTTPReality:
		return "xhttp", "reality", true
	default:
		return "", "", false
	}
}
