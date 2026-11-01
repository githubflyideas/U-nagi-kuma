package app

import "testing"

func TestZoneNodesRejectNonlocalParent(t *testing.T) {
	a := runtimeApp(t)
	raw := runtimeSOA + "remote IN NODE\nparent IN HTTP http://127.0.0.1/ nodes=remote\nchild IN HTTP http://127.0.0.1/ parent=parent\n"
	z, err := a.ImportZone([]byte(raw), ZoneImportOptions{Name: "parents"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err == nil {
		t.Fatal("missing local parent silently accepted")
	}
}

func TestZoneNodesExecuteOnlyAuthorizedLocalAssignments(t *testing.T) {
	a := runtimeApp(t)
	raw := runtimeSOA + "remote IN NODE\nlocal IN NODE\nlegacy IN HTTP http://127.0.0.1/\nhere IN HTTP http://127.0.0.1/ nodes=local\nthere IN HTTP http://127.0.0.1/ nodes=remote\nshared IN HTTP http://127.0.0.1/ nodes=local,remote\n"
	z, err := a.ImportZone([]byte(raw), ZoneImportOptions{Name: "assignments"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err != nil {
		t.Fatal(err)
	}
	_, ec, err := a.composeRuntime(a.baseApplied, a.zoneApplied)
	if err != nil {
		t.Fatal(err)
	}
	if len(ec.Monitors) != 3 {
		t.Fatalf("want legacy, here, shared local monitors; got %d", len(ec.Monitors))
	}
	for _, m := range ec.Monitors {
		if len(m.Nodes) != 0 {
			t.Fatalf("zone routed via central agents: %+v", m)
		}
	}
	if len(ec.Nodes) != 0 {
		t.Fatal("zone NODE registry leaked to legacy agent engine")
	}
}
