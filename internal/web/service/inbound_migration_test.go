package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	"github.com/Uber-Eins/phantom/v3/internal/xray"
)

func TestMigrationRequirementsBackfillsClientTraffic(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const email = "needsbackfill@example.com"
	inbound := &model.Inbound{
		UserId:   1,
		Tag:      "backfill",
		Enable:   true,
		Port:     30001,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"email":"` + email + `","id":"ce8d33df-3a64-4f10-8f9b-91c3a8e0c010","enable":true,"group":"old","tgId":42,"limitIp":2}]}`,
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	(&InboundService{}).MigrationRequirements()

	var traffic xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", email).First(&traffic).Error; err != nil {
		t.Fatalf("client traffic not backfilled: %v", err)
	}
	var got model.Inbound
	if err := database.GetDB().First(&got, inbound.Id).Error; err != nil {
		t.Fatalf("reload inbound: %v", err)
	}
	for _, removed := range []string{`"group"`, `"tgId"`, `"limitIp"`} {
		if strings.Contains(got.Settings, removed) {
			t.Fatalf("legacy client key %s survived: %s", removed, got.Settings)
		}
	}
}

func TestMigrationRequirementsCleansLegacyTag(t *testing.T) {
	setupConflictDB(t)
	legacy := &model.Inbound{
		UserId:   1,
		Tag:      "inbound-0.0.0.0:30002",
		Enable:   true,
		Port:     30002,
		Protocol: model.VLESS,
		Settings: `{"clients":[]}`,
	}
	if err := database.GetDB().Create(legacy).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	(&InboundService{}).MigrationRequirements()
	var got model.Inbound
	if err := database.GetDB().First(&got, legacy.Id).Error; err != nil {
		t.Fatalf("reload inbound: %v", err)
	}
	if got.Tag != "inbound-30002" {
		t.Fatalf("tag = %q, want inbound-30002", got.Tag)
	}
}
