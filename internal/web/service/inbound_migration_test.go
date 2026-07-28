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

func TestMigrationRemoveOrphanedTraffics(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	clientSvc := &ClientService{}
	inboundSvc := &InboundService{}

	const attachedEmail = "attached@example.com"
	attachedClient := model.Client{Email: attachedEmail, ID: "11111111-1111-1111-1111-111111111111", SubID: attachedEmail, Enable: true}
	attachedInbound := mkInbound(t, 30003, model.VLESS, clientsSettings(t, []model.Client{attachedClient}))
	if err := clientSvc.SyncInbound(nil, attachedInbound.Id, []model.Client{attachedClient}); err != nil {
		t.Fatalf("seed attached client: %v", err)
	}
	mkTraffic(t, attachedInbound.Id, attachedEmail, 0, 0, 0, 0, true)

	const detachedEmail = "detached@example.com"
	detachedClient := model.Client{Email: detachedEmail, ID: "22222222-2222-2222-2222-222222222222", SubID: detachedEmail, Enable: true}
	detachedInbound := mkInbound(t, 30004, model.VLESS, clientsSettings(t, []model.Client{detachedClient}))
	if err := clientSvc.SyncInbound(nil, detachedInbound.Id, []model.Client{detachedClient}); err != nil {
		t.Fatalf("seed detached client: %v", err)
	}
	mkTraffic(t, detachedInbound.Id, detachedEmail, 123, 456, 0, 0, true)
	detachedRecord := lookupClientRecord(t, detachedEmail)
	if _, err := clientSvc.Detach(inboundSvc, detachedRecord.Id, []int{detachedInbound.Id}); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	const jsonOnlyEmail = "jsononly@example.com"
	jsonOnlyClient := model.Client{Email: jsonOnlyEmail, ID: "33333333-3333-3333-3333-333333333333", SubID: jsonOnlyEmail, Enable: true}
	jsonOnlyInbound := mkInbound(t, 30005, model.VLESS, clientsSettings(t, []model.Client{jsonOnlyClient}))
	mkTraffic(t, jsonOnlyInbound.Id, jsonOnlyEmail, 0, 0, 0, 0, true)

	const trulyOrphanedEmail = "deleted@example.com"
	mkTraffic(t, attachedInbound.Id, trulyOrphanedEmail, 0, 0, 0, 0, true)

	inboundSvc.MigrationRemoveOrphanedTraffics()

	cases := []struct {
		name  string
		email string
		want  int64
	}{
		{"attached, in clients table and JSON", attachedEmail, 1},
		{"detached-but-alive, in clients table only", detachedEmail, 1},
		{"seeder-skipped-but-live, in JSON only", jsonOnlyEmail, 1},
		{"truly orphaned, in neither", trulyOrphanedEmail, 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var got int64
			if err := db.Model(xray.ClientTraffic{}).Where("email = ?", test.email).Count(&got).Error; err != nil {
				t.Fatalf("count client_traffics for %s: %v", test.email, err)
			}
			if got != test.want {
				t.Errorf("client_traffics count for %s: got %d, want %d", test.email, got, test.want)
			}
		})
	}
}
