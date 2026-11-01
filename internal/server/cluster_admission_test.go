package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
)

const admissionBind = `@ IN SOA master. admin.localhost. ( 2026103001 30s 5s 1s 1 )
edge1 IN NODE standby=https://127.0.0.1:1
edge2 IN NODE standby=https://127.0.0.1:2
edge3 IN NODE standby=https://127.0.0.1:3
probe IN NODE
`

func admissionRequest(e *env, path, token, node string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "https://localhost"+path, nil)
	r.RemoteAddr = "192.0.2.10:1234"
	r.TLS = &tls.ConnectionState{}
	r.Header.Set("Authorization", "Bearer "+token)
	if node != "" {
		r.Header.Set(cluster.NodeHeader, node)
	}
	if path == cluster.ConnectPath {
		r.Header.Set("Connection", "Upgrade")
		r.Header.Set("Upgrade", cluster.Upgrade)
	}
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	return w
}

func TestClusterAdmissionRejectBodyBeforeAuthentication(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	token, _ := e.app.Store().CreateAgentToken("edge1")
	r := httptest.NewRequest("GET", "https://localhost"+cluster.StatePath, strings.NewReader("unexpected"))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("body status: %d", w.Code)
	}
	tokens, _ := e.app.Store().AgentTokens()
	if tokens[0].LastUsed != nil {
		t.Fatal("body validated after token DB write")
	}
}

func TestClusterAdmissionRejectUpgradeBeforeAuthentication(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	token, _ := e.app.Store().CreateAgentToken("edge1")
	r := httptest.NewRequest("GET", "https://localhost"+cluster.ConnectPath, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	if w.Code != 426 {
		t.Fatalf("upgrade status: %d", w.Code)
	}
	tokens, _ := e.app.Store().AgentTokens()
	if tokens[0].LastUsed != nil {
		t.Fatal("upgrade validated after token DB write")
	}
}

type admissionBlockingWriter struct {
	*httptest.ResponseRecorder
	started, release chan struct{}
	once             sync.Once
}

func (w *admissionBlockingWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.started); <-w.release })
	return w.ResponseRecorder.Write(b)
}

func TestClusterAdmissionStateConcurrency(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	token1, _ := e.app.Store().CreateAgentToken("edge1")
	token2, _ := e.app.Store().CreateAgentToken("edge2")
	token3, _ := e.app.Store().CreateAgentToken("edge3")
	start := func(token string) {
		w := &admissionBlockingWriter{ResponseRecorder: httptest.NewRecorder(), started: make(chan struct{}), release: make(chan struct{})}
		r := httptest.NewRequest("GET", "https://localhost"+cluster.StatePath, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		done := make(chan struct{})
		go func() { e.srv.Handler().ServeHTTP(w, r); close(done) }()
		t.Cleanup(func() { close(w.release); <-done })
		select {
		case <-w.started:
		case <-time.After(5 * time.Second):
			t.Fatal("archive not started")
		}
	}
	start(token1)
	if w := admissionRequest(e, cluster.StatePath, token1, "edge1"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("parallel same node: %d", w.Code)
	}
	start(token2)
	if w := admissionRequest(e, cluster.StatePath, token3, "edge3"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("parallel global state cap: %d", w.Code)
	}
}

func TestClusterAdmissionSharedIPDefaultState(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	now := time.Now()
	e.srv.now = func() time.Time { return now }
	if got := e.app.Config().Config.Tunables.Rate("api.login-rate"); got != 5.0/60 {
		t.Fatalf("not default login rate: %v", got)
	}
	started := time.Now()
	tokens := make(map[string]string)
	for i := 1; i <= 3; i++ {
		node := fmt.Sprintf("edge%d", i)
		token, err := e.app.Store().CreateAgentToken(node)
		if err != nil {
			t.Fatal(err)
		}
		// Recorder cannot hijack; reaching 500 proves admission passed to the
		// upgrade handler without creating a long-lived fixture connection.
		if w := admissionRequest(e, cluster.ConnectPath, token, node); w.Code != 500 {
			t.Fatalf("%s connect: %d %s", node, w.Code, w.Body.String())
		}
		tokens[node] = token
	}
	for i := 1; i <= 3; i++ {
		node := fmt.Sprintf("edge%d", i)
		token := tokens[node]
		for n := 0; n < 2; n++ {
			w := admissionRequest(e, cluster.StatePath, token, node)
			if w.Code != 200 || w.Body.Len() == 0 {
				t.Fatalf("%s state %d: %d %s", node, n, w.Code, w.Body.String())
			}
		}
	}
	t.Logf("DEFAULT_ADMISSION 3 nodes, 3 connect admissions, 6 real archives, same IP: %s", time.Since(started))
}
