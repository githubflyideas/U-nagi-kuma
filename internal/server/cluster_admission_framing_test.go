package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
)

func TestClusterAdmissionFraming(t *testing.T) {
	tests := []struct {
		name, path string
		status     int
		change     func(*http.Request)
	}{
		{"no TLS", cluster.StatePath, 403, func(r *http.Request) { r.TLS = nil }},
		{"chunked state", cluster.StatePath, 400, func(r *http.Request) { r.TransferEncoding = []string{"chunked"}; r.ContentLength = -1 }},
		{"body connect", cluster.ConnectPath, 400, func(r *http.Request) { r.ContentLength = 1 }},
		{"wrong upgrade", cluster.ConnectPath, 426, func(r *http.Request) { r.Header.Set("Upgrade", "websocket") }},
		{"missing connection", cluster.ConnectPath, 426, func(r *http.Request) { r.Header.Del("Connection") }},
		{"ambiguous auth", cluster.StatePath, 401, func(r *http.Request) { r.Header.Add("Authorization", "Bearer bad") }},
		{"oversized token", cluster.StatePath, 401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 257)) }},
		{"missing token", cluster.StatePath, 401, func(r *http.Request) { r.Header.Del("Authorization") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, admissionBind, nil)
			token, err := e.app.Store().CreateAgentToken("edge1")
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "https://localhost"+tc.path, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Connection", "keep-alive, Upgrade")
			r.Header.Set("Upgrade", cluster.Upgrade)
			tc.change(r)
			w := httptest.NewRecorder()
			e.srv.Handler().ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
			tokens, err := e.app.Store().AgentTokens()
			if err != nil {
				t.Fatal(err)
			}
			if tokens[0].LastUsed != nil {
				t.Fatal("malformed request reached credential database write")
			}
			if e.srv.clusterAdmission.states.Len() != 0 || e.srv.clusterAdmission.connects.Len() != 0 {
				t.Fatal("malformed request allocated node bucket")
			}
			if tc.status == 400 && w.Header().Get("Connection") != "close" {
				t.Fatal("body rejection did not close connection")
			}
		})
	}
}

func TestClusterAdmissionFailureLockoutPrecedesCredentialLookup(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	now := time.Now()
	e.srv.now = func() time.Time { return now }
	token, err := e.app.Store().CreateAgentToken("edge1")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if w := admissionRequest(e, cluster.StatePath, "bad", ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	if w := admissionRequest(e, cluster.StatePath, token, ""); w.Code != 429 || w.Header().Get("Retry-After") != "12" {
		t.Fatalf("credential lockout: %d %v", w.Code, w.Header())
	}
	tokens, err := e.app.Store().AgentTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].LastUsed != nil {
		t.Fatal("credential lookup happened despite failure lockout")
	}
	now = now.Add(12 * time.Second)
	if w := admissionRequest(e, cluster.StatePath, token, ""); w.Code != 200 {
		t.Fatalf("refilled authenticated request: %d", w.Code)
	}
}

func TestClusterAdmissionAmbiguousNodeCannotAllocateBucket(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	token, err := e.app.Store().CreateAgentToken("edge1")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "https://localhost"+cluster.StatePath, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Add(cluster.NodeHeader, "edge1")
	r.Header.Add(cluster.NodeHeader, "edge2")
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	if w.Code != 403 || e.srv.clusterAdmission.states.Len() != 0 {
		t.Fatalf("ambiguous node: %d", w.Code)
	}
}
