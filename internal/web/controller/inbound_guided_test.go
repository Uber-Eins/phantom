package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/middleware"
)

func TestAddGuidedInboundRequestBindsFrontendPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"inbound": {
			"up": 0,
			"down": 0,
			"total": 0,
			"remark": "",
			"enable": true,
			"expiryTime": 0,
			"trafficReset": "never",
			"lastTrafficResetTime": 0,
			"listen": "/run/xray/VLESS-XHTTP-REALITY",
			"port": 0,
			"protocol": "vless",
			"settings": "{\"clients\":[],\"decryption\":\"none\",\"encryption\":\"none\"}",
			"streamSettings": "{\"network\":\"xhttp\",\"security\":\"reality\",\"xhttpSettings\":{\"path\":\"/guide\"},\"realitySettings\":{\"target\":\"www.amd.com:443\",\"serverNames\":[\"www.amd.com\",\"amd.com\"],\"privateKey\":\"private\",\"shortIds\":[\"0123456789abcdef\"],\"settings\":{\"publicKey\":\"public\",\"fingerprint\":\"chrome\",\"spiderX\":\"/\"}}}",
			"sniffing": "{\"enabled\":false}",
			"tag": "VLESS-XHTTP-REALITY"
		},
		"fronting": {
			"template": "vless-xhttp-reality",
			"decoyMode": "reality-target",
			"decoyValue": ""
		}
	}`)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/panel/api/inbounds/add-guided", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	if _, ok := middleware.BindJSONAndValidate[addGuidedInboundRequest](context); !ok {
		t.Fatalf("guided payload rejected: %s", recorder.Body.String())
	}
}
