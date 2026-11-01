package server

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/ratelimit"
)

// Credential failures have the login rate, but successful machine traffic
// neither consumes that quota nor competes with interactive login/enrollment.
// Serializing Check/validation/Allow prevents concurrent guesses overshooting
// the failure budget. No plaintext token or claimed node is used as a key.
// Validation is globally rate/queue bounded before the serialized DB lookup;
// state generation/streaming is limited to one per node and two overall.
type clusterAdmission struct {
	once                                   sync.Once
	mu                                     sync.Mutex
	failures, connects, states, validation *ratelimit.Keyed
	slots                                  chan struct{}
	active                                 map[string]bool
	failureRate                            float64
}

func (s *Server) admitAgent(w http.ResponseWriter, r *http.Request, state bool) (release func(), admitted bool) {
	a := &s.clusterAdmission
	a.once.Do(func() {
		a.failureRate = s.app.Config().Config.Tunables.Rate("api.login-rate")
		a.failures = ratelimit.NewKeyed(a.failureRate, 3, 0)
		a.connects = ratelimit.NewKeyed(12.0/60, 3, 0)
		a.states = ratelimit.NewKeyed(6.0/60, 2, 0)
		a.validation = ratelimit.NewKeyed(100, 100, 1)
		a.slots = make(chan struct{}, 16)
		a.active = make(map[string]bool)
	})
	if ok, wait := a.validation.Allow("all", s.now()); !ok {
		writeError(w, rateLimited(wait))
		return nil, false
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		writeError(w, rateLimited(time.Second))
		return nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if rate := s.app.Config().Config.Tunables.Rate("api.login-rate"); rate != a.failureRate {
		a.failures.SetRate(rate, 3)
		a.failureRate = rate
	}
	ip, now := s.ip(r), s.now()
	if ok, wait := a.failures.Check(ip, now); !ok {
		writeError(w, rateLimited(wait))
		return nil, false
	}
	reject := func(status int, message string) (func(), bool) {
		a.failures.Allow(ip, now)
		http.Error(w, message, status)
		return nil, false
	}
	if r.TLS == nil {
		return reject(http.StatusForbidden, "TLS required")
	}
	// Validate framing and upgrade before reading any body or touching credentials.
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		w.Header().Set("Connection", "close")
		r.Close = true
		return reject(http.StatusBadRequest, "agent requests must not have a body")
	}
	if !state {
		upgrade := false
		for _, v := range r.Header.Values("Connection") {
			for _, part := range strings.Split(v, ",") {
				if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
					upgrade = true
				}
			}
		}
		if !upgrade || !strings.EqualFold(r.Header.Get("Upgrade"), cluster.Upgrade) {
			w.Header().Set("Upgrade", cluster.Upgrade)
			return reject(http.StatusUpgradeRequired, "agent upgrade required")
		}
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(r.Header.Values("Authorization")) != 1 || !ok || token == "" || len(token) > 256 {
		return reject(http.StatusUnauthorized, "invalid agent token")
	}
	node, err := s.app.Store().AgentNodeByToken(token, r.RemoteAddr)
	if err != nil {
		return reject(http.StatusUnauthorized, "invalid agent token")
	}
	cfg := s.app.Config().Config
	n := cfg.FindNode(node)
	if n == nil || node == cfg.MName {
		return reject(http.StatusForbidden, "unknown agent node")
	}
	if len(r.Header.Values(cluster.NodeHeader)) > 1 {
		return reject(http.StatusForbidden, "ambiguous agent node")
	}
	if want := r.Header.Get(cluster.NodeHeader); want != "" && want != node {
		return reject(http.StatusForbidden, "agent node mismatch")
	}
	if state && n.Standby == "" {
		return reject(http.StatusForbidden, "node is not a standby")
	}
	if state && (a.active[node] || len(a.active) >= 2) {
		writeError(w, rateLimited(time.Second))
		return nil, false
	}
	limit := a.connects
	if state {
		limit = a.states
	}
	if ok, wait := limit.Allow(node, now); !ok {
		writeError(w, rateLimited(wait))
		return nil, false
	}
	if state {
		a.active[node] = true
		return func() { a.mu.Lock(); delete(a.active, node); a.mu.Unlock() }, true
	}
	return func() {}, true
}
