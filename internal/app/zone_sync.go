package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/config"
)

const ZoneSyncPath = "/api/v1/zone-sync"

type zoneSnapshot struct {
	Header config.ZoneHeader `json:"header"`
	Body   string            `json:"body"`
}

func zoneGrantKey(z ZoneRecord, node string) string {
	return "zone_sync_grant_v1_" + config.ZoneHash([]byte(fmt.Sprintf("%s/%d/%s", z.Header.ZoneID, z.Header.Generation, node)))
}

// GrantZoneSync is a local administrator operation. The returned capability is
// scoped to one zone generation and subscriber name. It grants no agent,
// standby, management or private-key access. Only its hash is persisted.
func (a *App) GrantZoneSync(id, node string) (string, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if !config.ValidName(node) {
		return "", fmt.Errorf("invalid subscriber")
	}
	z, err := a.zones.Get(id)
	if err != nil {
		return "", err
	}
	if z.Role(a.zones.local) != "primary" || a.SupersededBy() != nil {
		return "", ErrReadonly
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", err
	}
	token := "uzs_" + hex.EncodeToString(b)
	err = a.store.SetSetting(zoneGrantKey(z, node), config.ZoneHash([]byte(token)))
	if err != nil {
		return "", err
	}
	return token, nil
}

func (a *App) RevokeZoneSync(id, node string) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	z, err := a.zones.Get(id)
	if err != nil {
		return err
	}
	return a.store.SetSetting(zoneGrantKey(z, node), "")
}

// ZoneSyncHandler must be mounted on ZoneSyncPath on the cluster TLS listener.
// It performs its own zone-scoped capability authentication. No cookie/session
// or legacy agent credential authorizes this route. It serves APPLIED, not draft,
// configuration. The server must also impose connection/read-header limits.
func (a *App) ZoneSyncHandler() http.Handler {
	slots := make(chan struct{}, 4)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ZoneSyncPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.TLS == nil {
			http.Error(w, "TLS required", http.StatusForbidden)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		q := r.URL.Query()
		id, node := q.Get("zone_id"), r.Header.Get("X-Zone-Node")
		generation, err := strconv.ParseUint(q.Get("generation"), 10, 64)
		if !config.ValidZoneID(id) || !config.ValidName(node) || err != nil || generation == 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		a.applyMu.Lock()
		z, err := a.zones.Get(id)
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		want, e := a.store.Setting(zoneGrantKey(z, node))
		got := config.ZoneHash([]byte(token))
		if err != nil || e != nil || want == "" || len(token) != 68 || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
			a.applyMu.Unlock()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		applied, ok := a.zoneApplied[id]
		if a.SupersededBy() != nil || z.Role(a.zones.local) != "primary" || generation != z.Header.Generation || !ok || applied.Header.Generation != generation || applied.Header.SourceNode != a.zones.local {
			a.applyMu.Unlock()
			http.Error(w, "source generation not applied", http.StatusConflict)
			return
		}
		snapshot := zoneSnapshot{Header: applied.Header, Body: applied.Body}
		a.applyMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
}

// SyncZone fetches once using the locally authorized pin/remote identity and
// applies atomically against the binding revision captured BEFORE the request.
// endpoint and token are local operator inputs, never accepted from an import.
func (a *App) SyncZone(ctx context.Context, id, endpoint, token string) (ZoneRecord, error) {
	z, err := a.zones.Get(id)
	if err != nil {
		return ZoneRecord{}, err
	}
	b := z.Binding
	if z.Role(a.zones.local) != "secondary" || b.SourceZoneID == "" || b.SourceGeneration == 0 {
		return ZoneRecord{}, fmt.Errorf("explicit source zone binding required")
	}
	endpoint, err = cluster.NormalizeServer(endpoint)
	if err != nil {
		return ZoneRecord{}, err
	}
	client, err := cluster.ZoneHTTPClient(b.Fingerprint)
	if err != nil {
		return ZoneRecord{}, err
	}
	defer client.CloseIdleConnections()
	q := url.Values{"zone_id": {b.SourceZoneID}, "generation": {strconv.FormatUint(b.SourceGeneration, 10)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+ZoneSyncPath+"?"+q.Encode(), nil)
	if err != nil {
		return ZoneRecord{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Zone-Node", a.zones.local)
	resp, err := client.Do(req)
	if err != nil {
		return ZoneRecord{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ZoneRecord{}, fmt.Errorf("zone sync HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*MaxZoneBytes+1))
	if err != nil {
		return ZoneRecord{}, err
	}
	if len(raw) > 4*MaxZoneBytes {
		return ZoneRecord{}, fmt.Errorf("zone response too large")
	}
	var snapshot zoneSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return ZoneRecord{}, fmt.Errorf("invalid zone response")
	}
	if err = ctx.Err(); err != nil {
		return ZoneRecord{}, err
	}
	return a.acceptZoneSnapshot(id, z.Revision(), snapshot)
}

func (a *App) acceptZoneSnapshot(id, expected string, snapshot zoneSnapshot) (ZoneRecord, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return ZoneRecord{}, ErrSuperseded
	}
	a.zones.mu.Lock()
	defer a.zones.mu.Unlock()
	z, err := a.zones.get(id)
	if err != nil {
		return ZoneRecord{}, err
	}
	if z.Revision() != expected {
		return ZoneRecord{}, ErrConflict
	}
	b, h := z.Binding, snapshot.Header
	if z.Role(a.zones.local) != "secondary" || h.ZoneID != b.SourceZoneID || h.Generation != b.SourceGeneration || h.SourceNode != b.SourceNode || h.ContentHash != config.ZoneHash([]byte(snapshot.Body)) || len(snapshot.Body) > MaxZoneBytes {
		return ZoneRecord{}, fmt.Errorf("zone source identity or content mismatch")
	}
	origin, c, _, err := config.ParseZone("sync.bind", []byte(snapshot.Body), a.zones.validationOptions())
	if err != nil || origin != nil || c.Serial != h.Serial {
		return ZoneRecord{}, fmt.Errorf("invalid zone snapshot body or serial")
	}
	if z.Synced != nil {
		if err = config.CheckSerial(h.Serial, z.Synced.Serial); err != nil {
			return ZoneRecord{}, err
		}
		if h.Serial == z.Synced.Serial && h.ContentHash != z.Synced.ContentHash {
			return ZoneRecord{}, fmt.Errorf("same serial with different content")
		}
	}
	z.Body = snapshot.Body
	z.Header.Serial = h.Serial
	z.Header.ContentHash = h.ContentHash
	z.Synced = &h
	if previous, ok := a.zoneApplied[id]; ok && previous.Revision() == z.Revision() {
		return z, nil
	}
	next := maps.Clone(a.zoneApplied)
	next[id] = z
	combined, ec, err := a.composeRuntime(a.baseApplied, next, true)
	if err != nil {
		return ZoneRecord{}, err
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return ZoneRecord{}, err
	}
	// A failed SQLite commit may leave a newer draft, but never changes the
	// running/restart snapshot. Retrying that exact serial/hash remains safe.
	if err = a.zones.persist(z); err != nil {
		return ZoneRecord{}, err
	}
	if err = a.store.SetSetting(zoneAppliedSetting, string(raw)); err != nil {
		return ZoneRecord{}, err
	}
	a.zoneApplied = next
	a.installRuntime(a.baseApplied, combined, ec)
	return z, nil
}
