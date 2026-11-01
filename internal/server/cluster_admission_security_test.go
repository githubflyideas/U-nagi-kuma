package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/config"
)

func TestClusterAdmissionSecurity(t *testing.T) {
	for _, tc := range []struct {
		name, node, want string
		status           int
	}{
		{"invalid", "", "edge1", 401}, {"mismatch", "edge1", "edge2", 403},
		{"undeclared", "ghost", "ghost", 403}, {"local master", "master", "master", 403},
		{"probe cannot fetch", "probe", "probe", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, admissionBind, nil)
			now := time.Now()
			e.srv.now = func() time.Time { return now }
			token := "invalid"
			if tc.node != "" {
				var err error
				token, err = e.app.Store().CreateAgentToken(tc.node)
				if err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 4; i++ {
				w := admissionRequest(e, cluster.StatePath, token, tc.want)
				want := tc.status
				if i == 3 {
					want = 429
				}
				if w.Code != want {
					t.Fatalf("attempt %d: %d != %d", i, w.Code, want)
				}
				if w.Header().Get(cluster.StateTermHeader) != "" {
					t.Fatal("unauthorized archive started")
				}
			}
			if e.srv.clusterAdmission.states.Len() != 0 || e.srv.clusterAdmission.connects.Len() != 0 {
				t.Fatal("unvalidated identity allocated a workload bucket")
			}
		})
	}
}

func TestClusterAdmissionWorkloadIsolationAndRevocation(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	now := time.Now()
	e.srv.now = func() time.Time { return now }
	token, _ := e.app.Store().CreateAgentToken("edge1")
	other, _ := e.app.Store().CreateAgentToken("edge2")
	// Exhaust interactive/enrollment quota; valid machine work still proceeds.
	for i := 0; i < 3; i++ {
		e.srv.login.Allow("192.0.2.10", now)
	}
	for i := 0; i < 3; i++ {
		w := admissionRequest(e, cluster.StatePath, token, "")
		want := 200
		if i == 2 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("state %d: %d", i, w.Code)
		}
	}
	// New token for the same node cannot bypass its state budget.
	rotated, err := e.app.Store().CreateAgentToken("edge1")
	if err != nil {
		t.Fatal(err)
	}
	if w := admissionRequest(e, cluster.StatePath, rotated, ""); w.Code != 429 {
		t.Fatalf("rotated token bypass: %d", w.Code)
	}
	if w := admissionRequest(e, cluster.StatePath, other, ""); w.Code != 200 {
		t.Fatalf("other node: %d", w.Code)
	}
	for i := 0; i < 4; i++ {
		w := admissionRequest(e, cluster.ConnectPath, rotated, "")
		want := 500
		if i == 3 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("connect %d: %d", i, w.Code)
		}
	}
	now = now.Add(10 * time.Second)
	if w := admissionRequest(e, cluster.StatePath, rotated, ""); w.Code != 200 {
		t.Fatalf("state refill: %d", w.Code)
	}
	if w := admissionRequest(e, cluster.StatePath, token, ""); w.Code != 401 {
		t.Fatalf("old token accepted: %d", w.Code)
	}
	if err := e.app.Store().DeleteAgentToken("edge1"); err != nil {
		t.Fatal(err)
	}
	if w := admissionRequest(e, cluster.StatePath, rotated, ""); w.Code != 401 {
		t.Fatalf("revoked token accepted: %d", w.Code)
	}
}

func TestClusterAdmissionConcurrentFailures(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	now := time.Now()
	e.srv.now = func() time.Time { return now }
	var wg sync.WaitGroup
	codes := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- admissionRequest(e, cluster.StatePath, "bad", "").Code }()
	}
	wg.Wait()
	close(codes)
	unauthorized, limited := 0, 0
	for code := range codes {
		switch code {
		case 401:
			unauthorized++
		case 429:
			limited++
		default:
			t.Fatalf("status %d", code)
		}
	}
	if unauthorized != 3 || limited != 9 {
		t.Fatalf("credential budget escaped: unauthorized=%d limited=%d", unauthorized, limited)
	}
	now = now.Add(12 * time.Second)
	if w := admissionRequest(e, cluster.StatePath, "bad", ""); w.Code != 401 {
		t.Fatalf("failure refill: %d", w.Code)
	}
}

func TestClusterAdmissionValidationQueueBound(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	_ = admissionRequest(e, cluster.StatePath, "bad", "") // initialize admission state
	a := &e.srv.clusterAdmission
	a.mu.Lock()
	var unlock sync.Once
	release := func() { unlock.Do(a.mu.Unlock) }
	var wg sync.WaitGroup
	defer func() { release(); wg.Wait() }()
	codes := make(chan int, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := httptest.NewRequest("GET", "https://localhost"+cluster.StatePath, nil)
			r.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", i)
			w := httptest.NewRecorder()
			e.srv.Handler().ServeHTTP(w, r)
			codes <- w.Code
		}(i)
	}
	deadline := time.Now().Add(time.Second)
	for len(a.slots) != 16 {
		if time.Now().After(deadline) {
			t.Fatal("validation queue did not fill")
		}
		time.Sleep(time.Millisecond)
	}
	if w := admissionRequest(e, cluster.StatePath, "bad", ""); w.Code != 429 {
		t.Fatalf("full validation queue: %d", w.Code)
	}
	release()
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 401 {
			t.Fatalf("queued validation: %d", code)
		}
	}
}

