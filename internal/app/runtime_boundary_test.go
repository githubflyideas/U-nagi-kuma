package app

import (
	"context"
	"encoding/json"
	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/store"
	"testing"
	"time"
)

func foreignRuntimeFixture(t *testing.T, a *App) *store.MonitorRepository {
	t.Helper()
	uid, err := a.store.CreateUser("runtime-owner", "local-password")
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := a.store.DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	owner := store.TenantContext{TenantID: tenant.ID, UserID: uid}
	ws, err := a.store.CreateWorkspace(owner, "foreign")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := a.store.WorkspaceMonitors(store.WorkspaceContext{TenantContext: owner, WorkspaceID: ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.SyncMonitors([]store.MonitorDef{{Name: "foreign", DefHash: "owned"}}); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestRuntimeRegistrationFailureKeepsPreviousEngine(t *testing.T) {
	a, _, _, _ := newApp(t, testConfig)
	defer a.store.Close()
	repo := foreignRuntimeFixture(t, a)
	before := a.RuntimeConfig()
	_, cfg, d := config.Load("candidate.bind", []byte(testConfig+"foreign IN HTTP http://127.0.0.1/\n"), a.validators())
	if config.HasErrors(d) {
		t.Fatal(d)
	}
	a.applyConfig(cfg)
	if a.RuntimeConfig() != before {
		t.Fatal("registration failure published unowned runtime")
	}
	if _, err := repo.MonitorID("foreign"); err != nil {
		t.Fatal("foreign identity retired", err)
	}
}

func TestRuntimeZoneRegistrationFailureAtomic(t *testing.T) {
	a, _, _, _ := newApp(t, testConfig)
	defer a.store.Close()
	repo := foreignRuntimeFixture(t, a)
	z, err := a.ImportZone([]byte("@ IN SOA local. admin.localhost. ( 1 1s 500ms 300ms 2 )\nweb IN HTTP http://127.0.0.1/\n"), ZoneImportOptions{Name: "candidate"})
	if err != nil {
		t.Fatal(err)
	}
	name := "zone-" + z.Header.ZoneID + ".web"
	if err = repo.SyncMonitors([]store.MonitorDef{{Name: name, DefHash: "owned"}}); err != nil {
		t.Fatal(err)
	}
	before := a.RuntimeConfig()
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err == nil {
		t.Fatal("zone published despite ownership registration rejection")
	} else {
		t.Logf("rejected: %v", err)
	}
	if a.RuntimeConfig() != before {
		t.Fatal("failed zone changed effective configuration")
	}
	if len(a.zoneApplied) != 0 {
		t.Fatal("failed zone changed applied manifest")
	}
	raw, err := a.store.Setting(zoneAppliedSetting)
	if err != nil || raw != "" {
		t.Fatalf("failed zone persisted restart snapshot: %q %v", raw, err)
	}
}

func TestRuntimeStartupRejectsUnregisteredSnapshot(t *testing.T) {
	a, _, _, dir := newApp(t, testConfig)
	repo := foreignRuntimeFixture(t, a)
	z, err := a.ImportZone([]byte("@ IN SOA local. admin.localhost. ( 1 1s 500ms 300ms 2 )\nweb IN HTTP http://127.0.0.1/\n"), ZoneImportOptions{Name: "candidate"})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.SyncMonitors([]store.MonitorDef{{Name: "zone-" + z.Header.ZoneID + ".web", DefHash: "owned"}}); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]ZoneRecord{z.Header.ZoneID: z})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.store.SetSetting(zoneAppliedSetting, string(raw)); err != nil {
		t.Fatal(err)
	}
	a.store.Close()
	next, err := New(Options{DataDir: dir})
	if err == nil {
		next.store.Close()
		t.Fatal("startup accepted snapshot whose monitor registration failed")
	}
}

func TestRuntimeRestoreDoesNotAdoptForeignIncidents(t *testing.T) {
	a, fp, _, dir := newApp(t, testConfig)
	foreignRuntimeFixture(t, a)
	inc := &engine.Incident{Monitor: "foreign", Kind: engine.IncidentNormal, Policy: "default", State: engine.IncidentState{OpenedAt: time.Now(), HardAt: time.Now(), Tracked: true}}
	// Simulates a pre-boundary unscoped record. The global Store API is not a
	// workspace repository and remains outside the department activation contract.
	if err := a.store.SaveIncident(inc); err != nil {
		t.Fatal(err)
	}
	a.store.Close()
	next, err := New(Options{DataDir: dir, Probe: fp.run})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- next.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	for _, got := range next.engine.OpenIncidents() {
		if got.ID == inc.ID {
			t.Fatal("foreign incident restored into default engine")
		}
	}
}

func TestRuntimeSinkRejectsForeignRecords(t *testing.T) {
	a, _, _, _ := newApp(t, testConfig)
	defer a.store.Close()
	foreignRuntimeFixture(t, a)
	published := 0
	a.SetPublisher(func(string, any) { published++ })
	s := &sink{Store: a.store, a: a}
	s.SaveState(engine.StateRecord{Name: "foreign"})
	s.Verdict(engine.VerdictRecord{Monitor: "foreign"})
	s.Event(engine.Event{Monitor: "foreign", Message: "private payload"})
	s.Event(engine.Event{Monitor: "never-registered", Message: "unowned payload"})
	s.SaveState(engine.StateRecord{Name: "never-registered"})
	s.Verdict(engine.VerdictRecord{Monitor: "never-registered"})
	if published != 0 {
		t.Fatalf("foreign records globally published: %d", published)
	}
	if err := s.SaveIncident(&engine.Incident{Monitor: "foreign"}); err == nil {
		t.Fatal("foreign incident admitted")
	}
	if err := s.SaveIntent(&engine.Intent{ID: "foreign-intent", Message: notify.Message{Monitor: "foreign"}}); err == nil {
		t.Fatal("foreign notification admitted")
	}
	a.store.Flush()
	for _, name := range []string{"foreign", "never-registered"} {
		page, err := a.store.Events(store.EventFilter{Monitor: name})
		if err != nil || len(page.Items) != 0 {
			t.Fatalf("rejected event persisted: %s %v", name, err)
		}
		pageInc, err := a.store.Incidents("all", name, 0, 10)
		if err != nil || len(pageInc.Items) != 0 {
			t.Fatalf("rejected incident persisted: %s %v", name, err)
		}
	}
	if a.store.MonitorID("never-registered") != 0 {
		t.Fatal("rejected verdict manufactured monitor identity")
	}
	states, err := a.store.States()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range states {
		if r.Name == "foreign" || r.Name == "never-registered" {
			t.Fatal("rejected state persisted")
		}
	}
}
