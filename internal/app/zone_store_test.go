package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/config"
)

const zoneTestBody = "@ IN SOA node. admin.localhost. ( 1 1m 15s 5s 3 )\nweb IN HTTP https://example.com\n"

func TestZoneUpdateCASAndBinding(t *testing.T) {
	s, err := OpenZoneStore(t.TempDir(), "this-node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	z, err := s.Import([]byte(zoneTestBody), ZoneImportOptions{Name: "office"})
	if err != nil {
		t.Fatal(err)
	}
	next := []byte(strings.Replace(zoneTestBody, "( 1 ", "( 2 ", 1))
	for _, raw := range [][]byte{[]byte("bad"), next} {
		_, err = s.Import(raw, ZoneImportOptions{Name: "office", TargetID: z.Header.ZoneID, ExpectedRevision: "stale"})
		if err == nil {
			t.Fatal("invalid update accepted")
		}
		cur, _ := s.Get(z.Header.ZoneID)
		if cur.Revision() != z.Revision() {
			t.Fatal("invalid import changed disk")
		}
	}
	z2, err := s.Import(next, ZoneImportOptions{Name: "office", TargetID: z.Header.ZoneID, ExpectedRevision: z.Revision()})
	if err != nil {
		t.Fatal(err)
	}
	if z2.Header.Serial != 2 || z2.Header.ZoneID != z.Header.ZoneID {
		t.Fatal("update lost identity")
	}
	primary, err := s.Rebind(z2.Header.ZoneID, z2.Revision(), ZoneBinding{SourceNode: "this-node"})
	if err != nil || primary.Role("this-node") != "primary" || primary.Header.Generation != 2 {
		t.Fatalf("primary %v %v", primary, err)
	}
	if _, err = s.Rebind(primary.Header.ZoneID, primary.Revision(), ZoneBinding{SourceNode: "remote"}); err == nil {
		t.Fatal("missing fingerprint accepted")
	}
	secondary, err := s.Rebind(primary.Header.ZoneID, primary.Revision(), ZoneBinding{SourceNode: "remote", Fingerprint: strings.Repeat("a", 64)})
	if err != nil || secondary.Role("this-node") != "secondary" {
		t.Fatalf("secondary %v", err)
	}
	if _, err = s.Import(next, ZoneImportOptions{Name: "office", TargetID: z.Header.ZoneID, ExpectedRevision: secondary.Revision()}); !errors.Is(err, ErrReadonly) {
		t.Fatalf("secondary writable: %v", err)
	}
	if _, err = s.Rebind(z.Header.ZoneID, primary.Revision(), ZoneBinding{}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale rebind accepted")
	}
	local, err := s.Rebind(z.Header.ZoneID, secondary.Revision(), ZoneBinding{})
	if err != nil || local.Role("this-node") != "local" || local.Binding.Fingerprint != "" {
		t.Fatal("local incoherent")
	}
}

func TestZoneStoreRejectsCorruptBinding(t *testing.T) {
	for _, binding := range []ZoneBinding{{SourceNode: "remote"}, {SourceNode: "remote", Fingerprint: "bad"}, {Fingerprint: strings.Repeat("a", 64)}, {SourceNode: "bad/name", Fingerprint: strings.Repeat("a", 64)}} {
		dir := t.TempDir()
		s, err := OpenZoneStore(dir, "local", config.Options{})
		if err != nil {
			t.Fatal(err)
		}
		z, err := s.Import([]byte(zoneTestBody), ZoneImportOptions{Name: "office"})
		if err != nil {
			t.Fatal(err)
		}
		z.Binding = binding
		z.Header.SourceNode = binding.SourceNode
		b, err := json.Marshal(z)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "zones", z.Header.ZoneID+".json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Get(z.Header.ZoneID); err == nil {
			t.Fatalf("corrupt binding accepted: %+v", binding)
		}
	}
}

func TestZoneStoreConcurrentCAS(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenZoneStore(dir, "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := OpenZoneStore(dir, "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	z, err := s.Import([]byte(zoneTestBody), ZoneImportOptions{Name: "office"})
	if err != nil {
		t.Fatal(err)
	}
	next := []byte(strings.Replace(zoneTestBody, "( 1 ", "( 2 ", 1))
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, store := range []*ZoneStore{s, other} {
		wg.Add(1)
		go func(store *ZoneStore) {
			defer wg.Done()
			_, err := store.Import(next, ZoneImportOptions{Name: "office", TargetID: z.Header.ZoneID, ExpectedRevision: z.Revision()})
			results <- err
		}(store)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("CAS results: %d success, %d conflicts", successes, conflicts)
	}
	got, err := other.Get(z.Header.ZoneID)
	if err != nil || got.Header.Serial != 2 {
		t.Fatalf("persisted update: %v", err)
	}
}

func TestZoneImportIsolationAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenZoneStore(dir, "this-node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(strings.Replace(zoneTestBody, "\n", "\n$ZONE zone_id=12345678-1234-4234-8234-123456789abc name=office generation=9 source_node=stranger\n", 1))
	z, err := s.Import(raw, ZoneImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if z.Header.ZoneID == "12345678-1234-4234-8234-123456789abc" || z.Role("this-node") != "local" || z.Header.SourceNode != "" || z.Header.Generation != 1 || z.Origin == nil {
		t.Fatalf("trusted import: %+v", z)
	}
	if _, err = s.Import(raw, ZoneImportOptions{}); !errors.Is(err, ErrZoneNameConflict) {
		t.Fatalf("name collision: %v", err)
	}
	reopened, err := OpenZoneStore(dir, "this-node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(z.Header.ZoneID)
	if err != nil || got.Revision() != z.Revision() || got.Body != zoneTestBody {
		t.Fatalf("restart: %+v %v", got, err)
	}
	if _, err = s.Get("../monitors"); err == nil {
		t.Fatal("accepted traversal")
	}
	if _, err = s.Import([]byte(strings.Repeat("x", MaxZoneBytes+1)), ZoneImportOptions{Name: "huge"}); err == nil {
		t.Fatal("accepted oversized")
	}
	if _, err = os.Stat(filepath.Join(dir, "monitors.bind")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legacy file touched")
	}
}
