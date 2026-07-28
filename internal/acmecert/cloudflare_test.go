package acmecert

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareClientCreatesAndDeletesTXTRecord(t *testing.T) {
	var created struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	deleted := false

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer scoped-token" {
			t.Errorf("authorization header = %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/zones":
			if request.URL.Query().Get("name") != "example.com" {
				t.Errorf("zone query = %q", request.URL.Query().Get("name"))
			}
			_, _ = writer.Write([]byte(`{"success":true,"result":[{"id":"zone-id","name":"example.com"}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/zones/zone-id/dns_records":
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				t.Errorf("decode record: %v", err)
			}
			_, _ = writer.Write([]byte(`{"success":true,"result":{"id":"record-id"}}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/zones/zone-id/dns_records/record-id":
			deleted = true
			_, _ = writer.Write([]byte(`{"success":true,"result":{"id":"record-id"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := &cloudflareClient{
		baseURL:    server.URL,
		token:      "scoped-token",
		httpClient: server.Client(),
	}
	zoneID, recordID, err := client.createTXT(
		context.Background(),
		"www.example.com",
		"_acme-challenge.www.example.com",
		"challenge-value",
	)
	if err != nil {
		t.Fatal(err)
	}
	if zoneID != "zone-id" || recordID != "record-id" {
		t.Fatalf("record identity = %q/%q", zoneID, recordID)
	}
	if created.Type != "TXT" ||
		created.Name != "_acme-challenge.www.example.com" ||
		created.Content != "challenge-value" {
		t.Fatalf("created record = %#v", created)
	}
	if err := client.deleteRecord(context.Background(), zoneID, recordID); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DNS challenge record was not deleted")
	}
}

func TestCloudflareClientDoesNotExposeTokenInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Invalid token"}]}`))
	}))
	defer server.Close()

	client := &cloudflareClient{
		baseURL:    server.URL,
		token:      "do-not-leak-this-token",
		httpClient: server.Client(),
	}
	_, err := client.findZone(context.Background(), "example.com")
	if err == nil {
		t.Fatal("expected Cloudflare API error")
	}
	if strings.Contains(err.Error(), client.token) {
		t.Fatalf("error leaked token: %v", err)
	}
	if !strings.Contains(err.Error(), "Invalid token") {
		t.Fatalf("error = %v, want Cloudflare API detail", err)
	}
}
