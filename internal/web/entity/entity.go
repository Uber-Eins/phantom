package entity

import (
	"crypto/tls"
	"math"
	"net"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

type Msg struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
	Obj     any    `json:"obj"`
}

type AllSetting struct {
	WebListen         string `json:"webListen" form:"webListen"`
	WebDomain         string `json:"webDomain" form:"webDomain"`
	WebPort           int    `json:"webPort" form:"webPort" validate:"gte=1,lte=65535"`
	WebCertFile       string `json:"webCertFile" form:"webCertFile"`
	WebKeyFile        string `json:"webKeyFile" form:"webKeyFile"`
	WebBasePath       string `json:"webBasePath" form:"webBasePath"`
	SessionMaxAge     int    `json:"sessionMaxAge" form:"sessionMaxAge" validate:"gte=1,lte=525600"`
	TrustedProxyCIDRs string `json:"trustedProxyCIDRs" form:"trustedProxyCIDRs"`
	PanelOutbound     string `json:"panelOutbound" form:"panelOutbound"`

	PageSize    int    `json:"pageSize" form:"pageSize" validate:"gte=0,lte=1000"`
	ExpireDiff  int    `json:"expireDiff" form:"expireDiff" validate:"gte=0"`
	TrafficDiff int    `json:"trafficDiff" form:"trafficDiff" validate:"gte=0,lte=100"`
	Datepicker  string `json:"datepicker" form:"datepicker"`

	TimeLocation    string `json:"timeLocation" form:"timeLocation"`
	TwoFactorEnable bool   `json:"twoFactorEnable" form:"twoFactorEnable"`
	TwoFactorToken  string `json:"twoFactorToken" form:"twoFactorToken"`

	RestartXrayOnClientDisable bool `json:"restartXrayOnClientDisable" form:"restartXrayOnClientDisable"`

	WarpUpdateInterval int `json:"warpUpdateInterval" form:"warpUpdateInterval" validate:"gte=0"`
}

type AllSettingView struct {
	AllSetting

	HasTwoFactorToken bool `json:"hasTwoFactorToken"`
	HasWarpSecret     bool `json:"hasWarpSecret"`
	HasNordSecret     bool `json:"hasNordSecret"`
}

func pathHasForbiddenChar(s string) bool {
	for _, r := range s {
		if r == '\\' || r == ' ' || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func (s *AllSetting) CheckValid() error {
	if s.WebListen != "" {
		ip := net.ParseIP(s.WebListen)
		if ip == nil {
			return common.NewError("web listen is not valid ip:", s.WebListen)
		}
	}

	if s.WebPort <= 0 || s.WebPort > math.MaxUint16 {
		return common.NewError("web port is not a valid port:", s.WebPort)
	}

	if s.WebCertFile != "" || s.WebKeyFile != "" {
		_, err := tls.LoadX509KeyPair(s.WebCertFile, s.WebKeyFile)
		if err != nil {
			return common.NewErrorf("cert file <%v> or key file <%v> invalid: %v", s.WebCertFile, s.WebKeyFile, err)
		}
	}

	if pathHasForbiddenChar(s.WebBasePath) {
		return common.NewError("URI path contains an invalid character: web base path")
	}

	if !strings.HasPrefix(s.WebBasePath, "/") {
		s.WebBasePath = "/" + s.WebBasePath
	}
	if !strings.HasSuffix(s.WebBasePath, "/") {
		s.WebBasePath += "/"
	}
	for cidr := range strings.SplitSeq(s.TrustedProxyCIDRs, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if ip := net.ParseIP(cidr); ip != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return common.NewError("trusted proxy CIDR is not valid:", cidr)
		}
	}

	_, err := time.LoadLocation(s.TimeLocation)
	if err != nil {
		return common.NewError("time location not exist:", s.TimeLocation)
	}

	return nil
}
