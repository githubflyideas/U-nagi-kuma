package app

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestZoneAuthenticatedSync(t *testing.T) {
	source, target := runtimeApp(t), runtimeApp(t)
	z, err := source.ImportZone([]byte(runtimeSOA+"check IN HTTP http://127.0.0.1/\n"), ZoneImportOptions{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	z, err = source.RebindZone(z.Header.ZoneID, z.Revision(), ZoneBinding{SourceNode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	z, err = source.ApplyZone(z.Header.ZoneID, z.Revision())
	if err != nil {
		t.Fatal(err)
	}
	token, err := source.GrantZoneSync(z.Header.ZoneID, "secondary")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(source.ZoneSyncHandler())
	srv.TLS = source.PKI().TLSConfig(nil)
	srv.StartTLS()
	defer srv.Close()
	// Use distinct local identity; importing provenance alone never binds trust.
	target.zones.local = "secondary"
	dst, err := target.ImportZone([]byte(runtimeSOA), ZoneImportOptions{Name: "copy"})
	if err != nil {
		t.Fatal(err)
	}
	dst, err = target.RebindZone(dst.Header.ZoneID, dst.Revision(), ZoneBinding{SourceNode: "local", Fingerprint: strings.TrimPrefix(source.PKI().Fingerprint(), "sha256:"), SourceZoneID: z.Header.ZoneID, SourceGeneration: z.Header.Generation})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = target.SyncZone(context.Background(), dst.Header.ZoneID, srv.URL, "bad"); err == nil {
		t.Fatal("unauthorized sync")
	}
	got, err := target.SyncZone(context.Background(), dst.Header.ZoneID, srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	if got.Header.ContentHash != z.Header.ContentHash || len(target.RuntimeConfig().Monitors) != 1 {
		t.Fatal("authenticated snapshot not applied")
	}
	if _, err = target.SyncZone(context.Background(), dst.Header.ZoneID, srv.URL, token); err != nil {
		t.Fatal("idempotent sync:", err)
	}
	// A newer source draft is not published until its exact snapshot is applied.
	draftBody := strings.Replace(z.Body, "( 1 ", "( 2 ", 1) + "second IN HTTP http://127.0.0.1/\n"
	draft, err := source.ImportZone([]byte(draftBody), ZoneImportOptions{Name: "shared", TargetID: z.Header.ZoneID, ExpectedRevision: z.Revision()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = target.SyncZone(context.Background(), dst.Header.ZoneID, srv.URL, token); err != nil {
		t.Fatal(err)
	}
	if len(target.RuntimeConfig().Monitors) != 1 {
		t.Fatal("source draft leaked before apply")
	}
	if _, err = source.ApplyZone(draft.Header.ZoneID, draft.Revision()); err != nil {
		t.Fatal(err)
	}
	updated, err := target.SyncZone(context.Background(), dst.Header.ZoneID, srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Synced.Serial != 2 || len(target.RuntimeConfig().Monitors) != 2 {
		t.Fatal("new applied serial not synchronized")
	}
	if err = source.RevokeZoneSync(z.Header.ZoneID, "secondary"); err != nil {
		t.Fatal(err)
	}
	if _, err = target.SyncZone(context.Background(), dst.Header.ZoneID, srv.URL, token); err == nil {
		t.Fatal("revoked token accepted")
	}
	if len(target.RuntimeConfig().Monitors) != 2 {
		t.Fatal("last good lost on denial")
	}
}
