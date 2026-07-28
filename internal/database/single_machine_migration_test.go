package database

import (
	"encoding/json"
	"testing"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	"github.com/Uber-Eins/phantom/v3/internal/xray"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyNode struct {
	Id      int `gorm:"primaryKey;autoIncrement"`
	Name    string
	Address string
	Port    int
}

func (legacyNode) TableName() string { return "nodes" }

type legacyHost struct {
	Id        int `gorm:"primaryKey;autoIncrement"`
	InboundId int
	GroupId   string
	Remark    string
}

func (legacyHost) TableName() string { return "hosts" }

type legacyClientGroup struct {
	Id   int `gorm:"primaryKey;autoIncrement"`
	Name string
}

func (legacyClientGroup) TableName() string { return "client_groups" }

type legacyAPIToken struct {
	Id    int `gorm:"primaryKey;autoIncrement"`
	Name  string
	Token string
}

func (legacyAPIToken) TableName() string { return "api_tokens" }

type legacyExternalLink struct {
	Id       int `gorm:"primaryKey;autoIncrement"`
	ClientId int
	Kind     string
	Value    string
}

func (legacyExternalLink) TableName() string { return "client_external_links" }

type legacyNodeTraffic struct {
	Id int `gorm:"primaryKey;autoIncrement"`
}

func (legacyNodeTraffic) TableName() string { return "node_client_traffics" }

type legacyNodeIP struct {
	Id int `gorm:"primaryKey;autoIncrement"`
}

func (legacyNodeIP) TableName() string { return "node_client_ips" }

type legacyGlobalTraffic struct {
	Id int `gorm:"primaryKey;autoIncrement"`
}

func (legacyGlobalTraffic) TableName() string { return "client_global_traffics" }

type legacyInboundIP struct {
	Id          int `gorm:"primaryKey;autoIncrement"`
	ClientEmail string
	Ips         string
}

func (legacyInboundIP) TableName() string { return "inbound_client_ips" }

func TestMigrateSingleMachineSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/migration.db"), &gorm.Config{
		Logger:                                   logger.Discard,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	testSingleMachineMigration(t, db)
}

func singleMachineTestModels() []any {
	return []any{
		&model.User{},
		&model.Setting{},
		&model.Inbound{},
		&model.ClientRecord{},
		&model.ClientInbound{},
		&xray.ClientTraffic{},
		&model.InboundFallback{},
		&legacyNode{},
		&legacyHost{},
		&legacyClientGroup{},
		&legacyAPIToken{},
		&legacyExternalLink{},
		&legacyNodeTraffic{},
		&legacyNodeIP{},
		&legacyGlobalTraffic{},
		&legacyInboundIP{},
	}
}

func testSingleMachineMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(singleMachineTestModels()...); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	nodeID := 9
	local := &model.Inbound{
		UserId:         1,
		Tag:            "local",
		Port:           443,
		Protocol:       model.VLESS,
		Settings:       `{"clients":[{"id":"uuid","email":"alice","subId":"stable","group":"friends","tgId":42,"limitIp":2}]}`,
		StreamSettings: `{"network":"ws","externalProxy":[{"dest":"remote.example"}]}`,
	}
	remote := &model.Inbound{
		UserId:         1,
		Tag:            "remote",
		Port:           8443,
		Protocol:       model.VLESS,
		Settings:       `{"clients":[{"id":"uuid","email":"alice","subId":"stable"}]}`,
		StreamSettings: `{}`,
	}
	if err := db.Create(local).Error; err != nil {
		t.Fatalf("create local inbound: %v", err)
	}
	if err := db.Create(remote).Error; err != nil {
		t.Fatalf("create remote inbound: %v", err)
	}
	if err := db.Table("inbounds").Where("id = ?", local.Id).Updates(map[string]any{
		"sub_sort_index":      7,
		"share_addr_strategy": "custom",
		"share_addr":          "share.example",
		"origin_node_guid":    "old-guid",
	}).Error; err != nil {
		t.Fatalf("seed local historical columns: %v", err)
	}
	if err := db.Table("inbounds").Where("id = ?", remote.Id).Update("node_id", nodeID).Error; err != nil {
		t.Fatalf("mark remote inbound: %v", err)
	}

	alice := &model.ClientRecord{
		Email:  "alice",
		SubID:  "stable",
		UUID:   "uuid",
		Enable: true,
	}
	if err := db.Create(alice).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Table("clients").Where("id = ?", alice.Id).Updates(map[string]any{
		"limit_ip":   2,
		"tg_id":      42,
		"group_name": "friends",
	}).Error; err != nil {
		t.Fatalf("seed client historical columns: %v", err)
	}
	if err := db.Create([]model.ClientInbound{
		{ClientId: alice.Id, InboundId: local.Id},
		{ClientId: alice.Id, InboundId: remote.Id},
	}).Error; err != nil {
		t.Fatalf("attach client: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{
		InboundId: remote.Id,
		Email:     "alice",
		Up:        123,
		Down:      456,
		Enable:    true,
	}).Error; err != nil {
		t.Fatalf("create traffic: %v", err)
	}
	if err := db.Create(&model.InboundFallback{MasterId: local.Id, ChildId: remote.Id}).Error; err != nil {
		t.Fatalf("create fallback: %v", err)
	}
	if err := db.Create(&legacyNode{Id: nodeID, Name: "old-node", Address: "127.0.0.1", Port: 2053}).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := db.Create(&legacyHost{InboundId: local.Id, GroupId: "g", Remark: "override"}).Error; err != nil {
		t.Fatalf("create host: %v", err)
	}
	if err := db.Create(&legacyClientGroup{Name: "friends"}).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.Create(&legacyAPIToken{Name: "legacy", Token: "hash"}).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := db.Create(&legacyExternalLink{ClientId: alice.Id, Kind: "subscription", Value: "https://example.invalid/sub"}).Error; err != nil {
		t.Fatalf("create external link: %v", err)
	}
	if err := db.Create(&legacyInboundIP{ClientEmail: "alice", Ips: "[]"}).Error; err != nil {
		t.Fatalf("create IP record: %v", err)
	}
	for _, setting := range []model.Setting{
		{Key: "webPort", Value: "2053"},
		{Key: "twoFactorEnable", Value: "true"},
		{Key: "tgBotEnable", Value: "true"},
		{Key: "smtpEnable", Value: "true"},
		{Key: "ldapEnable", Value: "true"},
		{Key: "subPort", Value: "2096"},
		{Key: "nodeMtlsCaKeyPem", Value: "secret"},
		{Key: "apiToken", Value: "secret"},
		{Key: "remarkTemplate", Value: "{remark}-{email}"},
	} {
		if err := db.Create(&setting).Error; err != nil {
			t.Fatalf("create setting %s: %v", setting.Key, err)
		}
	}

	for run := 1; run <= 2; run++ {
		if err := MigrateSingleMachine(db); err != nil {
			t.Fatalf("migration run %d: %v", run, err)
		}
	}

	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		t.Fatalf("list inbounds: %v", err)
	}
	if len(inbounds) != 1 || inbounds[0].Id != local.Id {
		t.Fatalf("inbounds after cleanup = %#v, want only local id %d", inbounds, local.Id)
	}
	var inboundLegacy struct {
		NodeID            *int   `gorm:"column:node_id"`
		SubSortIndex      int    `gorm:"column:sub_sort_index"`
		ShareAddrStrategy string `gorm:"column:share_addr_strategy"`
		ShareAddr         string `gorm:"column:share_addr"`
		OriginNodeGuid    string `gorm:"column:origin_node_guid"`
	}
	if err := db.Table("inbounds").Where("id = ?", local.Id).Scan(&inboundLegacy).Error; err != nil {
		t.Fatalf("read historical inbound columns: %v", err)
	}
	if inboundLegacy.NodeID != nil || inboundLegacy.SubSortIndex != 0 ||
		inboundLegacy.ShareAddrStrategy != "" || inboundLegacy.ShareAddr != "" ||
		inboundLegacy.OriginNodeGuid != "" {
		t.Fatalf("legacy inbound columns not cleared: %#v", inboundLegacy)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(inbounds[0].Settings), &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	client := settings["clients"].([]any)[0].(map[string]any)
	for _, key := range []string{"group", "tgId", "limitIp"} {
		if _, exists := client[key]; exists {
			t.Errorf("settings client still contains %q: %#v", key, client)
		}
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(inbounds[0].StreamSettings), &stream); err != nil {
		t.Fatalf("decode stream settings: %v", err)
	}
	if _, exists := stream["externalProxy"]; exists {
		t.Errorf("stream settings still contain externalProxy: %#v", stream)
	}

	var gotClient model.ClientRecord
	if err := db.Where("email = ?", "alice").First(&gotClient).Error; err != nil {
		t.Fatalf("client was not preserved: %v", err)
	}
	if gotClient.SubID != "stable" {
		t.Errorf("client cleanup = %#v", gotClient)
	}
	var clientLegacy struct {
		Group   string `gorm:"column:group_name"`
		TgID    int64  `gorm:"column:tg_id"`
		LimitIP int    `gorm:"column:limit_ip"`
	}
	if err := db.Table("clients").Where("id = ?", gotClient.Id).Scan(&clientLegacy).Error; err != nil {
		t.Fatalf("read historical client columns: %v", err)
	}
	if clientLegacy.Group != "" || clientLegacy.TgID != 0 || clientLegacy.LimitIP != 0 {
		t.Errorf("historical client columns not cleared: %#v", clientLegacy)
	}
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", "alice").First(&traffic).Error; err != nil {
		t.Fatalf("traffic was not preserved: %v", err)
	}
	if traffic.InboundId != local.Id || traffic.Up != 123 || traffic.Down != 456 {
		t.Errorf("traffic after migration = %#v", traffic)
	}

	for _, table := range []string{
		"nodes", "hosts", "client_groups", "api_tokens", "client_external_links",
		"node_client_traffics", "node_client_ips", "client_global_traffics", "inbound_client_ips",
	} {
		if db.Migrator().HasTable(table) {
			t.Errorf("removed table %q still exists", table)
		}
	}
	var keptSettings []model.Setting
	if err := db.Find(&keptSettings).Error; err != nil {
		t.Fatalf("list settings: %v", err)
	}
	if len(keptSettings) != 2 {
		t.Fatalf("settings after cleanup = %#v, want webPort and twoFactorEnable", keptSettings)
	}
}
