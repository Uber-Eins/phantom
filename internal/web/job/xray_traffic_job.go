package job

import (
	"github.com/Uber-Eins/phantom/v3/internal/logger"
	"github.com/Uber-Eins/phantom/v3/internal/web/service"
	"github.com/Uber-Eins/phantom/v3/internal/web/service/outbound"
	"github.com/Uber-Eins/phantom/v3/internal/web/websocket"
	"github.com/Uber-Eins/phantom/v3/internal/xray"
)

// XrayTrafficJob collects local Xray traffic and updates the panel database.
type XrayTrafficJob struct {
	settingService  service.SettingService
	xrayService     service.XrayService
	inboundService  service.InboundService
	outboundService outbound.OutboundService
}

// clientStatsSnapshotMaxClients caps how many client_traffics rows the job
// ships as a full websocket snapshot per poll (same spirit as the
// controller's broadcastInboundsUpdateClientLimit). Above it, a snapshot
// would blow past the hub's payload cap and be dropped wholesale, so the job
// broadcasts only this poll's active rows and the UI leans on its 5s REST
// refetch for the rest.
const clientStatsSnapshotMaxClients = 5000

// NewXrayTrafficJob creates a new traffic collection job instance.
func NewXrayTrafficJob() *XrayTrafficJob {
	return new(XrayTrafficJob)
}

// Run collects traffic statistics from Xray, updates the database, and pushes
// real-time updates over WebSocket using compact delta payloads — no REST
// fallback, scales to 10k–20k+ clients per inbound.
func (j *XrayTrafficJob) Run() {
	if !j.xrayService.IsXrayRunning() {
		return
	}
	traffics, clientTraffics, err := j.xrayService.GetXrayTraffic()
	if err != nil {
		return
	}
	needRestart0, clientsDisabled, err := j.inboundService.AddTraffic(traffics, clientTraffics)
	if err != nil {
		logger.Warning("add inbound traffic failed:", err)
	}
	err, needRestart1 := j.outboundService.AddTraffic(traffics, clientTraffics)
	if err != nil {
		logger.Warning("add outbound traffic failed:", err)
	}
	if clientsDisabled {
		restartOnDisable, settingErr := j.settingService.GetRestartXrayOnClientDisable()
		if settingErr != nil {
			logger.Warning("get RestartXrayOnClientDisable failed:", settingErr)
		}
		if restartOnDisable {
			if err := j.xrayService.RestartXray(false); err != nil {
				logger.Warning("reconcile xray after disabling clients failed:", err)
				j.xrayService.SetToNeedRestart()
			}
		}
		websocket.BroadcastInvalidate(websocket.MessageTypeInbounds)
	}
	if needRestart0 || needRestart1 {
		j.xrayService.SetToNeedRestart()
	}

	// Derive the online set from this poll's per-email deltas rather than the
	// last_online column, which intentionally outlives a single polling cycle.
	activeEmails := make([]string, 0, len(clientTraffics))
	deltaActive := make(map[string]bool, len(clientTraffics))
	for _, ct := range clientTraffics {
		if ct != nil && ct.Up+ct.Down > 0 {
			activeEmails = append(activeEmails, ct.Email)
			deltaActive[ct.Email] = true
		}
	}
	// When the core supports the online-stats API, union in connection-based
	// onlines. Neither signal alone covers everything: an idle-but-connected
	// client moves no bytes between polls (the delta heuristic's blind spot),
	// while a short-lived connection can close before this poll yet still show
	// in the delta. Older cores fall back to deltas alone.
	if onlineUsers, apiMode, ouErr := j.xrayService.GetOnlineUsers(); ouErr != nil {
		logger.Debug("get online users from xray api failed:", ouErr)
	} else if apiMode {
		idleOnline := make([]string, 0, len(onlineUsers))
		for _, u := range onlineUsers {
			if !deltaActive[u.Email] {
				activeEmails = append(activeEmails, u.Email)
				idleOnline = append(idleOnline, u.Email)
			}
		}
		// The traffic path only bumps last_online on a non-zero delta; keep the
		// column fresh for clients kept online purely by a live connection.
		if err := j.inboundService.BumpClientsLastOnline(idleOnline); err != nil {
			logger.Warning("bump last online for connected clients failed:", err)
		}
	}
	// Pair the email signal with the inbound tags that moved bytes this poll.
	// Xray's user>>>email counter aggregates across every inbound a client is
	// attached to, so an online email alone can't say which inbound it used —
	// gating the per-inbound view on these tags keeps a multi-inbound client
	// off inbounds that saw no traffic. See issue #4859.
	activeInboundTags := make([]string, 0, len(traffics))
	for _, tr := range traffics {
		if tr != nil && tr.IsInbound && tr.Up+tr.Down > 0 {
			activeInboundTags = append(activeInboundTags, tr.Tag)
		}
	}
	j.inboundService.RefreshLocalOnlineClients(activeEmails, activeInboundTags)

	if !websocket.HasClients() {
		return
	}

	// Small installs broadcast the full snapshot (see GetAllClientTraffics for
	// why deltas alone left UI rows stale). Above the threshold the snapshot
	// would be dropped by the hub's payload cap anyway, so ship this poll's
	// active rows instead and scope last-online to them; the initial full map
	// still arrives over REST.
	snapshot := true
	if total, countErr := j.inboundService.CountClientTraffics(); countErr != nil {
		logger.Warning("count client traffics for websocket failed:", countErr)
	} else if total > clientStatsSnapshotMaxClients {
		snapshot = false
	}

	var stats []*xray.ClientTraffic
	var statsErr error
	if snapshot {
		stats, statsErr = j.inboundService.GetAllClientTraffics()
	} else {
		stats, statsErr = j.inboundService.GetActiveClientTraffics(activeEmails)
	}
	if statsErr != nil {
		logger.Warning("get client traffics for websocket failed:", statsErr)
	}

	var lastOnlineMap map[string]int64
	if snapshot {
		if lastOnlineMap, err = j.inboundService.GetClientsLastOnline(); err != nil {
			logger.Warning("get clients last online failed:", err)
		}
	} else {
		lastOnlineMap = make(map[string]int64, len(stats))
		for _, ct := range stats {
			if ct != nil {
				lastOnlineMap[ct.Email] = ct.LastOnline
			}
		}
	}
	if lastOnlineMap == nil {
		lastOnlineMap = make(map[string]int64)
	}
	onlineClients := j.inboundService.GetOnlineClients()
	if onlineClients == nil {
		onlineClients = []string{}
	}
	websocket.BroadcastTraffic(map[string]any{
		"traffics":       traffics,
		"clientTraffics": clientTraffics,
		"onlineClients":  onlineClients,
		"lastOnlineMap":  lastOnlineMap,
	})

	clientStatsPayload := map[string]any{"snapshot": snapshot}
	if len(stats) > 0 {
		clientStatsPayload["clients"] = stats
	}
	if inboundSummary, err := j.inboundService.GetInboundsTrafficSummary(); err != nil {
		logger.Warning("get inbounds traffic summary for websocket failed:", err)
	} else if len(inboundSummary) > 0 {
		clientStatsPayload["inbounds"] = inboundSummary
	}
	if len(clientStatsPayload) > 1 {
		websocket.BroadcastClientStats(clientStatsPayload)
	}

	if updatedOutbounds, err := j.outboundService.GetOutboundsTraffic(); err == nil && updatedOutbounds != nil {
		websocket.BroadcastOutbounds(updatedOutbounds)
	} else if err != nil {
		logger.Warning("get all outbounds for websocket failed:", err)
	}
}
