package controller

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	"github.com/Uber-Eins/phantom/v3/internal/web/middleware"
	"github.com/Uber-Eins/phantom/v3/internal/web/service/panel"
	"github.com/Uber-Eins/phantom/v3/internal/web/session"
)

// newAPIAuthTestEngine builds a gin engine that mirrors the production auth
// wiring: the sessions middleware, then checkAPIAuth guarding a sentinel
// handler that reports whether c.Next() was reached and whether api_authed was
// set. The APIController is the zero value, exactly as NewAPIController leaves
// its service fields (they query the global DB), so this exercises the real
// auth path. A fresh temp DB is initialised per test.
func newAPIAuthTestEngine(t *testing.T) (*gin.Engine, *APIController) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	engine := gin.New()
	store := cookie.NewStore([]byte("api-auth-test-secret"))
	engine.Use(sessions.Sessions("3x-ui", store))

	a := &APIController{}

	// Logs in as the first user so the session path can be exercised over a
	// cookie round-trip without reaching into checkAPIAuth's internals.
	engine.GET("/test-login", func(c *gin.Context) {
		u, err := (&panel.UserService{}).GetFirstUser()
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if err := session.SetLoginUser(c, u); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})
	engine.GET("/csrf-token", func(c *gin.Context) {
		if !session.IsLogin(c) {
			c.Status(http.StatusUnauthorized)
			return
		}
		token, err := session.EnsureCSRFToken(c)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": token})
	})

	api := engine.Group("/panel/api")
	api.Use(a.checkAPIAuth)
	api.Use(middleware.CSRFMiddleware())
	api.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	api.POST("/mutate", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return engine, a
}

func TestCheckAPIAuthRejectsBearer(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)

	req := httptest.NewRequest(http.MethodGet, "/panel/api/ping", nil)
	req.Header.Set("Authorization", "Bearer no-longer-supported")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
}

func TestCheckAPIAuthRejectsClientCertificate(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)

	req := httptest.NewRequest(http.MethodGet, "/panel/api/ping", nil)
	req.TLS = &tls.ConnectionState{
		VerifiedChains: [][]*x509.Certificate{{&x509.Certificate{}}},
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
}

// TestCheckAPIAuth_RejectsUnauthenticated characterizes the reject paths: no
// bearer token and no session yields 401 for XHR callers and 404 otherwise.
func TestCheckAPIAuth_RejectsUnauthenticated(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)

	cases := []struct {
		name string
		xhr  bool
		want int
	}{
		{"xhr gets 401", true, http.StatusUnauthorized},
		{"non-xhr gets 404", false, http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/panel/api/ping", nil)
			if c.xhr {
				req.Header.Set("X-Requested-With", "XMLHttpRequest")
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}

// TestCheckAPIAuth_SessionLoginPasses characterizes the session path: a
// logged-in browser session (no bearer token) reaches the handler.
func TestCheckAPIAuth_SessionLoginPasses(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)

	db := database.GetDB()
	var n int64
	if err := db.Model(&model.User{}).Count(&n).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if n == 0 {
		if err := db.Create(&model.User{Username: "sess", Password: "x"}).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}

	ts := httptest.NewServer(engine)
	defer ts.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{Jar: jar}

	loginResp, err := client.Get(ts.URL + "/test-login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}

	pingResp, err := client.Get(ts.URL + "/panel/api/ping")
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	pingResp.Body.Close()
	if pingResp.StatusCode != http.StatusOK {
		t.Fatalf("session ping status = %d, want 200", pingResp.StatusCode)
	}
}

func TestCheckAPIAuthSessionMutationRequiresCSRF(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{Jar: jar}
	if resp, err := client.Get(ts.URL + "/test-login"); err != nil {
		t.Fatalf("login: %v", err)
	} else {
		resp.Body.Close()
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/panel/api/mutate", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("mutation without token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mutation without token status = %d, want 403", resp.StatusCode)
	}

	tokenResp, err := client.Get(ts.URL + "/csrf-token")
	if err != nil {
		t.Fatalf("get csrf token: %v", err)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&body); err != nil {
		tokenResp.Body.Close()
		t.Fatalf("decode csrf token: %v", err)
	}
	tokenResp.Body.Close()
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/panel/api/mutate", nil)
	req.Header.Set(session.CSRFHeaderName, body.Token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("mutation with token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("mutation with token status = %d, want 204", resp.StatusCode)
	}
}

func TestRemovedAPIsAndPublicSubscriptionReturn404(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)
	NewAPIController(engine.Group(""))

	ts := httptest.NewServer(engine)
	defer ts.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginResp, err := client.Get(ts.URL + "/test-login")
	if err != nil {
		t.Fatal(err)
	}
	loginResp.Body.Close()

	for _, target := range []string{
		"/sub",
		"/sub/client-id",
		"/openapi.json",
		"/panel/openapi.json",
		"/panel/api-docs",
		"/panel/api/nodes",
		"/panel/api/groups",
		"/panel/api/hosts",
		"/panel/api/apiTokens",
		"/panel/api/fail2ban",
		"/panel/api/inbounds/allLinks",
		"/panel/api/server/getMigration",
	} {
		t.Run(target, func(t *testing.T) {
			resp, err := client.Get(ts.URL + target)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", resp.StatusCode)
			}
		})
	}
}
