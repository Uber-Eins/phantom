package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	"github.com/Uber-Eins/phantom/v3/internal/web/locale"
	"github.com/Uber-Eins/phantom/v3/internal/web/session"
)

func newSPAFallbackTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	oldDistFS := distFS
	SetDistFS(fstest.MapFS{
		"dist/index.html": {Data: []byte(`<!doctype html><html><body>spa shell</body></html>`)},
	})
	t.Cleanup(func() { SetDistFS(oldDistFS) })

	const basePath = "/admin-random/"
	engine := gin.New()
	engine.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("spa-fallback-test-secret"))))
	engine.Use(func(c *gin.Context) {
		c.Set("base_path", basePath)
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		if c.GetHeader("X-Test-Login") == "1" {
			session.SetAPIAuthUser(c, &model.User{Id: 1, Username: "test"})
		}
		c.Next()
	})

	ctrl := NewXUIController(engine.Group(basePath))
	engine.NoRoute(func(c *gin.Context) {
		if ctrl.HandleNoRoutePanelSPA(c) {
			return
		}
		c.AbortWithStatus(http.StatusNotFound)
	})
	return engine
}

func TestPanelSPAOnlyServesKnownPages(t *testing.T) {
	engine := newSPAFallbackTestEngine(t)
	for _, target := range []string{
		"/admin-random/panel/",
		"/admin-random/panel/inbounds",
		"/admin-random/panel/clients",
		"/admin-random/panel/settings",
		"/admin-random/panel/xray",
		"/admin-random/panel/outbound",
		"/admin-random/panel/routing",
	} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Header.Set("Accept", "text/html")
			req.Header.Set("X-Test-Login", "1")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "spa shell") {
				t.Fatalf("body does not contain SPA shell: %s", w.Body.String())
			}
		})
	}
}

func TestRemovedAndUnknownPanelPagesReturn404(t *testing.T) {
	engine := newSPAFallbackTestEngine(t)
	for _, target := range []string{
		"/admin-random/panel/groups",
		"/admin-random/panel/nodes",
		"/admin-random/panel/hosts",
		"/admin-random/panel/api-docs",
		"/admin-random/panel/future",
		"/admin-random/panel/clients/alice",
	} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Header.Set("Accept", "text/html")
			req.Header.Set("X-Test-Login", "1")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "spa shell") {
				t.Fatalf("removed route was served by SPA fallback: %s", w.Body.String())
			}
		})
	}
}

func TestKnownPanelPagePreservesAuthSemantics(t *testing.T) {
	engine := newSPAFallbackTestEngine(t)

	t.Run("browser redirects", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin-random/panel/inbounds", nil)
		req.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("status = %d, want 307", w.Code)
		}
	})

	t.Run("xhr is unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin-random/panel/inbounds", nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}

func TestPanelSPAFallbackExcludesAPIAndAssets(t *testing.T) {
	engine := newSPAFallbackTestEngine(t)
	for _, target := range []string{
		"/admin-random/panel/api",
		"/admin-random/panel/api/unknown",
		"/admin-random/panel/missing.js",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("X-Test-Login", "1")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", target, w.Code)
		}
	}
}

func TestPanelCSRFTokenRemainsExplicit(t *testing.T) {
	engine := newSPAFallbackTestEngine(t)
	req := httptest.NewRequest(http.MethodGet, "/admin-random/panel/csrf-token", nil)
	req.Header.Set("X-Test-Login", "1")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}

func TestPanelSPAFallbackPredicate(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{path: "/admin-random/panel", want: true},
		{path: "/admin-random/panel/", want: true},
		{path: "/admin-random/panel/inbounds", want: true},
		{path: "/admin-random/panel/groups"},
		{path: "/admin-random/panel/nodes"},
		{path: "/admin-random/panel/hosts"},
		{path: "/admin-random/panel/api-docs"},
		{path: "/admin-random/panel/future"},
		{path: "/admin-random/panel/api/unknown"},
		{path: "/admin-random/panel/missing.css"},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("base_path", "/admin-random/")
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Accept", "text/html")
		c.Request = req
		if got := isPanelSPAFallbackRequest(c); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.path, got, tc.want)
		}
	}
}
