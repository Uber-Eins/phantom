package service

import (
	"encoding/json"
	"testing"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func seedVlessInbound(t *testing.T, tag string, port int, clients []model.Client) {
	t.Helper()
	setupSettingTestDB(t)
	db := database.GetDB()
	inbound := &model.Inbound{
		Tag:      tag,
		Enable:   true,
		Port:     port,
		Protocol: model.VLESS,
		Settings: `{"clients":[],"decryption":"none"}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create vless inbound: %v", err)
	}
	svc := ClientService{}
	if err := svc.SyncInbound(nil, inbound.Id, clients); err != nil {
		t.Fatalf("SyncInbound: %v", err)
	}
}

// ClientRecord.Enable carries gorm:"default:true", so production disables
// clients through an UPDATE; mirror that here.
func disableClients(t *testing.T, emails ...string) {
	t.Helper()
	if err := database.GetDB().Model(&model.ClientRecord{}).
		Where("email IN ?", emails).
		Update("enable", false).Error; err != nil {
		t.Fatalf("disable clients: %v", err)
	}
}

func emittedClients(t *testing.T, tag string) (any, bool) {
	t.Helper()
	svc := &XrayService{}
	cfg, err := svc.GetXrayConfig()
	if err != nil {
		t.Fatalf("GetXrayConfig: %v", err)
	}
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Tag != tag {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(cfg.InboundConfigs[i].Settings), &settings); err != nil {
			t.Fatalf("unmarshal emitted settings: %v", err)
		}
		value, ok := settings["clients"]
		return value, ok
	}
	t.Fatalf("inbound %q not found in generated config", tag)
	return nil, false
}

func TestGetXrayConfigEmptyClientListIsArrayNotNull(t *testing.T) {
	seedVlessInbound(t, "vless-empty", 43101, []model.Client{
		{Email: "gone@x", ID: "11111111-1111-1111-1111-111111111111", Enable: true},
	})
	disableClients(t, "gone@x")

	value, present := emittedClients(t, "vless-empty")
	if !present {
		t.Fatal("the clients key must be present on a vless inbound")
	}
	if value == nil {
		t.Fatal("settings.clients must be [] when every client is filtered out, got null")
	}
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("settings.clients must be an array, got %T", value)
	}
	if len(list) != 0 {
		t.Fatalf("a disabled client must not reach the config, got %d entries", len(list))
	}
}

func TestGetXrayConfigEnabledClientsStillEmitted(t *testing.T) {
	seedVlessInbound(t, "vless-mixed", 43102, []model.Client{
		{Email: "live@x", ID: "22222222-2222-2222-2222-222222222222", Enable: true},
		{Email: "off@x", ID: "33333333-3333-3333-3333-333333333333", Enable: true},
	})
	disableClients(t, "off@x")

	value, _ := emittedClients(t, "vless-mixed")
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("settings.clients must be an array, got %T", value)
	}
	if len(list) != 1 {
		t.Fatalf("expected only the enabled client, got %d entries: %#v", len(list), list)
	}
	entry, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("client entry must be an object, got %T", list[0])
	}
	if entry["email"] != "live@x" {
		t.Errorf("wrong client emitted: %#v", entry)
	}
	if entry["id"] != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("client id not carried through: %#v", entry)
	}
}
