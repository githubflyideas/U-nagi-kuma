package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/config"
)

const runtimeSOA = "@ IN SOA local. admin.localhost. ( 1 1s 200ms 100ms 1 )\n"

func runtimeApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(runtimeSOA), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{DataDir: dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.store.Close() })
	return a
}

func TestZoneRuntimeMissingChannelDependencyBlocked(t *testing.T) {
	a := runtimeApp(t)
	t.Setenv("UNAGIKUMA_ZONE_CHANNEL_TOKEN", "local-fixture")
	z, err := a.ImportZone([]byte(runtimeSOA+"out IN CHANNEL webhook url=http://127.0.0.1:1/ header.X-Token=${env:UNAGIKUMA_ZONE_CHANNEL_TOKEN}\n"), ZoneImportOptions{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNAGIKUMA_ZONE_CHANNEL_TOKEN", "")
	_, ec, err := a.composeRuntime(a.baseApplied, a.zoneApplied)
	if err != nil {
		t.Fatal(err)
	}
	ch := ec.Channels["zone-"+z.Header.ZoneID+".out"]
	if ch.NoRecipients == "" {
		t.Fatal("missing channel credential leaves delivery enabled")
	}
}

func TestZoneRuntimeNamespaceCannotBeClaimedByBase(t *testing.T) {
	a := runtimeApp(t)
	z, err := a.ImportZone([]byte(runtimeSOA+"check IN HTTP http://127.0.0.1/\n"), ZoneImportOptions{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err != nil {
		t.Fatal(err)
	}
	before := a.Config()
	for _, record := range []string{" IN HTTP http://127.0.0.1/", " IN CHANNEL log"} {
		text := strings.Replace(runtimeSOA, "( 1 ", "( 2 ", 1) + "zone-" + z.Header.ZoneID + ".check" + record + "\n"
		if _, err = a.SaveConfig([]byte(text), before.ETag(), "test", "collision"); err == nil {
			t.Fatal("accepted base record in zone runtime namespace")
		}
		disk, err := os.ReadFile(a.ConfigPath())
		if err != nil || string(disk) != before.Text {
			t.Fatal("rejected base edit changed file")
		}
	}
}

func TestZoneAppliedSnapshotIntegrity(t *testing.T) {
	a := runtimeApp(t)
	for _, raw := range []string{"null", `{}`, `{"invalid":{}}`} {
		if err := a.store.SetSetting(zoneAppliedSetting, raw); err != nil {
			t.Fatal(err)
		}
		err := a.initZones()
		if raw == "{}" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatalf("accepted corrupt applied manifest %s", raw)
		}
	}
}

func TestZoneApplyRejectsMissingEnvWithoutPublishing(t *testing.T) {
	a := runtimeApp(t)
	t.Setenv("UNAGIKUMA_ZONE_TEST_MISSING", "")
	z, err := a.ImportZone([]byte(runtimeSOA+"out IN CHANNEL log\ncheck IN HTTP http://127.0.0.1/ header.X-Test=${env:UNAGIKUMA_ZONE_TEST_MISSING}\n"), ZoneImportOptions{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := a.store.Setting(zoneAppliedSetting)
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err == nil {
		t.Fatal("applied zone with missing environment dependency")
	}
	after, _ := a.store.Setting(zoneAppliedSetting)
	if before != after || len(a.applied.Load().Monitors) != 0 {
		t.Fatal("failed apply published state")
	}
}

func TestZoneTypedRuntimeDependencies(t *testing.T) {
	a := runtimeApp(t)
	raw := runtimeSOA + `office IN PERIOD "mon-fri 09:00-18:00"
 out IN CHANNEL log period=!office
 team IN CHANNEL group members=out
 pager IN POLICY chain=team network=out escalate=30m:out
 parent IN HTTP http://127.0.0.1/ policy=pager
 check IN HTTP http://127.0.0.1/parent policy=pager parent=parent title=parent header.X-Name=out
 window IN MAINT select="name:check" every=sun start=02:00 duration=1h
 `
	z, err := a.ImportZone([]byte(raw), ZoneImportOptions{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err != nil {
		t.Fatal(err)
	}
	p := "zone-" + z.Header.ZoneID + "."
	c := a.RuntimeConfig()
	m := c.FindMonitor(p + "check")
	if m == nil || m.Policy != p+"pager" || len(m.Parents) != 1 || m.Parents[0] != p+"parent" {
		t.Fatalf("monitor dependencies: %#v", m)
	}
	if m.Target != "http://127.0.0.1/parent" || m.Title != "parent" || m.Opts["header.x-name"] != "out" {
		t.Fatalf("arbitrary strings rewritten: %#v", m)
	}
	ch := c.FindChannel(p + "out")
	if ch.Period != "!"+p+"office" {
		t.Fatal(ch.Period)
	}
	if c.FindChannel(p + "team").Members[0] != p+"out" {
		t.Fatal("group edge")
	}
	pol := c.FindPolicy(p + "pager")
	if pol.Chain[0] != p+"team" || pol.Network[0] != p+"out" || pol.Escalate[0].Channels[0] != p+"out" {
		t.Fatal("policy edges")
	}
	if len(c.Maints) != 1 || len(c.Maints[0].Monitors) != 1 || c.Maints[0].Monitors[0] != p+"check" {
		t.Fatal("maintenance escaped zone")
	}
	// Base models must never be mutated by namespacing or repeated reapply.
	for i := 0; i < 3; i++ {
		a.Reapply()
		a.reapply()
	}
	if len(a.applied.Load().Monitors) != 2 || len(a.Config().Config.Monitors) != 0 {
		t.Fatal("composition duplicated or mutated base")
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, "stale"); err != ErrConflict {
		t.Fatal(err)
	}
}

func TestZonePreviewExportAndDraftIsolation(t *testing.T) {
	a := runtimeApp(t)
	raw := []byte(runtimeSOA + "out IN CHANNEL log\ncheck IN HTTP http://127.0.0.1/\n")
	preview, err := a.PreviewZone(raw, ZoneImportOptions{Name: "first"})
	if err != nil || len(preview.Missing) != 0 {
		t.Fatalf("preview: %#v %v", preview, err)
	}
	zs, _ := a.ListZones()
	if len(zs) != 0 {
		t.Fatal("preview persisted a zone")
	}
	z, err := a.ImportZone(raw, ZoneImportOptions{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err != nil {
		t.Fatal(err)
	}
	exported, err := a.ExportZone(z.Header.ZoneID)
	if err != nil || !strings.Contains(exported.Text, "$ZONE") {
		t.Fatalf("export: %v", err)
	}
	raw = []byte(strings.Replace(string(raw), "( 1 ", "( 2 ", 1) + "second IN HTTP http://127.0.0.1/\n")
	updated, err := a.ImportZone(raw, ZoneImportOptions{Name: "first", TargetID: z.Header.ZoneID, ExpectedRevision: z.Revision()})
	if err != nil {
		t.Fatal(err)
	}
	zs, _ = a.ListZones()
	if zs[0].AppliedRevision != z.Revision() || zs[0].Record.Revision() != updated.Revision() {
		t.Fatal("draft overwrote applied snapshot")
	}
	if len(a.applied.Load().Monitors) != 1 {
		t.Fatal("unapplied draft executed")
	}
	_, _, _, err = config.ParseZone("export", []byte(exported.Text), config.Options{})
	if err != nil {
		t.Fatal(err)
	}
}
