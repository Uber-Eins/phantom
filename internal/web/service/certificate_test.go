package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/acmecert"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type fakeCertificateIssuer struct {
	issue func(context.Context, acmecert.IssueRequest) (*acmecert.IssueResult, error)
	renew func(context.Context, acmecert.IssueRequest, string, string) (*acmecert.IssueResult, error)
}

func (f *fakeCertificateIssuer) Issue(
	ctx context.Context,
	request acmecert.IssueRequest,
) (*acmecert.IssueResult, error) {
	return f.issue(ctx, request)
}

func (f *fakeCertificateIssuer) Renew(
	ctx context.Context,
	request acmecert.IssueRequest,
	certificateFile string,
	keyFile string,
) (*acmecert.IssueResult, error) {
	return f.renew(ctx, request, certificateFile, keyFile)
}

func TestCertificateIssuePersistsRenewalInputsWithoutExposingToken(t *testing.T) {
	setupSettingTestDB(t)
	issuedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	var accountPrivateKey string
	fake := &fakeCertificateIssuer{
		issue: func(_ context.Context, request acmecert.IssueRequest) (*acmecert.IssueResult, error) {
			if request.CloudflareToken != "  persisted-token  " {
				t.Fatalf("issue token = %q", request.CloudflareToken)
			}
			return &acmecert.IssueResult{
				Remark:          request.Remark,
				Identifiers:     []string{"example.com", "*.example.com"},
				CertificateFile: "/managed/fullchain.pem",
				KeyFile:         "/managed/privkey.pem",
				IssuedAt:        issuedAt,
				ExpiresAt:       issuedAt.Add(90 * 24 * time.Hour),
			}, nil
		},
	}
	service := CertificateService{
		issuerFactory: func(_ *http.Client, _ string, key string) (certificateIssuer, error) {
			accountPrivateKey = key
			return fake, nil
		},
	}

	certificate, err := service.Issue(context.Background(), acmecert.IssueRequest{
		Remark:           "example.com",
		AddMethod:        acmecert.AddMethodACME,
		CA:               acmecert.CALetsEncrypt,
		ValidationMethod: acmecert.ValidationCloudflare,
		CloudflareToken:  "  persisted-token  ",
		Identifiers:      "example.com\n*.example.com",
		Email:            "admin@example.com",
		KeyType:          acmecert.KeyEC256,
		CertificateType:  acmecert.CertificateDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := acmecert.ValidateAccountPrivateKeyPEM(accountPrivateKey); err != nil {
		t.Fatalf("issuer received invalid global account key: %v", err)
	}

	var stored model.Certificate
	if err := database.GetDB().First(&stored, certificate.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CloudflareToken != "persisted-token" {
		t.Fatalf("stored Cloudflare token = %q", stored.CloudflareToken)
	}
	if stored.Identifiers != "example.com\n*.example.com" {
		t.Fatalf("stored identifiers = %q", stored.Identifiers)
	}

	encoded, err := json.Marshal(&stored)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(stored.CloudflareToken)) {
		t.Fatalf("certificate JSON exposed Cloudflare token: %s", encoded)
	}
}
