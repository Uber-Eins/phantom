package nginxfront

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfig(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())

	content, err := ReadConfig()
	if err != nil || content != "" {
		t.Fatalf("missing config returned (%q, %v)", content, err)
	}

	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	const expected = "events {}\n"
	if err := os.WriteFile(filepath.Join(ConfigDir(), "nginx.conf"), []byte(expected), 0o600); err != nil {
		t.Fatal(err)
	}

	content, err = ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if content != expected {
		t.Fatalf("config = %q, want %q", content, expected)
	}
}
