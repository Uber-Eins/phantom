package share

import (
	"net"
	"strings"

	"github.com/goccy/go-json"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	"github.com/Uber-Eins/phantom/v3/internal/web/service"
)

// LinkService generates authenticated connection links for local inbounds.
type LinkService struct {
	address        string
	inboundService service.InboundService

	// All caches are reset for every request. A fully primed client cache makes
	// a miss authoritative and avoids repeatedly decoding a large settings JSON.
	clientsByInbound    map[int]map[string]model.Client
	fullyPrimedInbounds map[int]bool
	settingsByInbound   map[int]map[string]any
}

func NewLinkService() *LinkService {
	return &LinkService{}
}

func (s *LinkService) PrepareForRequest(host string) {
	if !isRoutableHost(host) {
		host = "localhost"
	}
	s.address = host
	s.clientsByInbound = map[int]map[string]model.Client{}
	s.fullyPrimedInbounds = map[int]bool{}
	s.settingsByInbound = map[int]map[string]any{}
}

func (s *LinkService) primeLinkClients(inboundID int, clients []model.Client, complete bool) {
	if inboundID <= 0 {
		return
	}
	if s.clientsByInbound == nil {
		s.clientsByInbound = map[int]map[string]model.Client{}
	}
	cache := s.clientsByInbound[inboundID]
	if cache == nil {
		cache = make(map[string]model.Client, len(clients))
		s.clientsByInbound[inboundID] = cache
	}
	for _, client := range clients {
		if _, exists := cache[client.Email]; !exists {
			cache[client.Email] = client
		}
	}
	if complete {
		if s.fullyPrimedInbounds == nil {
			s.fullyPrimedInbounds = map[int]bool{}
		}
		s.fullyPrimedInbounds[inboundID] = true
	}
}

func (s *LinkService) clientForLink(inbound *model.Inbound, email string) (model.Client, bool) {
	if cache, ok := s.clientsByInbound[inbound.Id]; ok {
		if client, found := cache[email]; found {
			return client, true
		}
		if s.fullyPrimedInbounds[inbound.Id] {
			return model.Client{}, false
		}
	}
	clients, err := s.inboundService.GetClients(inbound)
	if err != nil {
		return model.Client{}, false
	}
	s.primeLinkClients(inbound.Id, clients, true)
	for _, client := range clients {
		if client.Email == email {
			return client, true
		}
	}
	return model.Client{}, false
}

// linkSettings decodes only inbound-level settings and skips the clients array.
func (s *LinkService) linkSettings(inbound *model.Inbound) map[string]any {
	if inbound.Id > 0 {
		if cached, ok := s.settingsByInbound[inbound.Id]; ok {
			return cached
		}
	}

	shallow := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(inbound.Settings), &shallow)
	settings := make(map[string]any, len(shallow))
	for key, raw := range shallow {
		if key == "clients" {
			continue
		}
		var value any
		_ = json.Unmarshal(raw, &value)
		settings[key] = value
	}
	if inbound.Id > 0 {
		if s.settingsByInbound == nil {
			s.settingsByInbound = map[int]map[string]any{}
		}
		s.settingsByInbound[inbound.Id] = settings
	}
	return settings
}

func isRoutableHost(host string) bool {
	if host == "" {
		return false
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return !ip.IsLoopback() && !ip.IsUnspecified()
	}
	return true
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func listenIsInternalOnly(listen string) bool {
	if listen == "" {
		return false
	}
	if listen[0] == '@' || listen[0] == '/' {
		return true
	}
	return isLoopbackHost(listen)
}

// projectThroughFallbackMaster makes an internal-only fallback child advertise
// the externally reachable master while preserving the child's transport.
func (s *LinkService) projectThroughFallbackMaster(inbound *model.Inbound) bool {
	if inbound == nil || !listenIsInternalOnly(inbound.Listen) {
		return false
	}
	db := database.GetDB()
	var master *model.Inbound

	var rule model.InboundFallback
	if err := db.Where("child_id = ?", inbound.Id).
		Order("sort_order ASC, id ASC").
		First(&rule).Error; err == nil {
		var candidate model.Inbound
		if err := db.Where("id = ?", rule.MasterId).First(&candidate).Error; err == nil {
			master = &candidate
		}
	}

	if master == nil && strings.HasPrefix(inbound.Listen, "@") {
		var candidate model.Inbound
		if err := db.Model(model.Inbound{}).
			Where("JSON_TYPE(settings, '$.fallbacks') = 'array'").
			Where("EXISTS (SELECT * FROM json_each(settings, '$.fallbacks') WHERE json_extract(value, '$.dest') = ?)", inbound.Listen).
			First(&candidate).Error; err == nil {
			master = &candidate
		}
	}

	if master == nil {
		return false
	}
	inbound.StreamSettings = mergeStreamFromMaster(inbound.StreamSettings, master.StreamSettings)
	inbound.Listen = master.Listen
	inbound.Port = master.Port
	return true
}

func mergeStreamFromMaster(childStream, masterStream string) string {
	var stream map[string]any
	_ = json.Unmarshal([]byte(childStream), &stream)
	if stream == nil {
		stream = map[string]any{}
	}
	var master map[string]any
	_ = json.Unmarshal([]byte(masterStream), &master)
	if master == nil {
		return childStream
	}

	stream["security"] = master["security"]
	copyOrDelete := func(key string) {
		if value, ok := master[key]; ok {
			stream[key] = value
		} else {
			delete(stream, key)
		}
	}
	copyOrDelete("tlsSettings")
	copyOrDelete("realitySettings")
	delete(stream, "externalProxy")

	out, err := json.MarshalIndent(stream, "", "  ")
	if err != nil {
		return childStream
	}
	return string(out)
}

// GetLink builds one connection URI for an inbound/client pair.
func (s *LinkService) GetLink(inbound *model.Inbound, email string) string {
	switch inbound.Protocol {
	case model.VMESS:
		return s.genVmessLink(inbound, email)
	case model.VLESS:
		return s.genVlessLink(inbound, email)
	case model.Trojan:
		return s.genTrojanLink(inbound, email)
	case model.Shadowsocks:
		return s.genShadowsocksLink(inbound, email)
	case model.Hysteria:
		return s.genHysteriaLink(inbound, email)
	case model.MTProto:
		return s.genMtprotoLink(inbound, email)
	case model.WireGuard:
		return s.genWireguardLink(inbound, email)
	default:
		return ""
	}
}
