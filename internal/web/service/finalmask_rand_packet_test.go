package service

import "testing"

func streamWithNoiseItem(t *testing.T, item map[string]any) map[string]any {
	t.Helper()
	return map[string]any{
		"network": "tcp",
		"finalmask": map[string]any{
			"udp": []any{map[string]any{
				"type": "noise",
				"settings": map[string]any{
					"reset": "60",
					"noise": []any{item},
				},
			}},
		},
	}
}

func noiseItem(t *testing.T, stream map[string]any) map[string]any {
	t.Helper()
	finalmask, _ := stream["finalmask"].(map[string]any)
	udp, _ := finalmask["udp"].([]any)
	mask, _ := udp[0].(map[string]any)
	settings, _ := mask["settings"].(map[string]any)
	noise, _ := settings["noise"].([]any)
	item, _ := noise[0].(map[string]any)
	return item
}

func TestDropEmptyRandPacketsClearsEditorResidue(t *testing.T) {
	stream := streamWithNoiseItem(t, map[string]any{
		"type":   "array",
		"rand":   "1-8192",
		"packet": []any{},
		"delay":  "5",
	})

	if cleared := dropEmptyRandPackets(stream["finalmask"]); cleared != 1 {
		t.Fatalf("cleared = %d, want 1", cleared)
	}
	item := noiseItem(t, stream)
	if _, present := item["packet"]; present {
		t.Fatalf("packet survived: %#v", item)
	}
	if item["rand"] != "1-8192" || item["delay"] != "5" {
		t.Fatalf("healing changed the mask: %#v", item)
	}
}

func TestDropEmptyRandPacketsLeavesRealPacketsAlone(t *testing.T) {
	tests := []struct {
		name string
		item map[string]any
	}{
		{"packet without a rand", map[string]any{"type": "array", "packet": []any{1.0, 2.0}, "rand": 0.0}},
		{"empty packet without a rand", map[string]any{"type": "array", "packet": []any{}}},
		{"empty packet with a zero rand", map[string]any{"type": "array", "packet": []any{}, "rand": 0.0}},
		{"empty packet with a zero range", map[string]any{"type": "array", "packet": []any{}, "rand": "0-0"}},
		{"string packet", map[string]any{"type": "str", "packet": "ping", "rand": "1-10"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := streamWithNoiseItem(t, test.item)
			if cleared := dropEmptyRandPackets(stream["finalmask"]); cleared != 0 {
				t.Fatalf("cleared = %d, want the item left alone", cleared)
			}
			if _, present := noiseItem(t, stream)["packet"]; !present {
				t.Fatal("packet was dropped")
			}
		})
	}
}

func TestDropEmptyRandPacketsReachesNestedItems(t *testing.T) {
	stream := map[string]any{
		"finalmask": map[string]any{
			"tcp": []any{map[string]any{
				"type": "header-custom",
				"settings": map[string]any{
					"clients": []any{[]any{map[string]any{"type": "array", "rand": 64.0, "packet": []any{}}}},
					"servers": []any{[]any{map[string]any{"type": "array", "rand": 32.0, "packet": []any{}}}},
				},
			}},
		},
	}
	if cleared := dropEmptyRandPackets(stream["finalmask"]); cleared != 2 {
		t.Fatalf("cleared = %d, want 2", cleared)
	}
}

func TestDropEmptyRandPacketsIgnoresMissingFinalMask(t *testing.T) {
	stream := map[string]any{"network": "tcp"}
	if cleared := dropEmptyRandPackets(stream["finalmask"]); cleared != 0 {
		t.Fatalf("cleared = %d, want 0", cleared)
	}
}

func TestHealedFinalMaskBuildsInXray(t *testing.T) {
	item := map[string]any{"type": "array", "rand": "1-8192", "packet": []any{}, "delay": "5"}
	stream := streamWithNoiseItem(t, item)
	inbound := map[string]any{
		"tag": "in-vless", "listen": "127.0.0.1", "port": 8443, "protocol": "vless",
		"settings":       map[string]any{"clients": []any{}, "decryption": "none"},
		"streamSettings": stream,
	}
	if err := buildGoldenInbound(t, inbound); err == nil {
		t.Fatal("xray-core is expected to refuse a packet and a rand on one item")
	}

	dropEmptyRandPackets(stream["finalmask"])
	inbound["streamSettings"] = stream
	assertXrayAccepts(t, "the healed noise mask", buildGoldenInbound(t, inbound))
}
