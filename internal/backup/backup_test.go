package backup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T) (string, DB) { return setupFuzz(t) }

func setupFuzz(t testing.TB) (string, DB) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"monitors.bind": "@ IN SOA m. a.b. ( 1 1m 15s 5s 3 )\n", "unagikuma.key": "KEY\n",
		"cluster-ca.crt": "CA\n", "cluster-ca.key": "CAKEY\n", "cluster.crt": "LEAF\n", "cluster.key": "LEAFKEY\n",
	} {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
	}
	db := func(_ context.Context, which string, w io.Writer) error {
		// Large enough to span several encrypted chunks.
		_, err := io.WriteString(w, which+strings.Repeat("x", 3*chunkSize))
		return err
	}
	return dir, db
}

func write(t *testing.T, dir string, db DB, o Options) []byte { return writeFuzz(t, dir, db, o) }

func writeFuzz(t testing.TB, dir string, db DB, o Options) []byte {
	t.Helper()
	o.DataDir, o.ConfigPath, o.Now = dir, filepath.Join(dir, "monitors.bind"), time.Now()
	var buf bytes.Buffer
	if _, err := Write(context.Background(), &buf, db, o); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPlainBackupLeavesKeysOut(t *testing.T) {
	dir, db := setup(t)
	arch := write(t, dir, db, Options{History: true})
	dst := t.TempDir()
	m, err := Restore(bytes.NewReader(arch), dst, filepath.Join(dst, "monitors.bind"), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if m.Keys || m.Encrypted || strings.Join(m.Files, ",") != "core.db,history.db,monitors.bind" {
		t.Fatalf("manifest: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(dst, "unagikuma.key")); err == nil {
		t.Fatal("a plain backup must not contain the key")
	}
	b, _ := os.ReadFile(filepath.Join(dst, "history.db"))
	if !strings.HasPrefix(string(b), "history") || len(b) != len("history")+3*chunkSize {
		t.Fatal("history.db content")
	}
}

func TestEncryptedBackupRoundTrip(t *testing.T) {
	dir, db := setup(t)
	arch := write(t, dir, db, Options{Passphrase: "correct horse"})
	if bytes.Contains(arch, []byte("CAKEY")) {
		t.Fatal("key material visible in an encrypted archive")
	}
	dst := t.TempDir()
	if _, err := Restore(bytes.NewReader(arch), dst, filepath.Join(dst, "monitors.bind"), "", false); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("no passphrase: %v", err)
	}
	if _, err := Restore(bytes.NewReader(arch), dst, filepath.Join(dst, "monitors.bind"), "wrong", false); err == nil || !strings.Contains(err.Error(), "wrong passphrase") {
		t.Fatalf("wrong passphrase: %v", err)
	}
	m, err := Restore(bytes.NewReader(arch), dst, filepath.Join(dst, "monitors.bind"), "correct horse", false)
	if err != nil || !m.Keys {
		t.Fatalf("%v %+v", err, m)
	}
	for _, f := range keyFiles {
		got, _ := os.ReadFile(filepath.Join(dst, f))
		want, _ := os.ReadFile(filepath.Join(dir, f))
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs", f)
		}
		if st, _ := os.Stat(filepath.Join(dst, f)); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %v", f, st.Mode())
		}
	}
	// An existing installation is not replaced without force.
	if _, err := Restore(bytes.NewReader(arch), dst, filepath.Join(dst, "monitors.bind"), "correct horse", false); err == nil {
		t.Fatal("overwrote without force")
	}
	if _, err := Restore(bytes.NewReader(arch), dst, filepath.Join(dst, "monitors.bind"), "correct horse", true); err != nil {
		t.Fatal(err)
	}
}

func TestTamperedBackupsAreRejected(t *testing.T) {
	dir, db := setup(t)
	arch := write(t, dir, db, Options{Passphrase: "p"})
	flip := append([]byte(nil), arch...)
	flip[len(flip)/2] ^= 1
	truncated := arch[:len(arch)-len(arch)/3]
	// Dropping the last chunk must not look like a complete archive.
	for name, b := range map[string][]byte{"flipped": flip, "truncated": truncated} {
		dst := t.TempDir()
		if _, err := Restore(bytes.NewReader(b), dst, filepath.Join(dst, "monitors.bind"), "p", false); err == nil {
			t.Errorf("%s archive accepted", name)
		}
		if _, err := os.Stat(filepath.Join(dst, "core.db")); err == nil {
			t.Errorf("%s archive left files behind", name)
		}
	}
	dst := t.TempDir()
	if _, err := Restore(strings.NewReader("not a backup"), dst, filepath.Join(dst, "monitors.bind"), "", false); err == nil {
		t.Error("garbage accepted")
	}
}

// Encrypted archives written before v1.6 carry an older header and must
// still restore.
func TestLegacyEncryptedHeaderRestores(t *testing.T) {
	dir, db := setup(t)
	arch := write(t, dir, db, Options{Passphrase: "p"})
	if !bytes.HasPrefix(arch, []byte(magic)) {
		t.Fatalf("new header: %q", arch[:len(magic)])
	}
	old := append(append([]byte(nil), legacyMagic...), arch[len(magic):]...)
	dst := t.TempDir()
	if _, err := Restore(bytes.NewReader(old), dst, filepath.Join(dst, "monitors.bind"), "p", false); err != nil {
		t.Fatalf("legacy archive: %v", err)
	}
}
