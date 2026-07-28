package service

import (
	"path/filepath"
	"testing"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/xray"
	"gorm.io/gorm"
)

func setupSettingTestDB(t *testing.T) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func initTrafficTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	return database.GetDB()
}

func readTraffic(t *testing.T, db *gorm.DB, email string) xray.ClientTraffic {
	t.Helper()
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", email).First(&traffic).Error; err != nil {
		t.Fatalf("read client traffic %q: %v", email, err)
	}
	return traffic
}
