package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/config"
)

func syncFixture(t *testing.T) (*App, ZoneRecord, zoneSnapshot) {
	t.Helper()
	a := runtimeApp(t)
	z, err := a.ImportZone([]byte(runtimeSOA), ZoneImportOptions{Name: "copy"})
	if err != nil {
		t.Fatal(err)
	}
	b := ZoneBinding{SourceNode: "source", Fingerprint: strings.Repeat("ab", 32), SourceZoneID: "11111111-1111-4111-8111-111111111111", SourceGeneration: 2}
	z, err = a.RebindZone(z.Header.ZoneID, z.Revision(), b)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(runtimeSOA, "( 1 ", "( 10 ", 1) + "check IN HTTP http://127.0.0.1/\n"
	s := zoneSnapshot{Header: config.ZoneHeader{ZoneID: b.SourceZoneID, Name: "sourcezone", SourceNode: "source", Generation: 2, Serial: 10, ContentHash: config.ZoneHash([]byte(body))}, Body: body}
	return a, z, s
}

func TestZoneSnapshotRejectsMismatchesAndPreservesLastGood(t *testing.T) {
	a, z, s := syncFixture(t)
	good, err := a.acceptZoneSnapshot(z.Header.ZoneID, z.Revision(), s)
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.store.Setting(zoneAppliedSetting)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*zoneSnapshot){
		"zone":        func(s *zoneSnapshot) { s.Header.ZoneID = "22222222-2222-4222-8222-222222222222" },
		"generation":  func(s *zoneSnapshot) { s.Header.Generation++ },
		"source":      func(s *zoneSnapshot) { s.Header.SourceNode = "old-source" },
		"hash":        func(s *zoneSnapshot) { s.Header.ContentHash = strings.Repeat("0", 64) },
		"serial-body": func(s *zoneSnapshot) { s.Header.Serial++ },
		"same-serial-new-body": func(s *zoneSnapshot) {
			s.Body += "another IN HTTP http://127.0.0.1/\n"
			s.Header.ContentHash = config.ZoneHash([]byte(s.Body))
		},
		"backwards": func(s *zoneSnapshot) {
			s.Header.Serial = 9
			s.Body = strings.Replace(s.Body, "( 10 ", "( 9 ", 1)
			s.Header.ContentHash = config.ZoneHash([]byte(s.Body))
		},
		"jump": func(s *zoneSnapshot) {
			s.Header.Serial = 2000000
			s.Body = strings.Replace(s.Body, "( 10 ", "( 2000000 ", 1)
			s.Header.ContentHash = config.ZoneHash([]byte(s.Body))
		},
		"missing-secret": func(s *zoneSnapshot) {
			s.Header.Serial = 11
			s.Body = strings.Replace(s.Body, "( 10 ", "( 11 ", 1) + "out IN CHANNEL webhook url=${secret:absent}\n"
			s.Header.ContentHash = config.ZoneHash([]byte(s.Body))
		},
		"embedded-origin": func(s *zoneSnapshot) {
			s.Body = strings.Replace(s.Body, "\n", "\n$ZONE zone_id=11111111-1111-4111-8111-111111111111 name=spoof generation=2\n", 1)
			s.Header.ContentHash = config.ZoneHash([]byte(s.Body))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			bad := s
			mutate(&bad)
			if _, err := a.acceptZoneSnapshot(z.Header.ZoneID, good.Revision(), bad); err == nil {
				t.Fatal("accepted invalid snapshot")
			}
			after, _ := a.store.Setting(zoneAppliedSetting)
			draft, e := a.zones.Get(z.Header.ZoneID)
			if e != nil || after != before || draft.Revision() != good.Revision() {
				t.Fatal("rejected update changed last-good or draft")
			}
		})
	}
	// A newly trusted source may start with a smaller serial, but the old source
	// cannot reclaim authority using a larger serial or a previously captured CAS.
	rebound, err := a.RebindZone(z.Header.ZoneID, good.Revision(), ZoneBinding{SourceNode: "new-source", Fingerprint: strings.Repeat("cd", 32), SourceZoneID: s.Header.ZoneID, SourceGeneration: 3})
	if err != nil {
		t.Fatal(err)
	}
	old := s
	old.Header.Serial = 99
	old.Body = strings.Replace(old.Body, "( 10 ", "( 99 ", 1)
	old.Header.ContentHash = config.ZoneHash([]byte(old.Body))
	if _, err = a.acceptZoneSnapshot(z.Header.ZoneID, good.Revision(), old); !errors.Is(err, ErrConflict) {
		t.Fatal("old CAS accepted:", err)
	}
	if _, err = a.acceptZoneSnapshot(z.Header.ZoneID, rebound.Revision(), old); err == nil {
		t.Fatal("old source with higher serial accepted")
	}
	fresh := s
	fresh.Header.SourceNode = "new-source"
	fresh.Header.Generation = 3
	fresh.Header.Serial = 1
	fresh.Body = strings.Replace(fresh.Body, "( 10 ", "( 1 ", 1)
	fresh.Header.ContentHash = config.ZoneHash([]byte(fresh.Body))
	if _, err = a.acceptZoneSnapshot(z.Header.ZoneID, rebound.Revision(), fresh); err != nil {
		t.Fatal("new source initial serial:", err)
	}
}

