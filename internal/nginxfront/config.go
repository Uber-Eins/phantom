package nginxfront

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

func httpsSocket() string {
	return filepath.Join(RunDir(), "https.sock")
}

func writeInlineCertificates(routes []route) error {
	certDir := filepath.Join(ConfigDir(), "certs")
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		return err
	}
	for _, item := range routes {
		if item.Cert.PEM == "" {
			continue
		}
		if err := os.WriteFile(item.Cert.File, []byte(item.Cert.PEM), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(item.Cert.KeyFile, []byte(item.Cert.KeyPEM), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func renderConfig(routes []route) (string, error) {
	if err := validateTopology(routes); err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString("include /etc/nginx/modules/*.conf;\n")
	out.WriteString("user root;\n")
	out.WriteString("daemon off;\n")
	out.WriteString("worker_processes auto;\n")
	fmt.Fprintf(&out, "pid %s;\n", nginxQuote(filepath.Join(RunDir(), "nginx.pid")))
	out.WriteString("error_log stderr notice;\n\n")
	out.WriteString("events { worker_connections 1024; }\n\n")

	httpRoutes := make([]route, 0)
	directRoutes := make([]route, 0)
	tcpTLSRoutes := make([]route, 0)
	for _, item := range routes {
		if isHTTPTLSTemplate(item.Fronting.Template) {
			httpRoutes = append(httpRoutes, item)
		} else {
			directRoutes = append(directRoutes, item)
		}
		if item.Fronting.Template == TemplateVlessTCPTLS {
			tcpTLSRoutes = append(tcpTLSRoutes, item)
		}
	}

	defaultBackend := ""
	if len(httpRoutes) > 0 || len(tcpTLSRoutes) > 0 {
		defaultBackend = "unix:" + httpsSocket()
	} else if len(directRoutes) > 0 {
		defaultBackend = "unix:" + directRoutes[0].Socket
	}

	out.WriteString("stream {\n")
	out.WriteString("    map $ssl_preread_server_name $front_backend {\n")
	fmt.Fprintf(&out, "        default %s;\n", nginxQuote(defaultBackend))
	seenSNIs := make(map[string]struct{})
	for _, item := range routes {
		backend := "unix:" + item.Socket
		if isHTTPTLSTemplate(item.Fronting.Template) {
			backend = "unix:" + httpsSocket()
		}
		for _, sni := range item.SNIs {
			if _, exists := seenSNIs[sni]; exists {
				continue
			}
			seenSNIs[sni] = struct{}{}
			fmt.Fprintf(&out, "        %s %s;\n", nginxQuote(sni), nginxQuote(backend))
		}
	}
	out.WriteString("    }\n\n")
	out.WriteString("    server {\n")
	out.WriteString("        listen 0.0.0.0:443;\n")
	out.WriteString("        listen [::]:443;\n")
	out.WriteString("        ssl_preread on;\n")
	out.WriteString("        proxy_protocol on;\n")
	out.WriteString("        proxy_connect_timeout 10s;\n")
	out.WriteString("        proxy_timeout 1h;\n")
	out.WriteString("        proxy_pass $front_backend;\n")
	out.WriteString("    }\n")
	out.WriteString("}\n")

	if len(httpRoutes) == 0 && len(tcpTLSRoutes) == 0 {
		return out.String(), nil
	}

	out.WriteString("\nhttp {\n")
	out.WriteString("    access_log off;\n")
	out.WriteString("    server_tokens off;\n")
	out.WriteString("    include /etc/nginx/mime.types;\n")
	out.WriteString("    default_type application/octet-stream;\n")
	out.WriteString("    map $http_upgrade $connection_upgrade {\n")
	out.WriteString("        default upgrade;\n")
	out.WriteString("        '' close;\n")
	out.WriteString("    }\n\n")
	out.WriteString("    map $proxy_protocol_addr $front_client_ip {\n")
	out.WriteString("        default $proxy_protocol_addr;\n")
	out.WriteString("        '' $remote_addr;\n")
	out.WriteString("    }\n\n")

	for _, item := range httpRoutes {
		fmt.Fprintf(&out, "    upstream xray_%d {\n", item.Inbound.Id)
		fmt.Fprintf(&out, "        server %s;\n", nginxQuote("unix:"+item.Socket))
		out.WriteString("    }\n")
	}

	if len(httpRoutes) > 0 {
		groups := groupHTTPRoutes(httpRoutes)
		first := groups[0][0]
		writeHTTPSDefaultServer(&out, first)
		for _, group := range groups {
			writeHTTPSRouteServer(&out, group)
		}
	} else {
		// TCP+TLS fallbacks still need a certificate-backed sink for unknown SNI.
		writeHTTPSDefaultServer(&out, tcpTLSRoutes[0])
	}

	if len(tcpTLSRoutes) > 0 {
		writeFallbackServers(&out, tcpTLSRoutes)
	}
	out.WriteString("}\n")
	return out.String(), nil
}

func groupHTTPRoutes(routes []route) [][]route {
	bySNI := make(map[string][]route)
	keys := make([]string, 0)
	for _, item := range routes {
		sni := item.SNIs[0]
		if _, exists := bySNI[sni]; !exists {
			keys = append(keys, sni)
		}
		bySNI[sni] = append(bySNI[sni], item)
	}
	sort.Strings(keys)
	result := make([][]route, 0, len(keys))
	for _, key := range keys {
		group := bySNI[key]
		slices.SortFunc(group, func(a, b route) int {
			return strings.Compare(a.Path, b.Path)
		})
		result = append(result, group)
	}
	return result
}

func writeHTTPSDefaultServer(out *strings.Builder, item route) {
	out.WriteString("\n    server {\n")
	fmt.Fprintf(out, "        listen %s ssl proxy_protocol default_server;\n", nginxQuote("unix:"+httpsSocket()))
	out.WriteString("        http2 on;\n")
	out.WriteString("        server_name _;\n")
	fmt.Fprintf(out, "        ssl_certificate %s;\n", nginxQuote(item.Cert.File))
	fmt.Fprintf(out, "        ssl_certificate_key %s;\n", nginxQuote(item.Cert.KeyFile))
	out.WriteString("        return 401;\n")
	out.WriteString("    }\n")
}

func writeHTTPSRouteServer(out *strings.Builder, group []route) {
	first := group[0]
	out.WriteString("\n    server {\n")
	fmt.Fprintf(out, "        listen %s ssl proxy_protocol;\n", nginxQuote("unix:"+httpsSocket()))
	out.WriteString("        http2 on;\n")
	fmt.Fprintf(out, "        server_name %s;\n", nginxQuote(first.SNIs[0]))
	fmt.Fprintf(out, "        ssl_certificate %s;\n", nginxQuote(first.Cert.File))
	fmt.Fprintf(out, "        ssl_certificate_key %s;\n", nginxQuote(first.Cert.KeyFile))
	for _, item := range group {
		writeTransportLocation(out, item)
	}
	writeDecoyLocation(out, first.Fronting.DecoyMode, first.Fronting.DecoyValue)
	out.WriteString("    }\n")
}

func writeTransportLocation(out *strings.Builder, item route) {
	upstream := "xray_" + strconv.Itoa(item.Inbound.Id)
	switch item.Network {
	case "grpc":
		path := strings.TrimSuffix(item.Path, "/") + "/"
		fmt.Fprintf(out, "        location ^~ %s {\n", nginxQuote(path))
		fmt.Fprintf(out, "            grpc_pass grpc://%s;\n", upstream)
		out.WriteString("            grpc_set_header X-Real-IP $proxy_protocol_addr;\n")
		out.WriteString("            grpc_set_header X-Forwarded-For $proxy_protocol_addr;\n")
		out.WriteString("        }\n")
	case "ws":
		fmt.Fprintf(out, "        location = %s {\n", nginxQuote(item.Path))
		fmt.Fprintf(out, "            proxy_pass http://%s;\n", upstream)
		out.WriteString("            proxy_http_version 1.1;\n")
		out.WriteString("            proxy_set_header Upgrade $http_upgrade;\n")
		out.WriteString("            proxy_set_header Connection $connection_upgrade;\n")
		out.WriteString("            proxy_set_header Host $host;\n")
		out.WriteString("            proxy_set_header X-Real-IP $proxy_protocol_addr;\n")
		out.WriteString("            proxy_set_header X-Forwarded-For $proxy_protocol_addr;\n")
		out.WriteString("        }\n")
	case "xhttp":
		path := strings.TrimRight(item.Path, "/")
		if path == "" {
			path = "/"
		}
		writeHTTPProxyLocation(out, "=", path, upstream)
		if path != "/" {
			writeHTTPProxyLocation(out, "^~", path+"/", upstream)
		}
	default:
		writeHTTPProxyLocation(out, "=", item.Path, upstream)
	}
}

func writeHTTPProxyLocation(out *strings.Builder, modifier, path, upstream string) {
	fmt.Fprintf(out, "        location %s %s {\n", modifier, nginxQuote(path))
	fmt.Fprintf(out, "            proxy_pass http://%s;\n", upstream)
	out.WriteString("            proxy_http_version 1.1;\n")
	out.WriteString("            proxy_set_header Host $host;\n")
	out.WriteString("            proxy_set_header X-Real-IP $proxy_protocol_addr;\n")
	out.WriteString("            proxy_set_header X-Forwarded-For $proxy_protocol_addr;\n")
	out.WriteString("        }\n")
}

func writeFallbackServers(out *strings.Builder, routes []route) {
	listeners := []string{H1FallbackSocket(), H2FallbackSocket()}
	for index, socket := range listeners {
		for i, item := range routes {
			out.WriteString("\n    server {\n")
			defaultMarker := ""
			if i == 0 {
				defaultMarker = " default_server"
			}
			fmt.Fprintf(out, "        listen %s proxy_protocol%s;\n", nginxQuote("unix:"+socket), defaultMarker)
			if index == 1 {
				out.WriteString("        http2 on;\n")
			}
			fmt.Fprintf(out, "        server_name %s;\n", nginxQuote(item.SNIs[0]))
			writeDecoyLocation(out, item.Fronting.DecoyMode, item.Fronting.DecoyValue)
			out.WriteString("    }\n")
		}
	}
}

func writeDecoyLocation(out *strings.Builder, mode, value string) {
	value = strings.TrimSpace(value)
	switch mode {
	case DecoyProxy:
		target, _ := url.Parse(value)
		host := target.Host
		out.WriteString("        location / {\n")
		fmt.Fprintf(out, "            proxy_pass %s;\n", nginxQuote(value))
		out.WriteString("            proxy_http_version 1.1;\n")
		fmt.Fprintf(out, "            proxy_set_header Host %s;\n", nginxQuote(host))
		out.WriteString("            proxy_set_header X-Real-IP $front_client_ip;\n")
		out.WriteString("            proxy_set_header X-Forwarded-For $front_client_ip;\n")
		if target.Scheme == "https" {
			out.WriteString("            proxy_ssl_server_name on;\n")
			fmt.Fprintf(out, "            proxy_ssl_name %s;\n", nginxQuote(target.Hostname()))
		}
		out.WriteString("        }\n")
	case DecoyStatic:
		out.WriteString("        location / {\n")
		fmt.Fprintf(out, "            root %s;\n", nginxQuote(filepath.Clean(value)))
		out.WriteString("            index index.html;\n")
		out.WriteString("            try_files $uri $uri/ =404;\n")
		out.WriteString("        }\n")
	default:
		out.WriteString("        location / { return 401; }\n")
	}
}

func nginxQuote(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\r", "",
		"\n", "",
	)
	return `"` + replacer.Replace(value) + `"`
}
