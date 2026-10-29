package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/oncall"
)

// A rotation notifies whoever is on call (a stand-in first), and the
// others in turn only when that fails.
func TestRotationDelivery(t *testing.T) {
	var sent []string
	fail := map[string]bool{}
	e := New(Deps{Send: func(_ context.Context, ch notify.Channel, _ *notify.Message) notify.Attempt {
		sent = append(sent, ch.Name)
		if fail[ch.Name] {
			return notify.Attempt{Error: "unreachable"}
		}
		return notify.Attempt{OK: true}
	}})
	rot, err := oncall.Parse(map[string]string{"rotate": "daily", "handoff": "00:00", "start": "2026-11-01"}, []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	log := func(name string) ChannelSpec { return ChannelSpec{Channel: notify.Channel{Name: name, Kind: "log"}} }
	ops := log("ops")
	ops.Kind, ops.Members, ops.Rotation = "group", []string{"a", "b", "c"}, rot
	e.cfg = Config{Location: time.UTC, Channels: map[string]ChannelSpec{"a": log("a"), "b": log("b"), "c": log("c"), "ops": ops}}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) // b's day
	send := func() (notify.Attempt, string) {
		sent = nil
		att, _ := e.sendRotation(context.Background(), e.groupMembers(ops, now), &notify.Message{Kind: "PROBLEM"})
		return att, strings.Join(sent, ",")
	}
	if att, got := send(); !att.OK || got != "b" {
		t.Fatalf("on call: %v %q", att, got)
	}
	fail["b"] = true
	if att, got := send(); !att.OK || got != "b,c" {
		t.Fatalf("fallback: %v %q", att, got)
	}
	e.cfg.Overrides = []oncall.Override{{ID: 1, Rotation: "ops", Member: "a", From: now.Add(-time.Hour), Until: now.Add(time.Hour)}}
	if att, got := send(); !att.OK || got != "a" {
		t.Fatalf("stand-in: %v %q", att, got)
	}
	fail["a"], fail["c"] = true, true
	if att, got := send(); att.OK || got != "a,b,c" || !strings.Contains(att.Error, "c: unreachable") {
		t.Fatalf("all fail: %v %q", att, got)
	}
}
