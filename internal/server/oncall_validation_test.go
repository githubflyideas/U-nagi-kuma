package server

import (
	"testing"
	"time"
)

func TestOnCallOverrideRejectsEmptyStoredInterval(t *testing.T) {
	e := newEnv(t, oncallBind, nil)
	e.setup()
	start := time.Now().Add(time.Hour).Truncate(time.Second)
	code := e.do("POST", "/api/v1/oncall/overrides", map[string]any{
		"rotation": "oncall", "member": "li", "from": start.Add(100 * time.Millisecond), "until": start.Add(900 * time.Millisecond),
	}, nil)
	if code != 400 {
		t.Errorf("interval truncated to zero seconds accepted: %d", code)
	}
	overrides, err := e.app.Store().Overrides(start.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Errorf("invalid interval persisted: %+v", overrides)
	}
}
