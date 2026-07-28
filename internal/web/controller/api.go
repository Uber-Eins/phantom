package controller

import (
	"net/http"

	"github.com/Uber-Eins/phantom/v3/internal/web/middleware"
	"github.com/Uber-Eins/phantom/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// APIController handles the main API routes for the phantom panel, including inbounds and server management.
type APIController struct {
	BaseController
	inboundController     *InboundController
	serverController      *ServerController
	settingController     *SettingController
	xraySettingController *XraySettingController
	certificateController *CertificateController
}

// NewAPIController creates a new APIController instance and initializes its routes.
func NewAPIController(g *gin.RouterGroup) *APIController {
	a := &APIController{}
	a.initRouter(g)
	return a
}

func (a *APIController) checkAPIAuth(c *gin.Context) {
	if !session.IsLogin(c) {
		if c.GetHeader("X-Requested-With") == "XMLHttpRequest" {
			c.AbortWithStatus(http.StatusUnauthorized)
		} else {
			c.AbortWithStatus(http.StatusNotFound)
		}
		return
	}
	c.Next()
}

// initRouter sets up the API routes for inbounds, server, and other endpoints.
func (a *APIController) initRouter(g *gin.RouterGroup) {
	// Main API group
	api := g.Group("/panel/api")
	api.Use(a.checkAPIAuth)
	api.Use(middleware.CSRFMiddleware())

	// Inbounds API
	inbounds := api.Group("/inbounds")
	a.inboundController = NewInboundController(inbounds)

	clients := api.Group("/clients")
	NewClientController(clients)

	// Server API
	server := api.Group("/server")
	a.serverController = NewServerController(server)

	// Settings + Xray config management live under the session-only API.
	a.settingController = NewSettingController(api)
	a.xraySettingController = NewXraySettingController(api)
	a.certificateController = NewCertificateController(api)
}
