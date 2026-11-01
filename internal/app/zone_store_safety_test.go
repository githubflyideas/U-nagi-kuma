package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/config"
)

func TestZoneStoreRejectedWritesPreserveRecord(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenZoneStore(dir, "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	z, err := s.Import([]byte(zoneTestBody), ZoneImportOptions{Name: "office"})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{zoneTestBody, "invalid", strings.Repeat("x", MaxZoneBytes+1)} {
		if _, err = s.Import([]byte(body), ZoneImportOptions{Name: "office", TargetID: z.Header.ZoneID, ExpectedRevision: z.Revision()}); err == nil {
			t.Fatal("invalid update accepted")
		}
		got, err := s.Get(z.Header.ZoneID)
		if err != nil || got.Revision() != z.Revision() {
			t.Fatal("rejected write changed record")
		}
	}
	z.Header.Generation = math.MaxUint64
	raw, err := json.Marshal(z)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "zones", z.Header.ZoneID+".json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Rebind(z.Header.ZoneID, z.Revision(), ZoneBinding{SourceNode: "node"}); err == nil {
		t.Fatal("generation wrapped")
	}
	got, err := s.Get(z.Header.ZoneID)
	if err != nil || got.Revision() != z.Revision() {
		t.Fatal("generation rejection changed record")
	}
	z.Body += "; changed\n"
	raw, err = json.Marshal(z)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(z.Header.ZoneID); err == nil {
		t.Fatal("body hash corruption accepted")
	}
}

func TestZoneStoreRejectsNonRegularRecord(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenZoneStore(dir, "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	id := "12345678-1234-4234-8234-123456789abc"
	p := filepath.Join(dir, "zones", id+".json")
	if err = os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(id); err == nil {
		t.Fatal("directory record accepted")
	}
}

func TestZoneStoreRejectsSymlinkRecord(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenZoneStore(dir, "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	z, err := s.Import([]byte(zoneTestBody), ZoneImportOptions{Name: "office"})
	if err != nil {
		t.Fatal(err)
	}
	id := "12345678-1234-4234-8234-123456789abc"
	if err = os.Symlink(filepath.Join(dir, "zones", z.Header.ZoneID+".json"), filepath.Join(dir, "zones", id+".json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err = s.Get(id); err == nil {
		t.Fatal("symlink record accepted")
	}
	got, err := s.Get(z.Header.ZoneID)
	if err != nil || got.Revision() != z.Revision() {
		t.Fatal("symlink rejection changed target")
	}
}
