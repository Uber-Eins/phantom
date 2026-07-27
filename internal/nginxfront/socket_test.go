package nginxfront

import "testing"

func TestGuidedSocketUsesXrayRuntimeDirectoryOnly(t *testing.T) {
	path, err := NewSocketPath(TemplateVlessTCPTLS)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/run/xray/VLESS-TCP-TLS" {
		t.Fatalf("guided socket = %q", path)
	}
	if IsManagedSocket("/etc/x-ui/nginx/run/VLESS-TCP-TLS") {
		t.Fatal("persistent Nginx run directory must not be treated as a guided socket")
	}
}
