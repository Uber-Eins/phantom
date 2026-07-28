package acmecert

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeDomainIdentifiers(t *testing.T) {
	got, err := normalizeDomainIdentifiers(" Example.COM \n*.example.com\r\nexample.com\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"example.com", "*.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identifiers = %#v, want %#v", got, want)
	}
}

func TestNormalizeDomainIdentifiersRejectsIP(t *testing.T) {
	_, err := normalizeDomainIdentifiers("203.0.113.10")
	if err == nil || !strings.Contains(err.Error(), "not a domain") {
		t.Fatalf("error = %v, want domain validation error", err)
	}
}

func TestNormalizeDomainIdentifiersRejectsInvalidDNSLabel(t *testing.T) {
	_, err := normalizeDomainIdentifiers("_acme.example.com")
	if err == nil || !strings.Contains(err.Error(), "not a valid domain") {
		t.Fatalf("error = %v, want invalid domain validation error", err)
	}
}

func TestValidateRequestRejectsIPCertificateForCloudflare(t *testing.T) {
	_, err := validateRequest(IssueRequest{
		Remark:           "IP certificate",
		AddMethod:        AddMethodACME,
		CA:               CALetsEncrypt,
		ValidationMethod: ValidationCloudflare,
		CloudflareToken:  "token",
		Identifiers:      "203.0.113.10",
		Email:            "admin@example.com",
		KeyType:          KeyEC256,
		CertificateType:  CertificateIP,
	})
	if err == nil || !strings.Contains(err.Error(), "does not support IP certificates") {
		t.Fatalf("error = %v, want unsupported IP certificate error", err)
	}
}