func TestZoneSyncTransportPinCancellationAndRebinding(t *testing.T) {
	for _, scenario := range []string{"wrong-pin", "cancel", "rebind", "oversize", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			a, z, s := syncFixture(t)
			source := runtimeApp(t)
			entered, release := make(chan struct{}), make(chan struct{})
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "cancel", "rebind":
					close(entered)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				case "oversize":
					_, _ = w.Write([]byte(strings.Repeat("x", 4*MaxZoneBytes+1)))
					return
				case "redirect":
					http.Redirect(w, r, "https://127.0.0.1:1/", http.StatusFound)
					return
				}
				_ = json.NewEncoder(w).Encode(s)
			}))
			srv.TLS = source.PKI().TLSConfig(nil)
			srv.StartTLS()
			defer srv.Close()
			if scenario != "wrong-pin" {
				b := z.Binding
				b.Fingerprint = strings.TrimPrefix(source.PKI().Fingerprint(), "sha256:")
				var err error
				z, err = a.RebindZone(z.Header.ZoneID, z.Revision(), b)
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := a.SyncZone(ctx, z.Header.ZoneID, srv.URL, "fixture"); done <- err }()
			if scenario == "cancel" || scenario == "rebind" {
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("request not started")
				}
				if scenario == "cancel" {
					cancel()
				} else {
					b := z.Binding
					b.SourceNode = "changed"
					if _, err := a.RebindZone(z.Header.ZoneID, z.Revision(), b); err != nil {
						t.Fatal(err)
					}
					close(release)
				}
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("bad transport response applied")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request not bounded/cancelled")
			}
			if len(a.RuntimeConfig().Monitors) != 0 {
				t.Fatal("failed response published")
			}
		})
	}
}

func TestZoneSyncLoopBackoffAndCancellation(t *testing.T) {
	a, z, _ := syncFixture(t)
	source := runtimeApp(t)
	var calls atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	srv.TLS = source.PKI().TLSConfig(nil)
	srv.StartTLS()
	defer srv.Close()
	b := z.Binding
	b.Fingerprint = strings.TrimPrefix(source.PKI().Fingerprint(), "sha256:")
	var err error
	z, err = a.RebindZone(z.Header.ZoneID, z.Revision(), b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	err = a.RunZoneSync(ctx, z.Header.ZoneID, ZoneSyncOptions{Endpoint: srv.URL, Token: "fixture", Interval: time.Second})
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 2 {
		t.Fatalf("backoff/cancel: calls=%d err=%v", calls.Load(), err)
	}
	for _, interval := range []time.Duration{time.Nanosecond, 2 * time.Hour} {
		if err := a.RunZoneSync(context.Background(), z.Header.ZoneID, ZoneSyncOptions{Interval: interval}); err == nil {
			t.Fatal("invalid interval accepted")
		}
	}
}
