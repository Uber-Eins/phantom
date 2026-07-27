package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/nginxfront"
)

func TestAddGuidedInboundPersistsFrontingAndManagedSocket(t *testing.T) {
	setupConflictDB(t)
	service := &InboundService{}
	inbound := &model.Inbound{
		Enable:   false,
		Remark:   "guided reality",
		Protocol: model.VLESS,
		Settings: `{"clients":[],"decryption":"none","encryption":"none"}`,
		StreamSettings: `{
			"network":"tcp",
			"security":"reality",
			"tcpSettings":{"header":{"type":"none"}},
			"realitySettings":{
				"target":"example.com:443",
				"serverNames":["example.com"],
				"privateKey":"test",
				"shortIds":["0123456789abcdef"],
				"settings":{"publicKey":"test","fingerprint":"chrome","spiderX":"/"}
			}
		}`,
		Sniffing: `{"enabled":false}`,
	}
	fronting := &model.InboundFronting{
		Template:  nginxfront.TemplateVlessTCPReality,
		DecoyMode: nginxfront.DecoyReality,
	}

	created, needRestart, err := service.AddGuidedInbound(inbound, fronting)
	if err != nil {
		t.Fatal(err)
	}
	if needRestart {
		t.Fatal("disabled guided inbound must not request an Xray restart")
	}
	if created.Port != 0 ||
		!nginxfront.IsTemplateSocket(created.Listen, nginxfront.TemplateVlessTCPReality) {
		t.Fatalf("unexpected guided listener: %q:%d", created.Listen, created.Port)
	}
	if created.Tag != "VLESS-TCP-REALITY" {
		t.Fatalf("guided tag = %q", created.Tag)
	}

	var stored model.InboundFronting
	if err := database.GetDB().First(&stored, "inbound_id = ?", created.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Template != nginxfront.TemplateVlessTCPReality {
		t.Fatalf("stored template = %q", stored.Template)
	}

	detail, err := service.GetInboundDetail(created.Id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Fronting == nil || detail.Fronting.DecoyMode != nginxfront.DecoyReality {
		t.Fatalf("fronting marker missing from detail: %#v", detail.Fronting)
	}
}
