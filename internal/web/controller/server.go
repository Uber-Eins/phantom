package controller

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"

	"github.com/gin-gonic/gin"
)

var filenameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-.]+$`)

// ServerController handles server management and status-related operations.
type ServerController struct {
	BaseController

	serverService      service.ServerService
	xrayMetricsService service.XrayMetricsService
}

// NewServerController creates a new ServerController, initializes routes, and starts background tasks.
func NewServerController(g *gin.RouterGroup) *ServerController {
	a := &ServerController{}
	service.RestoreSystemMetrics()
	a.initRouter(g)
	a.startTask()
	return a
}

// initRouter sets up the routes for server status, Xray management, and utility endpoints.
func (a *ServerController) initRouter(g *gin.RouterGroup) {
	g.GET("/status", a.status)
	g.GET("/cpuHistory/:bucket", a.getCpuHistoryBucket)
	g.GET("/history/:metric/:bucket", a.getMetricHistoryBucket)
	g.GET("/xrayMetricsState", a.getXrayMetricsState)
	g.GET("/xrayMetricsHistory/:metric/:bucket", a.getXrayMetricsHistoryBucket)
	g.GET("/xrayObservatory", a.getXrayObservatory)
	g.GET("/xrayObservatoryHistory/:tag/:bucket", a.getXrayObservatoryHistoryBucket)
	g.GET("/getXrayVersion", a.getXrayVersion)
	g.GET("/getConfigJson", a.getConfigJson)
	g.GET("/getNginxConfig", a.getNginxConfig)
	g.GET("/getDb", a.getDb)
	g.GET("/getNewUUID", a.getNewUUID)
	g.GET("/getNewX25519Cert", a.getNewX25519Cert)
	g.GET("/getNewmldsa65", a.getNewmldsa65)
	g.GET("/getNewmlkem768", a.getNewmlkem768)
	g.GET("/getNewVlessEnc", a.getNewVlessEnc)

	g.POST("/stopXrayService", a.stopXrayService)
	g.POST("/restartXrayService", a.restartXrayService)
	g.POST("/installXray/:version", a.installXray)
	g.POST("/logs/:count", a.getLogs)
	g.POST("/xraylogs/:count", a.getXrayLogs)
	g.POST("/importDB", a.importDB)
	g.POST("/getNewEchCert", a.getNewEchCert)
	g.POST("/getCertHash", a.getCertHash)
	g.POST("/getRemoteCertHash", a.getRemoteCertHash)
	g.POST("/scanRealityTarget", a.scanRealityTarget)
	g.POST("/scanRealityTargets", a.scanRealityTargets)
}

// startTask registers the @2s ticker that refreshes server status, samples
// xray metrics, and pushes the new snapshot to all websocket subscribers.
// State + sampling live in ServerService; the controller only orchestrates
// the cross-service side effects (xrayMetrics sample + websocket broadcast).
func (a *ServerController) startTask() {
	server := global.GetWebServer()
	if server == nil {
		return
	}
	c := server.GetCron()
	if c == nil {
		return
	}
	_, _ = c.AddFunc("@every 2s", func() {
		status := a.serverService.RefreshStatus()
		if status == nil {
			return
		}
		a.xrayMetricsService.Sample(time.Now())
		websocket.BroadcastStatus(status)
	})
	_, _ = c.AddFunc("@every 1m", func() {
		if err := service.PersistSystemMetrics(); err != nil {
			logger.Warning("persist system metrics failed:", err)
		}
	})
}

// status returns the current server status information.
func (a *ServerController) status(c *gin.Context) { jsonObj(c, a.serverService.LastStatus(), nil) }

func parseHistoryBucket(c *gin.Context) (int, bool) {
	bucket, err := strconv.Atoi(c.Param("bucket"))
	if err != nil || bucket <= 0 || !service.IsAllowedHistoryBucket(bucket) {
		jsonMsg(c, "invalid bucket", fmt.Errorf("unsupported bucket"))
		return 0, false
	}
	return bucket, true
}

// getCpuHistoryBucket retrieves aggregated CPU usage history based on the specified time bucket.
// Kept for back-compat; new callers should use /history/cpu/:bucket which
// returns {"t","v"} (uniform across all metrics) instead of {"t","cpu"}.
func (a *ServerController) getCpuHistoryBucket(c *gin.Context) {
	bucket, ok := parseHistoryBucket(c)
	if !ok {
		return
	}
	jsonObj(c, a.serverService.AggregateCpuHistory(bucket, 60), nil)
}

// getMetricHistoryBucket returns up to 60 buckets of history for a single
// system metric (cpu, mem, netUp, netDown, online, load1/5/15). The
// SystemHistoryModal calls one endpoint per active tab.
func (a *ServerController) getMetricHistoryBucket(c *gin.Context) {
	metric := c.Param("metric")
	if !slices.Contains(service.SystemMetricKeys, metric) {
		jsonMsg(c, "invalid metric", fmt.Errorf("unknown metric"))
		return
	}
	bucket, ok := parseHistoryBucket(c)
	if !ok {
		return
	}
	jsonObj(c, a.serverService.AggregateSystemMetric(metric, bucket, 60), nil)
}

func (a *ServerController) getXrayMetricsState(c *gin.Context) {
	jsonObj(c, a.xrayMetricsService.State(), nil)
}

func (a *ServerController) getXrayMetricsHistoryBucket(c *gin.Context) {
	metric := c.Param("metric")
	if !slices.Contains(service.XrayMetricKeys, metric) {
		jsonMsg(c, "invalid metric", fmt.Errorf("unknown metric"))
		return
	}
	bucket, ok := parseHistoryBucket(c)
	if !ok {
		return
	}
	jsonObj(c, a.xrayMetricsService.AggregateMetric(metric, bucket, 60), nil)
}

func (a *ServerController) getXrayObservatory(c *gin.Context) {
	jsonObj(c, a.xrayMetricsService.ObservatorySnapshot(), nil)
}

func (a *ServerController) getXrayObservatoryHistoryBucket(c *gin.Context) {
	tag := c.Param("tag")
	if !a.xrayMetricsService.HasObservatoryTag(tag) {
		jsonMsg(c, "invalid tag", fmt.Errorf("unknown observatory tag"))
		return
	}
	bucket, ok := parseHistoryBucket(c)
	if !ok {
		return
	}
	jsonObj(c, a.xrayMetricsService.AggregateObservatory(tag, bucket, 60), nil)
}

func (a *ServerController) getXrayVersion(c *gin.Context) {
	versions, err := a.serverService.GetXrayVersionsCached()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "getVersion"), err)
		return
	}
	jsonObj(c, versions, nil)
}

// stopXrayService stops the Xray service.
func (a *ServerController) stopXrayService(c *gin.Context) {
	err := a.serverService.StopXrayService()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.xray.stopError"), err)
		websocket.BroadcastXrayState("error", err.Error())
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.xray.stopSuccess"), err)
	websocket.BroadcastXrayState("stop", "")
	websocket.BroadcastNotification(
		I18nWeb(c, "pages.xray.stopSuccess"),
		"Xray service has been stopped",
		"warning",
	)
}

// restartXrayService restarts the Xray service.
func (a *ServerController) restartXrayService(c *gin.Context) {
	err := a.serverService.RestartXrayService()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.xray.restartError"), err)
		websocket.BroadcastXrayState("error", err.Error())
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.xray.restartSuccess"), err)
	websocket.BroadcastXrayState("running", "")
	websocket.BroadcastNotification(
		I18nWeb(c, "pages.xray.restartSuccess"),
		"Xray service has been restarted successfully",
		"success",
	)
}

// installXray installs or updates Xray to the specified version.
func (a *ServerController) installXray(c *gin.Context) {
	err := a.serverService.UpdateXray(c.Param("version"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.index.xraySwitch"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.index.xraySwitchVersionPopover"), err)
}

// getLogs retrieves the in-process application logs based on count and level.
func (a *ServerController) getLogs(c *gin.Context) {
	logs := a.serverService.GetLogs(
		c.Param("count"),
		c.PostForm("level"),
		c.PostForm("source"),
	)
	jsonObj(c, logs, nil)
}

// getXrayLogs retrieves Xray logs with filtering options for direct, blocked, and proxy traffic.
func (a *ServerController) getXrayLogs(c *gin.Context) {
	freedoms, blackholes := a.serverService.GetDefaultLogOutboundTags()
	logs := a.serverService.GetXrayLogs(
		c.Param("count"),
		c.PostForm("filter"),
		c.PostForm("showDirect"),
		c.PostForm("showBlocked"),
		c.PostForm("showProxy"),
		freedoms,
		blackholes,
	)
	jsonObj(c, logs, nil)
}

// getConfigJson retrieves the Xray configuration as JSON.
func (a *ServerController) getConfigJson(c *gin.Context) {
	configJson, err := a.serverService.GetConfigJson()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.index.getConfigError"), err)
		return
	}
	jsonObj(c, configJson, nil)
}

// getNginxConfig retrieves the generated Nginx configuration as plain text.
func (a *ServerController) getNginxConfig(c *gin.Context) {
	configText, err := a.serverService.GetNginxConfig()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.index.getConfigError"), err)
		return
	}
	jsonObj(c, configText, nil)
}

// getDb downloads the database file.
func (a *ServerController) getDb(c *gin.Context) {
	db, err := a.serverService.GetDb()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.index.getDatabaseError"), err)
		return
	}

	filename := a.serverService.BackupFilename(c.Request.Host)
	if !filenameRegex.MatchString(filename) {
		_ = c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid filename"))
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	_, _ = c.Writer.Write(db)
}

// importDB imports a database file and restarts the Xray service.
func (a *ServerController) importDB(c *gin.Context) {
	file, _, err := c.Request.FormFile("db")
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.index.readDatabaseError"), err)
		return
	}
	defer file.Close()
	if err := a.serverService.ImportDB(file); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.index.importDatabaseError"), err)
		return
	}
	jsonObj(c, I18nWeb(c, "pages.index.importDatabaseSuccess"), nil)
}

// getNewX25519Cert generates a new X25519 certificate.
func (a *ServerController) getNewX25519Cert(c *gin.Context) {
	cert, err := a.serverService.GetNewX25519Cert()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.getNewX25519CertError"), err)
		return
	}
	jsonObj(c, cert, nil)
}

// getNewmldsa65 generates a new ML-DSA-65 key.
func (a *ServerController) getNewmldsa65(c *gin.Context) {
	cert, err := a.serverService.GetNewmldsa65()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.getNewmldsa65Error"), err)
		return
	}
	jsonObj(c, cert, nil)
}

// getNewEchCert generates a new ECH certificate for the given SNI.
func (a *ServerController) getNewEchCert(c *gin.Context) {
	cert, err := a.serverService.GetNewEchCert(c.PostForm("sni"))
	if err != nil {
		jsonMsg(c, "get ech certificate", err)
		return
	}
	jsonObj(c, cert, nil)
}

// getCertHash returns the hex SHA-256 of the given certificate (file path or
// inline content) so the panel can fill the pinned-cert field.
func (a *ServerController) getCertHash(c *gin.Context) {
	hashes, err := a.serverService.GetCertHash(c.PostForm("certFile"), c.PostForm("certContent"))
	if err != nil {
		jsonMsg(c, "get cert hash", err)
		return
	}
	jsonObj(c, hashes, nil)
}

// getRemoteCertHash runs `xray tls ping` against the given server and returns
// its live certificate SHA-256 hash(es) for pinning.
func (a *ServerController) getRemoteCertHash(c *gin.Context) {
	hashes, err := a.serverService.GetRemoteCertHash(c.PostForm("server"))
	if err != nil {
		jsonMsg(c, "get remote cert hash", err)
		return
	}
	jsonObj(c, hashes, nil)
}

// scanRealityTarget runs a live TLS 1.3 probe against the candidate REALITY
// target and returns a structured feasibility verdict plus the cert SAN names.
func (a *ServerController) scanRealityTarget(c *gin.Context) {
	xver, _ := strconv.Atoi(c.PostForm("xver"))
	res, err := a.serverService.ScanRealityTarget(c.PostForm("target"), xver)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.scanRealityTargetError"), err)
		return
	}
	jsonObj(c, res, nil)
}

// scanRealityTargets probes a batch of candidate REALITY targets (the supplied
// comma-separated list, or the built-in seed set when empty) and returns each
// verdict ranked by feasibility then latency.
func (a *ServerController) scanRealityTargets(c *gin.Context) {
	res, err := a.serverService.ScanRealityTargets(c.PostForm("targets"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.scanRealityTargetError"), err)
		return
	}
	jsonObj(c, res, nil)
}

// getNewVlessEnc generates a new VLESS encryption key.
func (a *ServerController) getNewVlessEnc(c *gin.Context) {
	out, err := a.serverService.GetNewVlessEnc()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.getNewVlessEncError"), err)
		return
	}
	jsonObj(c, out, nil)
}

// getNewUUID generates a new UUID.
func (a *ServerController) getNewUUID(c *gin.Context) {
	uuidResp, err := a.serverService.GetNewUUID()
	if err != nil {
		jsonMsg(c, "Failed to generate UUID", err)
		return
	}
	jsonObj(c, uuidResp, nil)
}

// getNewmlkem768 generates a new ML-KEM-768 key.
func (a *ServerController) getNewmlkem768(c *gin.Context) {
	out, err := a.serverService.GetNewmlkem768()
	if err != nil {
		jsonMsg(c, "Failed to generate mlkem768 keys", err)
		return
	}
	jsonObj(c, out, nil)
}
