package server

import "testing"

func TestMonitorBulkDelete(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	cfg := func() string { return e.app.Config().ETag() }
	if c := e.do("POST", "/api/v1/monitors/bulk-delete", map[string]any{"names": []string{}}, nil); c != 400 {
		t.Fatalf("empty: %d", c)
	}
	if c := e.do("POST", "/api/v1/monitors/bulk-delete", map[string]any{"names": []string{"web", "nope"}}, nil); c != 404 || e.app.Config().Config.FindMonitor("web") == nil {
		t.Fatalf("unknown name: %d", c)
	}
	// A stale etag fails the whole request: nothing is deleted.
	before := cfg()
	if c := e.do("POST", "/api/v1/monitors/bulk-delete", map[string]any{"names": []string{"web", "db"}, "etags": map[string]string{"db": `"stale"`}}, nil); c != 412 {
		t.Fatalf("stale etag: %d", c)
	}
	if cfg() != before || e.app.Config().Config.FindMonitor("web") == nil {
		t.Fatal("a refused request changed the file")
	}
	serial := e.app.Config().Serial
	var st struct{ Serial uint32 }
	if c := e.do("POST", "/api/v1/monitors/bulk-delete", map[string]any{"names": []string{"web", "db"}}, &st); c != 200 {
		t.Fatalf("delete: %d", c)
	}
	c := e.app.Config().Config
	if c.FindMonitor("web") != nil || c.FindMonitor("db") != nil || c.FindMonitor("backup") == nil {
		t.Fatal("wrong monitors deleted")
	}
	if e.app.Config().Serial != st.Serial || st.Serial <= serial {
		t.Fatalf("serial %d → %d (response %d)", serial, e.app.Config().Serial, st.Serial)
	}
}
