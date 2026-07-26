package service

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const sampleXrayDigest = `# Hash Values

MD5= ee4e2ff74948a9b464624b1cabc44409
SHA1= b55b06e74e89083b9cedfdecf0d68b579cd2af72
SHA2-256= 23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae
SHA2-512= e8bc40a0687cac184bbe4b5c1f047e69064ccedc489fb25e208889ae287bbf8736dff16b108d68fc00dc33edc8bb53502e47a9698a277f4f51b67b83d899e518
`

const sampleXraySHA256 = "23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae"

func TestSupportedXrayVersion(t *testing.T) {
	for _, test := range []struct {
		version string
		want    bool
	}{
		{"v26.6.26", false},
		{"v26.6.27", true},
		{"v26.7.11", true},
		{"v27.1.0", true},
		{"latest", false},
	} {
		t.Run(test.version, func(t *testing.T) {
			if got := supportedXrayVersion(test.version); got != test.want {
				t.Fatalf("supportedXrayVersion(%q) = %v, want %v", test.version, got, test.want)
			}
		})
	}
}

func TestParseXrayDigestSHA256(t *testing.T) {
	got, err := parseXrayDigestSHA256([]byte(sampleXrayDigest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != sampleXraySHA256 {
		t.Fatalf("sha = %q, want %q", got, sampleXraySHA256)
	}
}

func TestParseXrayDigestSHA256Errors(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
	}{
		{"missing", "MD5= abc\n"},
		{"short", "SHA2-256= deadbeef\n"},
		{"non-hex", "SHA2-256= zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseXrayDigestSHA256([]byte(test.data)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestFetchXrayDigestSHA256(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleXrayDigest))
	}))
	defer server.Close()

	got, err := (&ServerService{}).fetchXrayDigestSHA256(server.Client(), server.URL+"/Xray-linux-64.zip.dgst")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got != sampleXraySHA256 {
		t.Fatalf("sha = %q, want %q", got, sampleXraySHA256)
	}
}

func TestFetchXrayDigestSHA256HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := (&ServerService{}).fetchXrayDigestSHA256(server.Client(), server.URL+"/missing.dgst"); err == nil {
		t.Fatal("expected an error on HTTP 404")
	}
}

func TestStageXrayBinary(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "xray.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("xray")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("new-xray")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	targetPath := filepath.Join(t.TempDir(), "bin", "xray-linux-amd64")
	stagedPath, err := stageXrayBinary(archivePath, "xray", targetPath)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	defer os.Remove(stagedPath)
	data, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-xray" {
		t.Fatalf("staged contents = %q", data)
	}
	info, err := os.Stat(stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("staged mode = %o, want 755", info.Mode().Perm())
	}
}
