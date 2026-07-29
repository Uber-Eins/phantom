package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/acmecert"
	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func writeCertificateFiles(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "ignored.example.com"},
		DNSNames:     []string{"actual.example.com", "*.actual.example.com"},
		IPAddresses:  []net.IP{net.ParseIP("203.0.113.8")},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "managed")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	certificateFile := filepath.Join(dir, "fullchain.pem")
	keyFile := filepath.Join(dir, "privkey.pem")
	if err := os.WriteFile(
		certificateFile,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		keyFile,
		pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		}),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return certificateFile, keyFile
}

func TestCertificateListUpdateAndDelete(t *testing.T) {
	setupSettingTestDB(t)
	certificateFile, keyFile := writeCertificateFiles(t)
	row := &model.Certificate{
		Remark:           "old",
		AddMethod:        acmecert.AddMethodACME,
		CA:               acmecert.CALetsEncrypt,
		ValidationMethod: acmecert.ValidationCloudflare,
		CloudflareToken:  "stored-token",
		Identifiers:      "stale-input.example.com",
		Email:            "old@example.com",
		KeyType:          acmecert.KeyEC256,
		CertificateType:  acmecert.CertificateDomain,
		CertificateFile:  certificateFile,
		KeyFile:          keyFile,
		IssuedAt:         time.Now().Add(-time.Hour),
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}
	if err := database.GetDB().Create(row).Error; err != nil {
		t.Fatal(err)
	}

	service := CertificateService{}
	listed, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	const identifiers = "actual.example.com\n*.actual.example.com\n203.0.113.8"
	if len(listed) != 1 ||
		listed[0].Identifiers != "stale-input.example.com" ||
		listed[0].CertificateIdentifiers != identifiers {
		t.Fatalf("listed identifiers = %#v, want %q", listed, identifiers)
	}

	updated, err := service.Update(row.Id, acmecert.IssueRequest{
		Remark:           "updated",
		AddMethod:        acmecert.AddMethodACME,
		CA:               acmecert.CAZeroSSL,
		ValidationMethod: acmecert.ValidationCloudflare,
		Identifiers:      "next.example.com",
		Email:            "next@example.com",
		KeyType:          acmecert.KeyRSA2048,
		CertificateType:  acmecert.CertificateDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Remark != "updated" ||
		updated.Identifiers != "next.example.com" ||
		updated.CertificateIdentifiers != identifiers {
		t.Fatalf("updated certificate = %#v", updated)
	}
	var stored model.Certificate
	if err := database.GetDB().First(&stored, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CloudflareToken != "stored-token" {
		t.Fatalf("blank update replaced stored token with %q", stored.CloudflareToken)
	}
	if stored.Identifiers != "next.example.com" {
		t.Fatalf("stored renewal identifiers = %q", stored.Identifiers)
	}

	if err := service.Delete(row.Id); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{certificateFile, keyFile} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("managed file %s still exists (err=%v)", path, err)
		}
	}
	var count int64
	if err := database.GetDB().Model(&model.Certificate{}).Where("id = ?", row.Id).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("certificate row still exists after delete")
	}
}
