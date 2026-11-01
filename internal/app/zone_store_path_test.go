package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/config"
)

func zonePathSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatalf("create regression symlink: %v", err)
	}
}

func TestZoneStoreAllowsAncestorAlias(t *testing.T) {
	for _, suffix := range []string{"", "new-data"} {
		t.Run("data-"+suffix, func(t *testing.T) {
			root := t.TempDir()
			real := filepath.Join(root, "real")
			if err := os.Mkdir(real, 0700); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(root, "alias")
			zonePathSymlink(t, real, alias)
			s, err := OpenZoneStore(filepath.Join(alias, suffix), "node", config.Options{})
			if err != nil {
				t.Fatalf("trusted ancestor alias rejected: %v", err)
			}
			other, err := OpenZoneStore(filepath.Join(real, suffix), "node", config.Options{})
			if err != nil {
				t.Fatal(err)
			}
			want, err := filepath.EvalSymlinks(filepath.Join(real, suffix, "zones"))
			if err != nil {
				t.Fatal(err)
			}
			if s.dir != want || other.dir != want || s.mu != other.mu {
				t.Fatal("aliases must use one canonical directory and writer lock")
			}
			z, err := s.Import([]byte(zoneTestBody), ZoneImportOptions{Name: "office"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := other.Get(z.Header.ZoneID)
			if err != nil || got.Revision() != z.Revision() {
				t.Fatalf("canonical reopen: %v", err)
			}
		})
	}
}

func TestZoneStoreRejectsZonesDirectoryAlias(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		name := "existing"
		if dangling {
			name = "dangling"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			data := filepath.Join(root, "data")
			outside := filepath.Join(root, "outside")
			if err := os.Mkdir(data, 0700); err != nil {
				t.Fatal(err)
			}
			if !dangling {
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
			}
			zonePathSymlink(t, outside, filepath.Join(data, "zones"))
			if _, err := OpenZoneStore(data, "node", config.Options{}); err == nil {
				t.Fatal("zones directory symlink accepted")
			}
			if dangling {
				if _, err := os.Lstat(outside); !os.IsNotExist(err) {
					t.Fatalf("dangling target created: %v", err)
				}
			} else if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("outside directory changed: %v", err)
			}
		})
	}
}

func TestZoneStoreRejectsExternalRecordAlias(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenZoneStore(dir, "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	z, err := s.Import([]byte(zoneTestBody), ZoneImportOptions{Name: "office"})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.dir, z.Header.ZoneID+".json")
	outside := filepath.Join(t.TempDir(), "record.json")
	if err := os.Rename(p, outside); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	zonePathSymlink(t, outside, p)
	if _, err := s.Get(z.Header.ZoneID); err == nil {
		t.Fatal("external symlink record read accepted")
	}
	if err := s.persist(z); err == nil {
		t.Fatal("external symlink record write accepted")
	}
	after, err := os.ReadFile(outside)
	if err != nil || string(after) != string(before) {
		t.Fatalf("external record changed: %v", err)
	}
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("rejected write replaced record symlink: %v", err)
	}
}
