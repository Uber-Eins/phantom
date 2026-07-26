package service

import (
	"testing"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Deleting a client that is attached to more than one inbound must still remove
// the user from the running runtime of the inbound being deleted from. The
// runtime user is keyed by inbound tag, so a sibling inbound still carrying the
// same email (emailShared) must not suppress the per-inbound runtime removal —
// otherwise the deleted user keeps connecting on that inbound until Xray
// restart (#5543).
func TestDelInboundClientByEmail_SharedEmailStillRemovesFromRuntime(t *testing.T) {
	setupBulkDB(t)
	fake := setupLocalRuntime(t)

	shared := []model.Client{{ID: uuid.NewString(), Email: "shared@x", Enable: true}}
	ibA := localInbound(t, 31001, shared)
	localInbound(t, 31002, shared)

	svc := &ClientService{}
	inboundSvc := &InboundService{}

	if _, err := svc.DelInboundClientByEmail(inboundSvc, ibA.Id, "shared@x", false, false); err != nil {
		t.Fatalf("DelInboundClientByEmail: %v", err)
	}

	if got := fake.removeUser.Load(); got != 1 {
		t.Fatalf("shared-email delete dispatched %d RemoveUser calls, want 1", got)
	}
}
