package service

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func readCertificateIdentifiers(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var leaf *x509.Certificate
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		leaf, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", err
		}
		break
	}
	if leaf == nil {
		return "", errors.New("certificate file contains no certificate")
	}

	identifiers := make([]string, 0, len(leaf.DNSNames)+len(leaf.IPAddresses))
	for _, name := range leaf.DNSNames {
		name = strings.TrimSpace(name)
		if name != "" && !slices.Contains(identifiers, name) {
			identifiers = append(identifiers, name)
		}
	}
	for _, address := range leaf.IPAddresses {
		value := address.String()
		if value != "" && !slices.Contains(identifiers, value) {
			identifiers = append(identifiers, value)
		}
	}
	if len(identifiers) == 0 {
		commonName := strings.TrimSpace(leaf.Subject.CommonName)
		if commonName != "" {
			identifiers = append(identifiers, commonName)
		}
	}
	return strings.Join(identifiers, "\n"), nil
}

func populateCertificateIdentifiers(certificate *model.Certificate) {
	if certificate == nil {
		return
	}
	identifiers, err := readCertificateIdentifiers(certificate.CertificateFile)
	if err != nil {
		certificate.CertificateIdentifiers = ""
		return
	}
	certificate.CertificateIdentifiers = identifiers
}
