package nginxfront

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const xrayRunDir = "/run/xray"

var templateNames = map[string]string{
	TemplateVlessTCPTLS:       "VLESS-TCP-TLS",
	TemplateVlessTCPReality:   "VLESS-TCP-REALITY",
	TemplateVlessWSTLS:        "VLESS-WS-TLS",
	TemplateVlessGRPCTLS:      "VLESS-gRPC-TLS",
	TemplateVlessGRPCReality:  "VLESS-gRPC-REALITY",
	TemplateVlessXHTTPTLS:     "VLESS-XHTTP-TLS",
	TemplateVlessXHTTPReality: "VLESS-XHTTP-REALITY",
}

func TemplateName(template string) (string, bool) {
	name, ok := templateNames[template]
	return name, ok
}

func PreferredSocketPath(template string) (string, error) {
	name, ok := TemplateName(template)
	if !ok {
		return "", fmt.Errorf("unsupported guided template %q", template)
	}
	return filepath.Join(xrayRunDir, name), nil
}

func NewSocketPath(template string) (string, error) {
	return PreferredSocketPath(template)
}

func IsManagedSocket(listen string) bool {
	return pathWithin(socketPath(listen), xrayRunDir)
}

func IsTemplateSocket(listen, template string) bool {
	name, ok := TemplateName(template)
	if !ok {
		return false
	}
	socket := filepath.Clean(socketPath(listen))
	return socket == filepath.Join(xrayRunDir, name)
}

func pathWithin(path, dir string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && relative != "." && relative != ".." &&
		!filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
