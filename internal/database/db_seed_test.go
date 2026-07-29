package database

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func TestSeedClientsFromInboundJSON_IsIdempotentAgainstExistingClients(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	settings, err := json.Marshal(map[string]any{
		"clients": []any{
			map[string]any{
				"id":      "ce8d33df-3a64-4f10-8f9b-91c3a8e0c001",
				"email":   "alice@example.com",
				"enable":  true,
				"flow":    "",
				"subId":   "alice-sub",
				"comment": "from-inbound-json",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	inbound := model.Inbound{
		UserId:   1,
		Port:     12345,
		Protocol: model.VLESS,
		Settings: string(settings),
		Tag:      "test-inbound",
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}

	preExisting := &model.ClientRecord{
		Email:   "alice@example.com",
		UUID:    "ce8d33df-3a64-4f10-8f9b-91c3a8e0c001",
		SubID:   "alice-sub",
		Enable:  true,
		Comment: "added-via-api",
	}
	if err := db.Create(preExisting).Error; err != nil {
		t.Fatalf("seed client row: %v", err)
	}

	if err := db.Where("seeder_name = ?", "ClientsTable").Delete(&model.HistoryOfSeeders{}).Error; err != nil {
		t.Fatalf("clear ClientsTable history: %v", err)
	}

	if err := seedClientsFromInboundJSON(); err != nil {
		t.Fatalf("seedClientsFromInboundJSON should be idempotent against existing rows, got: %v", err)
	}

	var count int64
	if err := db.Model(&model.ClientRecord{}).Where("email = ?", "alice@example.com").Count(&count).Error; err != nil {
		t.Fatalf("count clients: %v", err)
	}
	if count != 1 {
		t.Fatalf("alice@example.com should resolve to exactly one row, got %d", count)
	}
}

func TestRunSeeders_DoesNotCreateInboundClientSubIds(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	settings, err := json.Marshal(map[string]any{
		"clients": []any{
			map[string]any{
				"id":    "00000000-0000-0000-0000-000000000001",
				"email": "missing-sub@example.com",
				"subId": "",
			},
			map[string]any{
				"id":    "00000000-0000-0000-0000-000000000002",
				"email": "no-sub-key@example.com",
			},
			map[string]any{
				"id":    "00000000-0000-0000-0000-000000000003",
				"email": "has-sub@example.com",
				"subId": "keep-me-1234",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	inbound := model.Inbound{
		UserId:   1,
		Port:     23456,
		Protocol: model.VLESS,
		Settings: string(settings),
		Tag:      "subid-fix-inbound",
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}

	if err := runSeeders(false); err != nil {
		t.Fatalf("runSeeders: %v", err)
	}

	var reloaded model.Inbound
	if err := db.First(&reloaded, inbound.Id).Error; err != nil {
		t.Fatalf("reload inbound: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(reloaded.Settings), &parsed); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	clients, ok := parsed["clients"].([]any)
	if !ok || len(clients) != 3 {
		t.Fatalf("expected 3 clients, got %v", parsed["clients"])
	}

	first := clients[0].(map[string]any)
	if sub, _ := first["subId"].(string); sub != "" {
		t.Fatalf("empty legacy subId was changed to %q", sub)
	}
	if _, exists := clients[1].(map[string]any)["subId"]; exists {
		t.Fatal("missing legacy subId was created")
	}
	preserved := clients[2].(map[string]any)["subId"].(string)
	if preserved != "keep-me-1234" {
		t.Fatalf("expected existing subId preserved, got %q", preserved)
	}

	var historyCount int64
	if err := db.Model(&model.HistoryOfSeeders{}).Where("seeder_name = ?", "InboundClientSubIdFix").Count(&historyCount).Error; err != nil {
		t.Fatalf("count seeder history: %v", err)
	}
	if historyCount != 0 {
		t.Fatalf("unexpected InboundClientSubIdFix history row count: %d", historyCount)
	}
}

func TestNormalizeSettingPaths_RepairsLegacyValues(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	seed := []model.Setting{
		{Key: "webBasePath", Value: "panel"},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed setting %s: %v", seed[i].Key, err)
		}
	}

	if err := normalizeSettingPaths(); err != nil {
		t.Fatalf("normalizeSettingPaths: %v", err)
	}

	want := map[string]string{
		"webBasePath": "/panel/",
	}
	for key, expected := range want {
		var row model.Setting
		if err := db.Where("key = ?", key).First(&row).Error; err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		if row.Value != expected {
			t.Errorf("%s = %q, want %q", key, row.Value, expected)
		}
	}
}
