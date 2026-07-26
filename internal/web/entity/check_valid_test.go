package entity

import "testing"

func validSettings() *AllSetting {
	return &AllSetting{
		WebPort:       2053,
		WebBasePath:   "/",
		SessionMaxAge: 360,
		TimeLocation:  "UTC",
	}
}

func TestAllSettingCheckValid(t *testing.T) {
	settings := validSettings()
	if err := settings.CheckValid(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
}

func TestAllSettingCheckValidRejectsInvalidListen(t *testing.T) {
	settings := validSettings()
	settings.WebListen = "not-an-ip"
	if err := settings.CheckValid(); err == nil {
		t.Fatal("invalid listen address accepted")
	}
}

func TestAllSettingCheckValidNormalizesBasePath(t *testing.T) {
	settings := validSettings()
	settings.WebBasePath = "panel"
	if err := settings.CheckValid(); err != nil {
		t.Fatal(err)
	}
	if settings.WebBasePath != "/panel/" {
		t.Fatalf("base path = %q", settings.WebBasePath)
	}
}
