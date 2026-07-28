package acmecert

import "testing"

func TestGenerateAccountPrivateKeyPEMRoundTrip(t *testing.T) {
	encoded, err := GenerateAccountPrivateKeyPEM()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAccountPrivateKeyPEM(encoded); err != nil {
		t.Fatalf("generated account private key is invalid: %v", err)
	}

	manager := NewManager(nil, t.TempDir())
	if err := manager.SetAccountPrivateKeyPEM(encoded); err != nil {
		t.Fatal(err)
	}
	if manager.accountKey == nil {
		t.Fatal("manager did not retain the global account private key")
	}
}
