package store

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"testing"
	"time"
)

// Both readers' aggregate paths (rather than only the raw round path) must be
// tenant isolated. Identity is unrelated to a target or definition hash.
func TestTenantMonitorAggregateTiers(t *testing.T) {
	for _, age := range []time.Duration{2 * time.Hour, 48 * time.Hour} {
		t.Run(age.String(), func(t *testing.T) {
			s, _, _ := open(t)
			a, b, _ := tenantMonitorRepos(t, s)
			s.ret.Store(&Retention{Raw: time.Hour, Agg5m: 24 * time.Hour, Agg1h: 90 * 24 * time.Hour})
			s.agg5mSince = s.now().Add(-96 * time.Hour)
			at := s.now().Add(-age).Truncate(time.Hour)
			for i, r := range []*MonitorRepository{a, b} {
				if err := r.SyncMonitors([]MonitorDef{{"same", "h"}}); err != nil {
					t.Fatal(err)
				}
				id, _ := r.MonitorID("same")
				status := "ok"
				lat := 10.0
				if i == 1 {
					status = "critical"
					lat = 80
				}
				for j := 0; j < 2; j++ {
					if err := r.Verdict(id, engine.VerdictRecord{Monitor: "same", At: at.Add(time.Duration(j) * time.Minute), Status: status, Hard: true, LatencyMS: lat, Interval: time.Minute}); err != nil {
						t.Fatal(err)
					}
				}
				u, err := r.UptimeOf(id, at, at.Add(time.Hour))
				if err != nil || u.UpS+u.DownS != 60 || (i == 0 && u.DownS != 0) || (i == 1 && u.UpS != 0) {
					t.Fatalf("uptime %+v %v", u, err)
				}
				pts, err := r.Latency(id, at, at.Add(time.Hour))
				if err != nil || len(pts) != 1 || pts[0].Avg != lat {
					t.Fatalf("aggregate latency %+v %v", pts, err)
				}
				cells, err := r.Cells(id, at, at.Add(time.Hour), 1)
				if err != nil || len(cells) != 1 || cells[0].Status != status {
					t.Fatalf("aggregate cells %+v %v", cells, err)
				}
			}
		})
	}
}

func TestTenantMonitorInternalDefaultBoundary(t *testing.T) {
	s, _, _ := open(t)
	a, _, cb := tenantMonitorRepos(t, s)
	if err := s.syncMonitors(cb.TenantID, nil, []MonitorDef{{"unsafe", "h"}}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("implicit trusted nondefault access: %v", err)
	}
	if err := s.syncMonitors(cb.TenantID, &a.ctx, []MonitorDef{{"unsafe", "h"}}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("mismatched context: %v", err)
	}
}

func TestTenantMonitorReviveIsTenantScoped(t *testing.T) {
	s, _, _ := open(t)
	a, b, _ := tenantMonitorRepos(t, s)
	for _, r := range []*MonitorRepository{a, b} {
		if err := r.SyncMonitors([]MonitorDef{{"same", "h"}}); err != nil {
			t.Fatal(err)
		}
	}
	ai, _ := a.MonitorID("same")
	bi, _ := b.MonitorID("same")
	if err := b.SyncMonitors(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.MonitorID("same"); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("retired resolution %v", err)
	}
	if err := b.SyncMonitors([]MonitorDef{{"same", "h"}}); err != nil {
		t.Fatal(err)
	}
	if id, err := b.MonitorID("same"); err != nil || id != bi {
		t.Fatalf("revival ID %d %v", id, err)
	}
	if id, err := a.MonitorID("same"); err != nil || id != ai {
		t.Fatalf("cross revival %d %v", id, err)
	}
	if err := b.SyncMonitors(nil); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncMonitors([]MonitorDef{{"same", "different"}}); err != nil {
		t.Fatal(err)
	}
	if id, err := b.MonitorID("same"); err != nil || id == bi || id == ai {
		t.Fatalf("retired ID reused %d %v", id, err)
	}
}
