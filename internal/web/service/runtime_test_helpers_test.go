package service

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

type fakeLocalRuntime struct {
	addInbound    atomic.Int64
	delInbound    atomic.Int64
	updateInbound atomic.Int64
	addUser       atomic.Int64
	removeUser    atomic.Int64
}

func (f *fakeLocalRuntime) Name() string { return "fake-local" }

func (f *fakeLocalRuntime) AddInbound(context.Context, *model.Inbound) error {
	f.addInbound.Add(1)
	return nil
}

func (f *fakeLocalRuntime) DelInbound(context.Context, *model.Inbound) error {
	f.delInbound.Add(1)
	return nil
}

func (f *fakeLocalRuntime) UpdateInbound(context.Context, *model.Inbound, *model.Inbound) error {
	f.updateInbound.Add(1)
	return nil
}

func (f *fakeLocalRuntime) AddUser(context.Context, *model.Inbound, map[string]any) error {
	f.addUser.Add(1)
	return nil
}

func (f *fakeLocalRuntime) RemoveUser(context.Context, *model.Inbound, string) error {
	f.removeUser.Add(1)
	return nil
}

func (f *fakeLocalRuntime) RestartXray(context.Context) error { return nil }

func (f *fakeLocalRuntime) ResetClientTraffic(context.Context, *model.Inbound, string) error {
	return nil
}

func (f *fakeLocalRuntime) ResetInboundTraffic(context.Context, *model.Inbound) error {
	return nil
}

func (f *fakeLocalRuntime) ResetAllTraffics(context.Context) error { return nil }

func setupLocalRuntime(t *testing.T) *fakeLocalRuntime {
	t.Helper()
	fake := &fakeLocalRuntime{}
	manager := runtime.NewManager(runtime.LocalDeps{APIPort: func() int { return 0 }})
	manager.SetLocalRuntimeOverride(fake)
	runtime.SetManager(manager)
	t.Cleanup(func() { runtime.SetManager(nil) })
	return fake
}

func localInbound(t *testing.T, port int, clients []model.Client) *model.Inbound {
	t.Helper()
	settings, err := json.Marshal(map[string]any{"clients": clients, "decryption": "none"})
	if err != nil {
		t.Fatalf("marshal clients: %v", err)
	}
	inbound := &model.Inbound{
		UserId:   1,
		Tag:      baseInboundTag(port) + "-tcp",
		Enable:   true,
		Port:     port,
		Protocol: model.VLESS,
		Settings: string(settings),
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatalf("create local inbound: %v", err)
	}
	if err := (&ClientService{}).SyncInbound(database.GetDB(), inbound.Id, clients); err != nil {
		t.Fatalf("sync local inbound clients: %v", err)
	}
	return inbound
}
