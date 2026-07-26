package share

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/goccy/go-json"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	wgutil "github.com/mhsanaei/3x-ui/v3/internal/util/wireguard"
)

// genWireguardLink builds a per-client wireguard:// share link mirroring the
// frontend genWireguardLink: the client's private key is the userinfo, the
// server public key (derived from the inbound secretKey) and the client's
// tunnel address ride in the query. Returns "" when the client has no key.
func (s *LinkService) genWireguardLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.WireGuard {
		return ""
	}
	settings := s.linkSettings(inbound)
	secretKey, _ := settings["secretKey"].(string)

	resolved, ok := s.clientForLink(inbound, email)
	if !ok || resolved.PrivateKey == "" {
		return ""
	}
	client := &resolved

	link := fmt.Sprintf("wireguard://%s@%s", encodeUserinfo(client.PrivateKey), joinHostPort(s.resolveInboundAddress(inbound), inbound.Port))
	params := make(map[string]string)
	if secretKey != "" {
		if pub, err := wgutil.PublicKeyFromPrivate(secretKey); err == nil {
			params["publickey"] = pub
		}
	}
	if joined := strings.Join(client.AllowedIPs, ","); joined != "" {
		params["address"] = joined
	}
	if mtu, ok := settings["mtu"].(float64); ok && mtu > 0 {
		params["mtu"] = strconv.Itoa(int(mtu))
	}
	if dns, ok := settings["dns"].(string); ok && dns != "" {
		params["dns"] = dns
	}
	if client.PreSharedKey != "" {
		params["presharedkey"] = client.PreSharedKey
	}
	if client.KeepAlive > 0 {
		params["keepalive"] = strconv.Itoa(client.KeepAlive)
	}
	return buildLinkWithParams(link, params, s.genRemark(inbound, email))
}

// genMtprotoLink builds a per-client Telegram proxy deep link for an mtproto
// inbound: the server/port pair plus the client's own FakeTLS secret. The link
// carries no remark fragment — Telegram proxy deep links have no name field, and
// a trailing "#remark" is appended to the last query value by lenient parsers,
// corrupting the server address. The remark is shown separately in the panel UI.
// Returns "" when the client has no secret.
func (s *LinkService) genMtprotoLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.MTProto {
		return ""
	}
	resolved, ok := s.clientForLink(inbound, email)
	if !ok || resolved.Secret == "" {
		return ""
	}
	params := map[string]string{
		"server": s.resolveInboundAddress(inbound),
		"port":   fmt.Sprintf("%d", inbound.Port),
		"secret": resolved.Secret,
	}
	return buildLinkWithParams("tg://proxy", params, "")
}

func (s *LinkService) genVmessLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.VMESS {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	obj := map[string]any{
		"v":    "2",
		"add":  address,
		"port": inbound.Port,
		"type": "none",
	}
	stream := unmarshalStreamSettings(inbound.StreamSettings)
	network, _ := stream["network"].(string)
	applyVmessNetworkParams(stream, network, obj)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskObj(finalmask, obj)
	}
	security, _ := stream["security"].(string)
	obj["tls"] = security
	if security == "tls" {
		applyVmessTLSParams(stream, obj)
	}

	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	obj["id"] = client.ID
	obj["scy"] = normalizeVmessSecurity(client.Security)
	obj["ps"] = s.genRemark(inbound, email)
	return buildVmessLink(obj)
}

// normalizeVmessSecurity maps the vmess security values xray-core v26.7.11
// removed ("none"/"zero"), plus the legacy empty string, to "auto" so links
// stop advertising values the upgraded server rejects on the wire.
func normalizeVmessSecurity(security string) string {
	switch security {
	case "", "none", "zero":
		return "auto"
	}
	return security
}

func vlessEncryptionEnabled(settings map[string]any) bool {
	for _, key := range []string{"encryption", "decryption"} {
		if v, ok := settings[key].(string); ok && v != "" && v != "none" {
			return true
		}
	}
	return false
}

// vlessFlowAllowed reports whether a client's XTLS Vision flow belongs in
// generated links. Vision runs on TCP with TLS/Reality, and on XHTTP whenever
// VLESS encryption is enabled.
func vlessFlowAllowed(network, security string, settings map[string]any) bool {
	switch network {
	case "tcp":
		return security == "tls" || security == "reality"
	case "xhttp":
		return vlessEncryptionEnabled(settings)
	}
	return false
}

func (s *LinkService) genVlessLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.VLESS {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	stream := unmarshalStreamSettings(inbound.StreamSettings)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	streamNetwork, _ := stream["network"].(string)
	params := map[string]string{"type": streamNetwork}

	settings := s.linkSettings(inbound)
	if encryption, ok := settings["encryption"].(string); ok {
		params["encryption"] = encryption
	}

	applyShareNetworkParams(stream, streamNetwork, params)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskParams(finalmask, params)
	}
	security, _ := stream["security"].(string)
	switch security {
	case "tls":
		applyShareTLSParams(stream, params)
	case "reality":
		applyShareRealityParams(stream, params, subKey(client))
	default:
		params["security"] = "none"
	}
	if client.Flow != "" && vlessFlowAllowed(streamNetwork, security, settings) {
		params["flow"] = client.Flow
	}

	link := fmt.Sprintf("vless://%s@%s", client.ID, joinHostPort(address, inbound.Port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email))
}

