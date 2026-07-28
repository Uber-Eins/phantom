package service

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/acmecert"
	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

func TestCertificateAutoRenewUsesPersistedTokensAndConfiguredSchedules(t *testing.T) {
	setupSettingTestDB(t)
	now := time.Date(2026, time.January, 2, 6, 0, 0, 0, time.UTC)
	key, err := acmecert.GenerateAccountPrivateKeyPEM()
	if err != nil {
		t.Fatal(err)
	}
	service := CertificateService{}
	if err := service.SaveConfig(&CertificateConfig{
		RenewBeforeDays:       30,
		ShortRenewBeforeHours: 24,
		ShortCheckTimesPerDay: 4,
		CheckTime:             "05:00:00",
		GlobalPrivateKey:      key,
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Setting{Key: "timeLocation", Value: "UTC"}).Error; err != nil {
		t.Fatal(err)
	}

	certificates := []*model.Certificate{
		testCertificate("regular-due", "regular-token", now.Add(-60*24*time.Hour), now.Add(20*24*time.Hour)),
		testCertificate("short-due", "short-token", now.Add(-140*time.Hour), now.Add(20*time.Hour)),
		testCertificate("regular-not-due", "unused-token", now.Add(-50*24*time.Hour), now.Add(40*24*time.Hour)),
	}
	if err := database.GetDB().Create(&certificates).Error; err != nil {
		t.Fatal(err)
	}

	var renewedRemarks []string
	var renewedTokens []string
	fake := &fakeCertificateIssuer{
		renew: func(
			_ context.Context,
			request acmecert.IssueRequest,
			certificateFile string,
			keyFile string,
		) (*acmecert.IssueResult, error) {
			renewedRemarks = append(renewedRemarks, request.Remark)
			renewedTokens = append(renewedTokens, request.CloudflareToken)
			validity := 90 * 24 * time.Hour
			if request.Remark == "short-due" {
				validity = 160 * time.Hour
			}
			return &acmecert.IssueResult{
				Remark:          request.Remark,
				Identifiers:     []string{"example.com"},
				CertificateFile: certificateFile,
				KeyFile:         keyFile,
				IssuedAt:        now,
				ExpiresAt:       now.Add(validity),
			}, nil
		},
	}
	service.issuerFactory = func(_ *http.Client, _ string, accountKey string) (certificateIssuer, error) {
		if accountKey != strings.TrimSpace(key) {
			t.Fatalf("renewal account key changed")
		}
		return fake, nil
	}

	renewed, err := service.AutoRenew(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if renewed != 2 {
		t.Fatalf("renewed = %d, want 2", renewed)
	}
	if !slices.Equal(renewedRemarks, []string{"regular-due", "short-due"}) {
		t.Fatalf("renewed certificates = %#v", renewedRemarks)
	}
	if !slices.Equal(renewedTokens, []string{"regular-token", "short-token"}) {
		t.Fatalf("renewal tokens = %#v", renewedTokens)
	}

	renewed, err = service.AutoRenew(context.Background(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if renewed != 0 {
		t.Fatalf("second schedule pass renewed %d certificates", renewed)
	}
}

func TestCertificateCheckSchedules(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	now := time.Date(2026, time.January, 2, 5, 0, 0, 0, location)
	if regularCertificateCheckDue(now.Add(-time.Second), time.Time{}, "05:00:00") {
		t.Fatal("regular check ran before configured time")
	}
	if !regularCertificateCheckDue(now, time.Time{}, "05:00:00") {
		t.Fatal("regular check did not run at configured time")
	}
	if regularCertificateCheckDue(now, now.Add(-time.Hour), "05:00:00") {
		t.Fatal("regular check ran twice on the same local date")
	}

	lastShortCheck := now.Add(-6 * time.Hour)
	if !shortCertificateCheckDue(now, lastShortCheck, 4) {
		t.Fatal("short-lived check was not due after its interval")
	}
	if shortCertificateCheckDue(now.Add(-time.Second), lastShortCheck, 4) {
		t.Fatal("short-lived check ran before its interval")
	}
}

func testCertificate(
	remark string,
	token string,
	issuedAt time.Time,
	expiresAt time.Time,
) *model.Certificate {
	return &model.Certificate{
		Remark:           remark,
		AddMethod:        acmecert.AddMethodACME,
		CA:               acmecert.CALetsEncrypt,
		ValidationMethod: acmecert.ValidationCloudflare,
		CloudflareToken:  token,
		Identifiers:      "example.com",
		Email:            "admin@example.com",
		KeyType:          acmecert.KeyEC256,
		CertificateType:  acmecert.CertificateDomain,
		CertificateFile:  "/managed/" + remark + "/fullchain.pem",
		KeyFile:          "/managed/" + remark + "/privkey.pem",
		IssuedAt:         issuedAt,
		ExpiresAt:        expiresAt,
		ValidityHours:    certificateValidityHours(issuedAt, expiresAt),
	}
}
