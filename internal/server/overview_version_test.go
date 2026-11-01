package server

import "testing"

func TestOverviewIncludesApplicationVersion(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	var overview map[string]any
	if code := e.do("GET", "/api/v1/overview", nil, &overview); code != 200 {
		t.Fatalf("overview status: %d", code)
	}
	version, exists := overview["version"]
	if !exists || version != e.app.Version() {
		t.Fatalf("overview version = %#v (present %v), want application version %q", version, exists, e.app.Version())
	}
}
