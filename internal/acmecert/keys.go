package acmecert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

func generateCertificateKey(keyType string) (crypto.Signer, error) {
	switch keyType {
	case KeyEC256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case KeyEC384:
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case KeyRSA2048:
		return rsa.GenerateKey(rand.Reader, 2048)
	case KeyRSA4096:
		return rsa.GenerateKey(rand.Reader, 4096)
	default:
		return nil, fmt.Errorf("unsupported certificate key type %q", keyType)
	}
}

func generateAccountKey() (crypto.Signer, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// GenerateAccountPrivateKeyPEM creates a reusable ACME account private key.
func GenerateAccountPrivateKeyPEM() (string, error) {
	key, err := generateAccountKey()
	if err != nil {
		return "", err
	}
	encoded, err := marshalPrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ValidateAccountPrivateKeyPEM verifies that encoded contains a supported signing key.
func ValidateAccountPrivateKeyPEM(encoded string) error {
	_, err := parsePrivateKey([]byte(encoded))
	return err
}

func marshalPrivateKey(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func parsePrivateKey(data []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("private key is not PEM encoded")
	}

	var (
		key any
		err error
	)
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("stored private key cannot sign")
	}
	return signer, nil
}

func validateCertificateKeyType(key crypto.Signer, keyType string) error {
	switch typed := key.(type) {
	case *ecdsa.PrivateKey:
		switch {
		case keyType == KeyEC256 && typed.Curve == elliptic.P256():
			return nil
		case keyType == KeyEC384 && typed.Curve == elliptic.P384():
			return nil
		}
	case *rsa.PrivateKey:
		if (keyType == KeyRSA2048 && typed.N.BitLen() == 2048) ||
			(keyType == KeyRSA4096 && typed.N.BitLen() == 4096) {
			return nil
		}
	}
	return fmt.Errorf("stored certificate key does not match requested key type %q", keyType)
}
