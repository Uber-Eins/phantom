package service

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func settingsClientEmails(t *testing.T, inboundId int) []string {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil {
		t.Fatalf("load inbound %d: %v", inboundId, err)
	}
	clients, err := (&InboundService{}).GetClients(&ib)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	emails := make([]string, 0, len(clients))
	for _, c := range clients {
		emails = append(emails, c.Email)
	}
	return emails
}

// Re-adding a client that is already on the inbound must be an idempotent
// no-op, not a second settings entry: checkEmailsExistForClients exempts a
// matching subId (so one identity can span inbounds), which let retried or
// raced adds duplicate the same email inside one settings array (#5770).
func TestAddInboundClient_SkipsClientsAlreadyOnInbound(t *testing.T) {
	setupBulkDB(t)
	setupLocalRuntime(t)

	alice := model.Client{ID: uuid.NewString(), Email: "alice@dup", SubID: "alice-sub-1234567", Enable: true}
	ib := localInbound(t, 33001, []model.Client{alice})

	svc := &ClientService{}
	inboundSvc := &InboundService{}

	if _, err := svc.AddInboundClient(inboundSvc, &model.Inbound{Id: ib.Id, Protocol: model.VLESS, Settings: clientsSettings(t, []model.Client{alice})}); err != nil {
		t.Fatalf("re-add of existing client should be a no-op, got error: %v", err)
	}
	if emails := settingsClientEmails(t, ib.Id); len(emails) != 1 || emails[0] != "alice@dup" {
		t.Fatalf("settings after duplicate re-add: expected exactly [alice@dup], got %v", emails)
	}

	bob := model.Client{ID: uuid.NewString(), Email: "bob@dup", SubID: "bob-sub-123456789", Enable: true}
	if _, err := svc.AddInboundClient(inboundSvc, &model.Inbound{Id: ib.Id, Protocol: model.VLESS, Settings: clientsSettings(t, []model.Client{alice, bob})}); err != nil {
		t.Fatalf("mixed add (one duplicate, one new): %v", err)
	}
	if emails := settingsClientEmails(t, ib.Id); len(emails) != 2 || emails[0] != "alice@dup" || emails[1] != "bob@dup" {
		t.Fatalf("settings after mixed add: expected [alice@dup bob@dup], got %v", emails)
	}
}
