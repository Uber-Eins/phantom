package nginxfront

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func validateDecoy(fronting *model.InboundFronting, security string) error {
	if security == "reality" {
		if fronting.DecoyMode != DecoyReality || fronting.DecoyValue != "" {
			return errors.New("REALITY uses its configured target as the decoy")
		}
		return nil
	}
	switch fronting.DecoyMode {
	case DecoyUnauthorized:
		if strings.TrimSpace(fronting.DecoyValue) != "" {
			return errors.New("401 decoy does not accept a value")
		}
	case DecoyProxy:
		parsed, err := url.ParseRequestURI(strings.TrimSpace(fronting.DecoyValue))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			return errors.New("reverse proxy target must be an http or https URL without credentials")
		}
		if strings.ContainsAny(fronting.DecoyValue, "$;{}\r\n") {
			return errors.New("reverse proxy target contains unsupported characters")
		}
	case DecoyStatic:
		value := strings.TrimSpace(fronting.DecoyValue)
		if !filepath.IsAbs(value) || strings.ContainsAny(value, "\x00\r\n") {
			return errors.New("local site path must be an absolute container path")
		}
		info, err := os.Stat(value)
		if err != nil || !info.IsDir() {
			return errors.New("local site path must be an existing directory")
		}
		dir, err := os.Open(value)
		if err != nil {
			return errors.New("local site path must be readable")
		}
		_ = dir.Close()
	default:
		return fmt.Errorf("unsupported decoy mode %q", fronting.DecoyMode)
	}
	return nil
}

func validateTopology(routes []route) error {
	owners := map[string][]route{}
	for _, candidate := range routes {
		if err := validateDecoy(candidate.Fronting, candidate.Security); err != nil {
			return err
		}
		for _, sni := range candidate.SNIs {
			owners[sni] = append(owners[sni], candidate)
		}
	}
	for sni, group := range owners {
		if len(group) == 1 {
			continue
		}
		for _, item := range group {
			if !isHTTPTLSTemplate(item.Fronting.Template) {
				return fmt.Errorf("SNI %s is already used by an exclusive TCP/REALITY route", sni)
			}
		}
		first := group[0]
		paths := map[string]string{}
		for _, item := range group {
			if item.Cert.Identity != first.Cert.Identity ||
				item.Fronting.DecoyMode != first.Fronting.DecoyMode ||
				item.Fronting.DecoyValue != first.Fronting.DecoyValue {
				return fmt.Errorf("TLS routes sharing SNI %s must use the same certificate and decoy", sni)
			}
			if _, exists := paths[item.Path]; exists {
				return fmt.Errorf("TLS routes sharing SNI %s must use distinct paths", sni)
			}
			for existingPath, existingNetwork := range paths {
				if (item.Network == "grpc" || existingNetwork == "grpc") &&
					pathsOverlap(item.Path, existingPath) {
					return fmt.Errorf("TLS routes sharing SNI %s have overlapping gRPC paths", sni)
				}
			}
			paths[item.Path] = item.Network
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	a = strings.TrimSuffix(a, "/")
	b = strings.TrimSuffix(b, "/")
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
