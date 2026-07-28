package acmecert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCertificateUsesSafeUniqueDirectories(t *testing.T) {
	baseDir := t.TempDir()
	manager := NewManager(nil, baseDir)

	certFile, keyFile, err := manager.saveCertificate("../../Panel cert", []byte("cert"), []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(certFile, baseDir+string(filepath.Separator)) ||
		!strings.HasPrefix(keyFile, baseDir+string(filepath.Separator)) {
		t.Fatalf("certificate paths escaped base directory: %q, %q", certFile, keyFile)
	}
	if filepath.Base(filepath.Dir(certFile)) != "panel-cert" {
		t.Fatalf("certificate directory = %q", filepath.Dir(certFile))
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %o, want 600", keyInfo.Mode().Perm())
	}

	secondCert, _, err := manager.saveCertificate("../../Panel cert", []byte("cert"), []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(secondCert) == filepath.Dir(certFile) {
		t.Fatal("second certificate overwrote the first certificate directory")
	}
}

func TestLoadOrCreateAccountKeyReusesStoredKey(t *testing.T) {
	manager := NewManager(nil, t.TempDir())
	first, created, err := manager.loadOrCreateAccountKey(CALetsEncrypt, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first account key was not reported as created")
	}
	second, created, err := manager.loadOrCreateAccountKey(CALetsEncrypt, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("stored account key was unexpectedly recreated")
	}
	if first.Public() == nil || second.Public() == nil {
		t.Fatal("account key is missing a public key")
	}
}

func TestSaveRenewedCertificateReplacesChainAndKeepsPrivateKey(t *testing.T) {
	manager := NewManager(nil, t.TempDir())
	certificateFile, keyFile, err := manager.saveCertificate(
		"renewal",
		[]byte("old certificate"),
		[]byte("existing private key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.saveRenewedCertificate(
		certificateFile,
		keyFile,
		[]byte("renewed certificate"),
	); err != nil {
		t.Fatal(err)
	}
	certificate, err := os.ReadFile(certificateFile)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(certificate) != "renewed certificate" {
		t.Fatalf("certificate file = %q", certificate)
	}
	if string(privateKey) != "existing private key" {
		t.Fatalf("private key file changed: %q", privateKey)
	}
}

func TestManagedCertificateFilesCannotEscapeBaseDirectory(t *testing.T) {
	baseDir := t.TempDir()
	manager := NewManager(nil, baseDir)
	outside := filepath.Join(filepath.Dir(baseDir), "outside.pem")

	err := manager.saveRenewedCertificate(outside, outside, []byte("certificate"))
	if err == nil || !strings.Contains(err.Error(), "outside the managed ACME directory") {
		t.Fatalf("error = %v, want managed-directory validation error", err)
	}
}
