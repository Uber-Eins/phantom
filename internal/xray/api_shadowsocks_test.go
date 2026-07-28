package xray

import (
	"strings"
	"testing"

	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"google.golang.org/protobuf/proto"
)

func decodeSSAccount(t *testing.T, user map[string]any) (typeURL string, legacy *shadowsocks.Account, modern *shadowsocks_2022.Account) {
	t.Helper()
	tm, err := buildUserAccount("shadowsocks", user)
	if err != nil {
		t.Fatalf("buildUserAccount: %v", err)
	}
	if tm == nil {
		t.Fatal("buildUserAccount returned no account for shadowsocks")
	}
	typeURL = tm.Type
	switch {
	case strings.Contains(typeURL, "shadowsocks_2022"):
		modern = new(shadowsocks_2022.Account)
		if err := proto.Unmarshal(tm.Value, modern); err != nil {
			t.Fatalf("unmarshal shadowsocks_2022 account: %v", err)
		}
	case strings.Contains(typeURL, "shadowsocks"):
		legacy = new(shadowsocks.Account)
		if err := proto.Unmarshal(tm.Value, legacy); err != nil {
			t.Fatalf("unmarshal shadowsocks account: %v", err)
		}
	default:
		t.Fatalf("unexpected account type %q", typeURL)
	}
	return typeURL, legacy, modern
}

func TestBuildUserAccountShadowsocksLegacyCiphers(t *testing.T) {
	tests := []struct {
		cipher string
		want   shadowsocks.CipherType
	}{
		{"aes-128-gcm", shadowsocks.CipherType_AES_128_GCM},
		{"aead_aes_128_gcm", shadowsocks.CipherType_AES_128_GCM},
		{"aes-256-gcm", shadowsocks.CipherType_AES_256_GCM},
		{"AES-256-GCM", shadowsocks.CipherType_AES_256_GCM},
		{"aead_aes_256_gcm", shadowsocks.CipherType_AES_256_GCM},
		{"chacha20-poly1305", shadowsocks.CipherType_CHACHA20_POLY1305},
		{"chacha20-ietf-poly1305", shadowsocks.CipherType_CHACHA20_POLY1305},
		{"aead_chacha20_poly1305", shadowsocks.CipherType_CHACHA20_POLY1305},
		{"xchacha20-poly1305", shadowsocks.CipherType_XCHACHA20_POLY1305},
		{"xchacha20-ietf-poly1305", shadowsocks.CipherType_XCHACHA20_POLY1305},
		{"aead_xchacha20_poly1305", shadowsocks.CipherType_XCHACHA20_POLY1305},
	}
	for _, test := range tests {
		t.Run(test.cipher, func(t *testing.T) {
			user := map[string]any{"email": "a@example.test", "password": "pw", "cipher": test.cipher}
			typeURL, legacy, modern := decodeSSAccount(t, user)
			if modern != nil {
				t.Fatalf("cipher %q built a shadowsocks-2022 account (%s)", test.cipher, typeURL)
			}
			if legacy.CipherType != test.want {
				t.Fatalf("CipherType = %v, want %v", legacy.CipherType, test.want)
			}
			if legacy.Password != "pw" {
				t.Fatalf("Password = %q, want %q", legacy.Password, "pw")
			}
		})
	}
}

func TestBuildUserAccountShadowsocks2022Ciphers(t *testing.T) {
	for _, cipher := range []string{
		"2022-blake3-aes-128-gcm",
		"2022-blake3-aes-256-gcm",
		"2022-blake3-chacha20-poly1305",
	} {
		t.Run(cipher, func(t *testing.T) {
			user := map[string]any{"email": "a@example.test", "password": b64Key(7), "cipher": cipher}
			typeURL, _, modern := decodeSSAccount(t, user)
			if modern == nil {
				t.Fatalf("cipher %q built a legacy account (%s), want shadowsocks-2022", cipher, typeURL)
			}
			if modern.Key != b64Key(7) {
				t.Fatalf("Key = %q, want the client password", modern.Key)
			}
		})
	}
}

func TestBuildUserAccountShadowsocksReadsMethodKey(t *testing.T) {
	user := map[string]any{"email": "a@example.test", "password": "pw", "method": "aes-256-gcm"}
	_, legacy, modern := decodeSSAccount(t, user)
	if modern != nil {
		t.Fatal("a client carrying only method must still build a legacy account")
	}
	if legacy.CipherType != shadowsocks.CipherType_AES_256_GCM {
		t.Fatalf("CipherType = %v, want AES_256_GCM", legacy.CipherType)
	}
}

func TestBuildUserAccountShadowsocksCipherKeyWins(t *testing.T) {
	user := map[string]any{
		"email":    "a@example.test",
		"password": b64Key(3),
		"cipher":   "2022-blake3-aes-128-gcm",
		"method":   "aes-256-gcm",
	}
	_, _, modern := decodeSSAccount(t, user)
	if modern == nil {
		t.Fatal("the explicit cipher must win over a stale method")
	}
}

func TestBuildUserAccountShadowsocksUnknownCipherErrors(t *testing.T) {
	for _, cipher := range []string{"", "rc4-md5", "2022-blake3-future-gcm", "none"} {
		t.Run("cipher="+cipher, func(t *testing.T) {
			user := map[string]any{"email": "a@example.test", "password": "pw"}
			if cipher != "" {
				user["cipher"] = cipher
			}
			account, err := buildUserAccount("shadowsocks", user)
			if err == nil {
				t.Fatalf("cipher %q built account %v, want an error", cipher, account)
			}
			if !strings.Contains(err.Error(), "unknown cipher") {
				t.Fatalf("error = %q, want it to name the unknown cipher", err)
			}
		})
	}
}

func TestBuildUserAccountShadowsocksMissingPassword(t *testing.T) {
	user := map[string]any{"email": "a@example.test", "cipher": "aes-256-gcm"}
	if _, err := buildUserAccount("shadowsocks", user); err == nil {
		t.Fatal("expected an error for a shadowsocks user without a password")
	}
}
