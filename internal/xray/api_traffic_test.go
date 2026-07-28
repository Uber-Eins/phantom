package xray

import (
	"context"
	"net"
	"testing"

	statsService "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
)

type fakeStatsServer struct {
	statsService.UnimplementedStatsServiceServer
	rounds [][]*statsService.Stat
	calls  int
}

func (f *fakeStatsServer) QueryStats(context.Context, *statsService.QueryStatsRequest) (*statsService.QueryStatsResponse, error) {
	round := f.calls
	f.calls++
	if round >= len(f.rounds) {
		round = len(f.rounds) - 1
	}
	return &statsService.QueryStatsResponse{Stat: f.rounds[round]}, nil
}

func stat(name string, value int64) *statsService.Stat {
	return &statsService.Stat{Name: name, Value: value}
}

func startFakeStats(t *testing.T, rounds [][]*statsService.Stat) *XrayAPI {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	statsService.RegisterStatsServiceServer(srv, &fakeStatsServer{rounds: rounds})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	api := &XrayAPI{}
	if err := api.Init(lis.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatalf("api init: %v", err)
	}
	t.Cleanup(api.Close)
	return api
}

func clientTrafficByEmail(t *testing.T, traffics []*ClientTraffic) map[string]*ClientTraffic {
	t.Helper()
	byEmail := make(map[string]*ClientTraffic, len(traffics))
	for _, traffic := range traffics {
		byEmail[traffic.Email] = traffic
	}
	return byEmail
}

func TestGetTrafficFirstPollIsBaselineOnly(t *testing.T) {
	api := startFakeStats(t, [][]*statsService.Stat{
		{stat("user>>>alice>>>traffic>>>uplink", 5000)},
	})

	_, clients, err := api.GetTraffic()
	if err != nil {
		t.Fatalf("GetTraffic: %v", err)
	}
	if len(clients) != 0 {
		t.Fatalf("first poll reported %+v, want no traffic", clients[0])
	}
	if got := api.StatsLastValues["user>>>alice>>>traffic>>>uplink"]; got != 5000 {
		t.Fatalf("baseline = %d, want 5000", got)
	}
}

func TestGetTrafficCountsNewStatFromZero(t *testing.T) {
	api := startFakeStats(t, [][]*statsService.Stat{
		{stat("user>>>alice>>>traffic>>>uplink", 100)},
		{
			stat("user>>>alice>>>traffic>>>uplink", 180),
			stat("user>>>bob>>>traffic>>>uplink", 4096),
			stat("user>>>bob>>>traffic>>>downlink", 8192),
		},
	})

	if _, _, err := api.GetTraffic(); err != nil {
		t.Fatalf("GetTraffic (baseline): %v", err)
	}
	_, clients, err := api.GetTraffic()
	if err != nil {
		t.Fatalf("GetTraffic: %v", err)
	}

	byEmail := clientTrafficByEmail(t, clients)
	bob, ok := byEmail["bob"]
	if !ok {
		t.Fatal("a new client's counters reported no traffic")
	}
	if bob.Up != 4096 || bob.Down != 8192 {
		t.Fatalf("bob = up %d / down %d, want 4096 / 8192", bob.Up, bob.Down)
	}
	alice, ok := byEmail["alice"]
	if !ok || alice.Up != 80 {
		t.Fatalf("alice = %+v, want uplink delta 80", alice)
	}
}

func TestGetTrafficCountsAfterCounterReset(t *testing.T) {
	api := startFakeStats(t, [][]*statsService.Stat{
		{stat("user>>>alice>>>traffic>>>uplink", 100)},
		{stat("user>>>alice>>>traffic>>>uplink", 900)},
		{stat("user>>>alice>>>traffic>>>uplink", 250)},
	})

	if _, _, err := api.GetTraffic(); err != nil {
		t.Fatalf("GetTraffic (baseline): %v", err)
	}
	if _, _, err := api.GetTraffic(); err != nil {
		t.Fatalf("GetTraffic (delta): %v", err)
	}
	_, clients, err := api.GetTraffic()
	if err != nil {
		t.Fatalf("GetTraffic (after reset): %v", err)
	}

	alice, ok := clientTrafficByEmail(t, clients)["alice"]
	if !ok || alice.Up != 250 {
		t.Fatalf("alice = %+v, want 250 counted from zero", alice)
	}
}

func TestGetTrafficSkipsAPIInboundAndPrunes(t *testing.T) {
	api := startFakeStats(t, [][]*statsService.Stat{
		{
			stat("inbound>>>api>>>traffic>>>uplink", 10),
			stat("inbound>>>in-443>>>traffic>>>uplink", 10),
			stat("inbound>>>gone-1>>>traffic>>>uplink", 10),
			stat("inbound>>>gone-2>>>traffic>>>uplink", 10),
			stat("inbound>>>gone-3>>>traffic>>>uplink", 10),
			stat("inbound>>>gone-4>>>traffic>>>uplink", 10),
			stat("inbound>>>gone-5>>>traffic>>>uplink", 10),
		},
		{
			stat("inbound>>>api>>>traffic>>>uplink", 99),
			stat("inbound>>>in-443>>>traffic>>>uplink", 60),
			stat("inbound>>>in-443>>>traffic>>>downlink", 70),
		},
	})

	if _, _, err := api.GetTraffic(); err != nil {
		t.Fatalf("GetTraffic (baseline): %v", err)
	}
	tags, _, err := api.GetTraffic()
	if err != nil {
		t.Fatalf("GetTraffic: %v", err)
	}

	if len(tags) != 1 {
		t.Fatalf("got %d tag traffics, want one: %+v", len(tags), tags)
	}
	if tags[0].Tag != "in-443" || !tags[0].IsInbound || tags[0].IsOutbound {
		t.Fatalf("tag traffic = %+v, want inbound in-443", tags[0])
	}
	if tags[0].Up != 50 || tags[0].Down != 70 {
		t.Fatalf("in-443 = up %d / down %d, want 50 / 70", tags[0].Up, tags[0].Down)
	}
	if _, stale := api.StatsLastValues["inbound>>>gone-1>>>traffic>>>uplink"]; stale {
		t.Fatal("baselines for vanished stats were not pruned")
	}
}
