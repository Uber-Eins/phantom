package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/acmecert"
)

func TestCertificateConfigDefaultsAndPersistence(t *testing.T) {
	setupSettingTestDB(t)
	service := CertificateService{}

	defaults, err := service.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if defaults.RenewBeforeDays != 30 ||
		defaults.ShortRenewBeforeHours != 24 ||
		defaults.ShortCheckTimesPerDay != 4 ||
		defaults.CheckTime != "05:00:00" {
		t.Fatalf("unexpected certificate defaults: %#v", defaults)
	}
	if err := acmecert.ValidateAccountPrivateKeyPEM(defaults.GlobalPrivateKey); err != nil {
		t.Fatalf("default global account key is invalid: %v", err)
	}

	want := &CertificateConfig{
		RenewBeforeDays:       20,
		ShortRenewBeforeHours: 36,
		ShortCheckTimesPerDay: 6,
		CheckTime:             "06:30:15",
		DefaultEmail:          "admin@example.com",
		GlobalPrivateKey:      defaults.GlobalPrivateKey,
	}
	if err := service.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := service.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Fatalf("saved config = %#v, want %#v", got, want)
	}
}

func TestCertificateConfigRejectsInvalidValues(t *testing.T) {
	key, err := acmecert.GenerateAccountPrivateKeyPEM()
	if err != nil {
		t.Fatal(err)
	}
	tests := []CertificateConfig{
		{
			RenewBeforeDays:       0,
			ShortRenewBeforeHours: 24,
			ShortCheckTimesPerDay: 4,
			CheckTime:             "05:00:00",
			GlobalPrivateKey:      key,
		},
		{
			RenewBeforeDays:       30,
			ShortRenewBeforeHours: 23,
			ShortCheckTimesPerDay: 4,
			CheckTime:             "05:00:00",
			GlobalPrivateKey:      key,
		},
		{
			RenewBeforeDays:       30,
			ShortRenewBeforeHours: 24,
			ShortCheckTimesPerDay: 7,
			CheckTime:             "05:00:00",
			GlobalPrivateKey:      key,
		},
		{
			RenewBeforeDays:       30,
			ShortRenewBeforeHours: 24,
			ShortCheckTimesPerDay: 4,
			CheckTime:             "25:00:00",
			GlobalPrivateKey:      key,
		},
		{
			RenewBeforeDays:       30,
			ShortRenewBeforeHours: 24,
			ShortCheckTimesPerDay: 4,
			CheckTime:             "05:00:00",
			DefaultEmail:          "invalid",
			GlobalPrivateKey:      key,
		},
		{
			RenewBeforeDays:       30,
			ShortRenewBeforeHours: 24,
			ShortCheckTimesPerDay: 4,
			CheckTime:             "05:00:00",
			GlobalPrivateKey:      "not a key",
		},
	}
	for index := range tests {
		if err := validateCertificateConfig(&tests[index]); err == nil {
			t.Fatalf("invalid config %d was accepted", index)
		}
	}
}
