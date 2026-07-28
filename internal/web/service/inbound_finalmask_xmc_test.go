package service

import (
	"encoding/json"
	"testing"

	"github.com/xtls/xray-core/infra/conf"
)

const completeXmcProfile = `{"username":"Notch","uuid":"069a79f4-44e9-4726-a5be-fca90e38aaf5","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}`

func TestValidateFinalMaskXmcProfiles(t *testing.T) {
	tests := []struct {
		name           string
		streamSettings string
		wantErr        bool
	}{
		{name: "empty streamSettings", streamSettings: ""},
		{name: "no finalmask", streamSettings: `{"network":"tcp","security":"none"}`},
		{
			name:           "non-XMC mask",
			streamSettings: `{"finalmask":{"tcp":[{"type":"fragment","settings":{"packets":"tlshello"}}]}}`,
		},
		{
			name:           "complete profile",
			streamSettings: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[` + completeXmcProfile + `]}}]}}`,
		},
		{
			name:           "legacy usernames",
			streamSettings: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"usernames":["Dream"]}}]}}`,
			wantErr:        true,
		},
		{
			name:           "empty profiles",
			streamSettings: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[]}}]}}`,
			wantErr:        true,
		},
		{
			name:           "missing signature",
			streamSettings: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[{"username":"Notch","uuid":"069a79f4-44e9-4726-a5be-fca90e38aaf5","texturesValue":"dmFsdWU=","texturesSignature":""}]}}]}}`,
			wantErr:        true,
		},
		{
			name:           "invalid UUID",
			streamSettings: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[{"username":"Notch","uuid":"invalid","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}]}}]}}`,
			wantErr:        true,
		},
		{
			name:           "invalid username",
			streamSettings: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[{"username":"ab","uuid":"069a79f4-44e9-4726-a5be-fca90e38aaf5","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}]}}]}}`,
			wantErr:        true,
		},
		{
			name:           "one incomplete profile",
			streamSettings: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[` + completeXmcProfile + `,{"username":"Herobrine","uuid":"","texturesValue":"","texturesSignature":""}]}}]}}`,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFinalMaskXmcProfiles(tt.streamSettings)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestXmcMaskProfilesCompleteMatchesCoreValidation(t *testing.T) {
	profiles := []struct {
		name string
		raw  string
	}{
		{name: "complete", raw: completeXmcProfile},
		{name: "undashed UUID", raw: `{"username":"Notch","uuid":"069a79f444e94726a5befca90e38aaf5","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}`},
		{name: "16 character username", raw: `{"username":"Abcdefghijklmnop","uuid":"069a79f4-44e9-4726-a5be-fca90e38aaf5","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}`},
		{name: "long username", raw: `{"username":"Abcdefghijklmnopq","uuid":"069a79f4-44e9-4726-a5be-fca90e38aaf5","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}`},
		{name: "hyphenated username", raw: `{"username":"No-tch","uuid":"069a79f4-44e9-4726-a5be-fca90e38aaf5","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}`},
		{name: "empty UUID", raw: `{"username":"Notch","uuid":"","texturesValue":"dmFsdWU=","texturesSignature":"c2ln"}`},
		{name: "missing textures value", raw: `{"username":"Notch","uuid":"069a79f4-44e9-4726-a5be-fca90e38aaf5","texturesValue":"","texturesSignature":"c2ln"}`},
	}

	for _, tt := range profiles {
		t.Run(tt.name, func(t *testing.T) {
			var coreProfile conf.XMCProfile
			if err := json.Unmarshal([]byte(tt.raw), &coreProfile); err != nil {
				t.Fatalf("unmarshal core profile: %v", err)
			}
			_, coreErr := coreProfile.Build()

			var settings map[string]any
			if err := json.Unmarshal([]byte(`{"profiles":[`+tt.raw+`]}`), &settings); err != nil {
				t.Fatalf("unmarshal panel settings: %v", err)
			}
			panelAccepts := xmcMaskProfilesComplete(map[string]any{
				"type":     "xmc",
				"settings": settings,
			})
			coreAccepts := coreErr == nil
			if panelAccepts != coreAccepts {
				t.Errorf("panel accepts = %v, core accepts = %v (error %v)", panelAccepts, coreAccepts, coreErr)
			}
		})
	}
}

func TestXmcEmptyProfilesRejectedByCore(t *testing.T) {
	var core conf.XMC
	if err := json.Unmarshal([]byte(`{"hostname":"mc.example.com","password":"pw","profiles":[]}`), &core); err != nil {
		t.Fatalf("unmarshal XMC settings: %v", err)
	}
	if _, err := core.Build(); err == nil {
		t.Fatal("conf.XMC.Build accepted an empty profiles list")
	}
}

func TestStripIncompleteXmcMasks(t *testing.T) {
	tests := []struct {
		name        string
		stream      string
		wantDropped int
		wantStream  string
	}{
		{
			name:        "legacy mask removes empty finalmask",
			stream:      `{"network":"tcp","finalmask":{"tcp":[{"type":"xmc","settings":{"usernames":["Dream"]}}]}}`,
			wantDropped: 1,
			wantStream:  `{"network":"tcp"}`,
		},
		{
			name:       "complete mask survives",
			stream:     `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[` + completeXmcProfile + `]}}]}}`,
			wantStream: `{"finalmask":{"tcp":[{"type":"xmc","settings":{"profiles":[` + completeXmcProfile + `]}}]}}`,
		},
		{
			name:        "sibling TCP mask survives",
			stream:      `{"finalmask":{"tcp":[{"type":"xmc","settings":{}},{"type":"fragment","settings":{"packets":"tlshello"}}]}}`,
			wantDropped: 1,
			wantStream:  `{"finalmask":{"tcp":[{"type":"fragment","settings":{"packets":"tlshello"}}]}}`,
		},
		{
			name:        "UDP masks survive",
			stream:      `{"finalmask":{"tcp":[{"type":"xmc","settings":{}}],"udp":[{"type":"salamander"}]}}`,
			wantDropped: 1,
			wantStream:  `{"finalmask":{"udp":[{"type":"salamander"}]}}`,
		},
		{
			name:       "stream without finalmask",
			stream:     `{"network":"tcp","security":"tls"}`,
			wantStream: `{"network":"tcp","security":"tls"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stream map[string]any
			if err := json.Unmarshal([]byte(tt.stream), &stream); err != nil {
				t.Fatalf("unmarshal stream: %v", err)
			}
			if dropped := stripIncompleteXmcMasks(stream); dropped != tt.wantDropped {
				t.Errorf("dropped = %d, want %d", dropped, tt.wantDropped)
			}
			var want map[string]any
			if err := json.Unmarshal([]byte(tt.wantStream), &want); err != nil {
				t.Fatalf("unmarshal expected stream: %v", err)
			}
			gotJSON, _ := json.Marshal(stream)
			wantJSON, _ := json.Marshal(want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("stream = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}
