package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWebSecurityOriginAndCSRF(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	body := map[string]string{"username": "admin", "password": "secret123"}
	for _, headers := range [][]string{
		{"Origin", "https://attacker.example"},
		{"Origin", "null"},
		{"Sec-Fetch-Site", "cross-site"},
		{"Origin", "http://sibling.example", "Sec-Fetch-Site", "same-site"},
	} {
		if code := e.do("POST", "/api/v1/session", body, nil, headers...); code != 403 {
			t.Errorf("login %v: %d", headers, code)
		}
		if code := e.do("PUT", "/api/v1/preferences", map[string]string{"lang": "ja"}, nil, headers...); code != 403 {
			t.Errorf("cookie mutation %v: %d", headers, code)
		}
	}
	for _, headers := range [][]string{nil, {"Origin", e.ts.URL}, {"Origin", e.ts.URL, "Sec-Fetch-Site", "same-origin"}} {
		if code := e.do("PUT", "/api/v1/preferences", map[string]string{"lang": "en"}, nil, headers...); code != 200 {
			t.Errorf("same-origin/no-origin HTTP write %v: %d", headers, code)
		}
	}
	if code := e.do("PUT", "/api/v1/preferences", map[string]string{"lang": "ja"}, nil, "Origin", e.ts.URL, "X-Unagikuma-CSRF", "wrong"); code != 403 {
		t.Errorf("same-origin must still require CSRF: %d", code)
	}
	var prefs map[string]string
	if code := e.do("GET", "/api/v1/preferences", nil, &prefs); code != 200 || prefs["lang"] != "en" {
		t.Fatalf("rejected write altered preferences: %d %v", code, prefs)
	}
}

func TestWebSecurityBearerWritesRemainUsable(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	var token struct{ Token string }
	if code := e.do("POST", "/api/v1/tokens", map[string]any{"name": "security-test", "scopes": []string{"write"}}, &token); code != 200 {
		t.Fatalf("create token: %d", code)
	}
	r := httptest.NewRequest("POST", "/api/v1/incidents/bulk", strings.NewReader(`{"ids":[],"action":"ack"}`))
	r.Header.Set("Authorization", "Bearer "+token.Token)
	r.Header.Set("Origin", "https://api-client.example")
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("bearer write without cookie or CSRF: %d %s", w.Code, w.Body)
	}
}

func TestWebSecurityAnonymousSensitiveAPIs(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	for _, path := range []string{
		"/api/v1/session", "/api/v1/config", "/api/v1/config/history", "/api/v1/config/diff",
		"/api/v1/settings", "/api/v1/tunables", "/api/v1/people", "/api/v1/oncall", "/api/v1/oncall/calendar",
		"/api/v1/channels", "/api/v1/policies", "/api/v1/monitors", "/api/v1/monitors/backup/push",
		"/api/v1/users", "/api/v1/tokens", "/api/v1/secrets", "/api/v1/backup", "/api/v1/audit/export",
		"/api/v1/nodes", "/api/v1/cluster/agent", "/api/v1/notifications", "/api/v1/stream", "/metrics",
		"/api/v1/%63onfig",
	} {
		t.Run(path, func(t *testing.T) {
			for _, method := range []string{"GET", "HEAD"} {
				w := httptest.NewRecorder()
				e.srv.Handler().ServeHTTP(w, httptest.NewRequest(method, path, nil))
				if w.Code != 401 {
					t.Errorf("anonymous %s %s: status=%d body=%s", method, path, w.Code, w.Body)
				}
			}
		})
	}
}

func TestWebSecurityPublicCachingAndHeaders(t *testing.T) {
	e := newEnv(t, testBind, fstest.MapFS{
		"index.html":    {Data: []byte("console shell")},
		"vendor/lib.js": {Data: []byte("export const value=1;")},
	})
	for _, tc := range []struct{ path, cache string }{
		{"/", "no-cache"}, {"/vendor/lib.js", "public, max-age=86400"},
		{"/api/v1/status/main", "public, max-age=30"},
	} {
		w := httptest.NewRecorder()
		e.srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "http://localhost"+tc.path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("%s: %d headers=%v", tc.path, w.Code, w.Header())
		}
		if w.Header().Get("Strict-Transport-Security") != "" {
			t.Error("plain HTTP must remain usable without HSTS")
		}
		for _, directive := range []string{"script-src 'self'", "object-src 'none'", "frame-ancestors 'none'", "base-uri 'none'", "form-action 'self'"} {
			if !strings.Contains(w.Header().Get("Content-Security-Policy"), directive) {
				t.Errorf("missing CSP directive %s", directive)
			}
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("missing security headers: %v", w.Header())
		}
		if strings.Contains(w.Body.String(), "192.168.1.") {
			t.Errorf("private target leaked on %s", tc.path)
		}
	}
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "https://localhost/", nil))
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("TLS response missing HSTS")
	}
	for _, path := range []string{"/vendor/%2e%2e/%2e%2e/monitors.bind", "/vendor/%2e%2e%2fmonitors.bind", "/vendor/..%5cmonitors.bind", "/%00", "/api/v1/unknown", "/api/v1/config%2fhistory"} {
		w := httptest.NewRecorder()
		e.srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code >= 200 && w.Code < 300 {
			t.Errorf("unexpected successful path %s: %d %s", path, w.Code, w.Body)
		}
	}
}

func TestWebSecurityPrivateDownloadsNotCacheable(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	for _, path := range []string{"/api/v1/backup", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			resp, err := e.c.Get(e.ts.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("status=%d", resp.StatusCode)
			}
			if got := resp.Header.Get("Cache-Control"); got != "no-store" {
				t.Fatalf("sensitive response cache policy=%q; want no-store", got)
			}
		})
	}
}

// A cross-origin fetch can send JSON as text/plain without a preflight.
// Setup's loopback shortcut must not trust such a browser request.
func TestWebSecurityCrossOriginSetup(t *testing.T) {
	e := newEnv(t, testBind, nil)
	r := httptest.NewRequest("POST", "http://localhost/api/v1/setup", strings.NewReader(`{"username":"attacker","password":"attacker123"}`))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Content-Type", "text/plain")
	r.Header.Set("Origin", "https://attacker.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-origin setup: got %d, want 403; body=%s", w.Code, w.Body)
	}
	n, err := e.app.Store().UserCount()
	if err != nil || n != 0 {
		t.Fatalf("cross-origin request created an account: count=%d err=%v", n, err)
	}
}
