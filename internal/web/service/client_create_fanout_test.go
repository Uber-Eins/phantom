package service

import (
	"strings"
	"testing"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func TestCreateDoesNotGenerateSubscriptionID(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}
	inbound := mkInbound(t, 23000, model.VLESS, `{"clients":[]}`)

	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client: model.Client{
			Email:  "without-sub-id@x",
			ID:     "aaaaaaaa-1111-2222-3333-444444444444",
			Enable: true,
		},
		InboundIds: []int{inbound.Id},
	}); err != nil {
		t.Fatal(err)
	}

	record := lookupClientRecord(t, "without-sub-id@x")
	if record.SubID != "" {
		t.Fatalf("generated client record subId = %q", record.SubID)
	}
	reloaded, err := inboundSvc.GetInbound(inbound.Id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reloaded.Settings, `"subId"`) {
		t.Fatalf("new inbound client settings contain subId: %s", reloaded.Settings)
	}
}

func TestCreateAcrossManyInboundsUsesOneEmailSnapshot(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}

	const uuid = "bbbbbbbb-1111-2222-3333-555555555555"
	ids := make([]int, 0, 6)
	for i := range 6 {
		ib := mkInbound(t, 23001+i, model.VLESS, `{"clients":[]}`)
		ids = append(ids, ib.Id)
	}

	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "fan@x", ID: uuid, SubID: "sub-fan", Enable: true},
		InboundIds: ids,
	}); err != nil {
		t.Fatalf("Create across %d inbounds: %v", len(ids), err)
	}

	if n := countClientRecords(t); n != 1 {
		t.Fatalf("client records = %d, want 1", n)
	}
	rec := lookupClientRecord(t, "fan@x")
	if rec.UUID != uuid || rec.SubID != "sub-fan" {
		t.Fatalf("record = {uuid:%q sub:%q}, want {%q sub-fan}", rec.UUID, rec.SubID, uuid)
	}
	for _, id := range ids {
		if !settingsHoldUUID(t, inboundSvc, id, uuid) {
			t.Fatalf("inbound %d settings missing the client", id)
		}
	}

	linked, err := svc.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		t.Fatalf("GetInboundIdsForRecord: %v", err)
	}
	if len(linked) != len(ids) {
		t.Fatalf("linked inbounds = %d, want %d", len(linked), len(ids))
	}
}

func TestAttachAcrossManyInboundsUsesOneEmailSnapshot(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}

	first := mkInbound(t, 23101, model.VLESS, `{"clients":[]}`)
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "att@x", ID: "cccccccc-1111-2222-3333-666666666666", SubID: "sub-att", Enable: true},
		InboundIds: []int{first.Id},
	}); err != nil {
		t.Fatalf("seed Create: %v", err)
	}
	rec := lookupClientRecord(t, "att@x")

	ids := []int{first.Id}
	for i := range 4 {
		ib := mkInbound(t, 23102+i, model.VLESS, `{"clients":[]}`)
		ids = append(ids, ib.Id)
	}

	if _, err := svc.Attach(inboundSvc, rec.Id, ids); err != nil {
		t.Fatalf("Attach across %d inbounds: %v", len(ids), err)
	}

	if n := countClientRecords(t); n != 1 {
		t.Fatalf("client records after attach = %d, want 1", n)
	}
	linked, err := svc.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		t.Fatalf("GetInboundIdsForRecord: %v", err)
	}
	if len(linked) != len(ids) {
		t.Fatalf("linked inbounds = %d, want %d", len(linked), len(ids))
	}
}