func (s *LinkService) genTrojanLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.Trojan {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	stream := unmarshalStreamSettings(inbound.StreamSettings)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	streamNetwork, _ := stream["network"].(string)
	params := map[string]string{"type": streamNetwork}

	applyShareNetworkParams(stream, streamNetwork, params)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskParams(finalmask, params)
	}
	security, _ := stream["security"].(string)
	switch security {
	case "tls":
		applyShareTLSParams(stream, params)
	case "reality":
		applyShareRealityParams(stream, params, subKey(client))
		if streamNetwork == "tcp" && client.Flow != "" {
			params["flow"] = client.Flow
		}
	default:
		params["security"] = "none"
	}

	link := fmt.Sprintf("trojan://%s@%s", encodeUserinfo(client.Password), joinHostPort(address, inbound.Port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email))
}

func encodeUserinfo(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func joinHostPort(host string, port int) string {
	return net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
}

func (s *LinkService) genShadowsocksLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.Shadowsocks {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	stream := unmarshalStreamSettings(inbound.StreamSettings)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}

	settings := s.linkSettings(inbound)
	inboundPassword, _ := settings["password"].(string)
	method, _ := settings["method"].(string)
	streamNetwork, _ := stream["network"].(string)
	params := map[string]string{"type": streamNetwork}

	applyShareNetworkParams(stream, streamNetwork, params)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskParams(finalmask, params)
	}
	if security, _ := stream["security"].(string); security == "tls" {
		applyShareTLSParams(stream, params)
	}

	if streamNetwork == "tcp" && params["headerType"] == "http" {
		host := params["host"]
		delete(params, "type")
		delete(params, "headerType")
		delete(params, "host")
		delete(params, "path")
		params["plugin"] = "obfs-local;obfs=http;obfs-host=" + host
	}

	var userInfo string
	if strings.HasPrefix(method, "2022") {
		userInfo = fmt.Sprintf("%s:%s:%s",
			url.QueryEscape(method),
			url.QueryEscape(inboundPassword),
			url.QueryEscape(client.Password))
	} else {
		userInfo = base64.RawURLEncoding.EncodeToString(fmt.Appendf(nil, "%s:%s", method, client.Password))
	}

	link := fmt.Sprintf("ss://%s@%s", userInfo, joinHostPort(address, inbound.Port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email))
}

func (s *LinkService) genHysteriaLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.Hysteria {
		return ""
	}
	var stream map[string]any
	_ = json.Unmarshal([]byte(inbound.StreamSettings), &stream)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	params := map[string]string{"security": "tls"}

	tlsSetting, _ := stream["tlsSettings"].(map[string]any)
	alpns, _ := tlsSetting["alpn"].([]any)
	var alpn []string
	for _, value := range alpns {
		if text, ok := value.(string); ok {
			alpn = append(alpn, text)
		}
	}
	if len(alpn) > 0 {
		params["alpn"] = strings.Join(alpn, ",")
	}
	if sniValue, ok := searchKey(tlsSetting, "serverName"); ok {
		params["sni"], _ = sniValue.(string)
	}

	tlsSettings, _ := searchKey(tlsSetting, "settings")
	if tlsSetting != nil {
		if fpValue, ok := searchKey(tlsSettings, "fingerprint"); ok {
			params["fp"], _ = fpValue.(string)
		}
		if echValue, ok := searchKey(tlsSettings, "echConfigList"); ok {
			if ech, _ := echValue.(string); ech != "" {
				params["ech"] = ech
			}
		}
		if vcn, ok := verifyPeerCertByNameValue(tlsSettings); ok {
			params["vcn"] = vcn
		}
		if pins, ok := pinnedSha256List(tlsSettings); ok {
			for i, pin := range pins {
				pins[i] = hysteriaPinHex(pin)
			}
			params["pinSHA256"] = strings.Join(pins, ",")
		}
	}

	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		if udpMasks, ok := finalmask["udp"].([]any); ok {
			for _, rawMask := range udpMasks {
				mask, _ := rawMask.(map[string]any)
				if mask == nil || mask["type"] != "salamander" {
					continue
				}
				maskSettings, _ := mask["settings"].(map[string]any)
				if password, ok := maskSettings["password"].(string); ok && password != "" {
					params["obfs"] = "salamander"
					params["obfs-password"] = password
					break
				}
			}
		}
	}

	settings := s.linkSettings(inbound)
	version, _ := settings["version"].(float64)
	protocol := "hysteria2"
	if int(version) == 1 {
		protocol = "hysteria"
	}
	if hopPorts := hysteriaHopPorts(stream); hopPorts != "" {
		params["mport"] = hopPorts
	}

	link := fmt.Sprintf("%s://%s@%s", protocol, encodeUserinfo(client.Auth), joinHostPort(s.resolveInboundAddress(inbound), inbound.Port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email))
}

func hysteriaHopPorts(stream map[string]any) string {
	finalmask, _ := stream["finalmask"].(map[string]any)
	quicParams, _ := finalmask["quicParams"].(map[string]any)
	udpHop, _ := quicParams["udpHop"].(map[string]any)
	ports, _ := udpHop["ports"].(string)
	return strings.TrimSpace(ports)
}
