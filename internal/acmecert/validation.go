package acmecert

import (
	"fmt"
	"net"
	"net/mail"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

const maxIdentifiers = 100

func validateRequest(request IssueRequest) ([]string, error) {
	if strings.TrimSpace(request.Remark) == "" {
		return nil, fmt.Errorf("certificate remark is required")
	}
	if request.AddMethod != AddMethodACME {
		return nil, fmt.Errorf("unsupported certificate add method %q", request.AddMethod)
	}
	if request.CA != CAZeroSSL && request.CA != CALetsEncrypt {
		return nil, fmt.Errorf("unsupported certificate authority %q", request.CA)
	}
	if request.ValidationMethod != ValidationCloudflare {
		return nil, fmt.Errorf("unsupported validation method %q", request.ValidationMethod)
	}
	if strings.TrimSpace(request.CloudflareToken) == "" {
		return nil, fmt.Errorf("Cloudflare API token is required")
	}
	if request.CertificateType == CertificateIP {
		return nil, fmt.Errorf("Cloudflare DNS validation does not support IP certificates")
	}
	if request.CertificateType != CertificateDomain {
		return nil, fmt.Errorf("unsupported certificate type %q", request.CertificateType)
	}
	switch request.KeyType {
	case KeyEC256, KeyEC384, KeyRSA2048, KeyRSA4096:
	default:
		return nil, fmt.Errorf("unsupported certificate key type %q", request.KeyType)
	}
	email := strings.TrimSpace(request.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return nil, fmt.Errorf("a valid account email is required")
	}

	return normalizeDomainIdentifiers(request.Identifiers)
}

func normalizeDomainIdentifiers(raw string) ([]string, error) {
	seen := make(map[string]struct{})
	identifiers := make([]string, 0)

	for lineNumber, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		normalized, err := normalizeDomain(candidate)
		if err != nil {
			return nil, fmt.Errorf("identifier on line %d: %w", lineNumber+1, err)
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		identifiers = append(identifiers, normalized)
		if len(identifiers) > maxIdentifiers {
			return nil, fmt.Errorf("at most %d identifiers are allowed", maxIdentifiers)
		}
	}

	if len(identifiers) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}
	return identifiers, nil
}

func normalizeDomain(value string) (string, error) {
	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	wildcard := strings.HasPrefix(value, "*.")
	if wildcard {
		value = strings.TrimPrefix(value, "*.")
	}
	if value == "" || strings.Contains(value, "*") || net.ParseIP(value) != nil {
		return "", fmt.Errorf("%q is not a domain name", value)
	}

	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid domain name: %w", value, err)
	}
	if len(ascii) > 253 {
		return "", fmt.Errorf("%q is too long", value)
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 ||
			strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("%q is not a valid domain name", value)
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') &&
				(character < '0' || character > '9') &&
				character != '-' {
				return "", fmt.Errorf("%q is not a valid domain name", value)
			}
		}
	}
	if _, err := publicsuffix.EffectiveTLDPlusOne(ascii); err != nil {
		return "", fmt.Errorf("%q is not a registrable domain name", value)
	}
	if wildcard {
		return "*." + ascii, nil
	}
	return ascii, nil
}

func zoneName(identifier string) (string, error) {
	domain := strings.TrimPrefix(identifier, "*.")
	zone, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		return "", fmt.Errorf("find DNS zone for %q: %w", identifier, err)
	}
	return zone, nil
}