func TestClusterAdmissionGlobalValidationBound(t *testing.T) {
	e := newEnv(t, admissionBind, nil)
	now := time.Now()
	e.srv.now = func() time.Time { return now }
	for i := 0; i < 101; i++ {
		r := httptest.NewRequest("GET", "https://localhost"+cluster.StatePath, nil)
		r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", i)
		w := httptest.NewRecorder()
		e.srv.Handler().ServeHTTP(w, r)
		want := 401
		if i == 100 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
}

func TestClusterAdmissionUnauthenticatedEntryBudgets(t *testing.T) {
	for _, path := range []string{cluster.JoinPath, cluster.RetirePath, "/api/v1/setup", "/api/v1/session"} {
		t.Run(path, func(t *testing.T) {
			e := newEnv(t, admissionBind, nil)
			now := time.Now()
			e.srv.now = func() time.Time { return now }
			if path == "/api/v1/session" {
				e.setup()
			}
			for i := 0; i < 3; i++ {
				e.srv.login.Allow("192.0.2.10", now)
			}
			r := httptest.NewRequest("POST", "https://localhost"+path, strings.NewReader(`{"username":"wrong","password":"wrong"}`))
			r.RemoteAddr = "192.0.2.10:1234"
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			e.srv.Handler().ServeHTTP(w, r)
			if w.Code != 429 {
				t.Fatalf("exhausted unauthenticated endpoint: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

// Real loopback TLS, one-time join codes, SQLite-backed credentials, actual
// upgraded sessions and encrypted archives. No rate/scheduling overrides.
func TestClusterAdmissionDefaultJoinStateTiming(t *testing.T) {
	e := newEnv(t, "@ IN SOA master. admin.localhost. ( 2026103001 30s 5s 1s 1 )\n", nil)
	e.setup() // consumes one of the default shared login burst's three tokens
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: e.srv.Handler()}
	done := make(chan struct{})
	go func() { _ = hs.Serve(cluster.SniffListener(ln, e.app.PKI().TLSConfig(nil), time.Second)); close(done) }()
	t.Cleanup(func() { hs.Close(); <-done })
	url := "https://" + ln.Addr().String()
	total := time.Now()
	for i := 1; i <= 2; i++ {
		node := fmt.Sprintf("joined%d", i)
		code, _, err := e.app.CreateJoinCode(url)
		if err != nil {
			t.Fatal(err)
		}
		info, err := cluster.DecodeJoin(code)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		joined, err := cluster.Join(t.Context(), info, cluster.JoinRequest{Code: info.Code, Node: node})
		if err != nil {
			t.Fatal(err)
		}
		joinElapsed := time.Since(start)
		_, err = e.app.EditConfig("test", "enable standby", func(f *config.File) error {
			return f.UpsertRecord(config.RecordSpec{Name: node, Type: "NODE", Opts: []config.Opt{{Key: "standby", Value: url}}})
		})
		if err != nil {
			t.Fatal(err)
		}
		ag, err := cluster.NewAgent(cluster.AgentOptions{Server: url, Node: node, Token: joined.Token, Fingerprint: info.Fingerprint, Log: e.srv.log})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		agentDone := make(chan struct{})
		go func() { _ = ag.Run(ctx); close(agentDone) }()
		t.Cleanup(func() { cancel(); <-agentDone })
		deadline := time.Now().Add(5 * time.Second)
		for !ag.Status().Connected {
			if time.Now().After(deadline) {
				t.Fatalf("connect: %+v", ag.Status())
			}
			time.Sleep(time.Millisecond)
		}
		connected := time.Since(start)
		var archive bytes.Buffer
		key, _, err := ag.FetchState(t.Context(), &archive)
		if err != nil {
			t.Fatal(err)
		}
		if key == "" || archive.Len() == 0 {
			t.Fatal("empty archive")
		}
		first := time.Since(start)
		archive.Reset()
		if _, _, err := ag.FetchState(t.Context(), &archive); err != nil {
			t.Fatal(err)
		}
		t.Logf("DEFAULT_JOIN_STATE node=%s join_ms=%.3f connected_ms=%.3f first_archive_ms=%.3f second_archive_ms=%.3f archive_bytes=%d", node, float64(joinElapsed)/float64(time.Millisecond), float64(connected)/float64(time.Millisecond), float64(first)/float64(time.Millisecond), float64(time.Since(start))/float64(time.Millisecond), archive.Len())
	}
	t.Logf("DEFAULT_JOIN_STATE_TOTAL two joins, two live sessions, four archives: %s", time.Since(total))
}
