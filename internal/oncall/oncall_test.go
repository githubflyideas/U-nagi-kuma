package oncall

import (
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, opts map[string]string, members ...string) *Rotation {
	t.Helper()
	r, err := Parse(opts, members)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestParse(t *testing.T) {
	if r, err := Parse(map[string]string{}, []string{"a"}); r != nil || err != nil {
		t.Fatalf("no rotate=: %v %v", r, err)
	}
	for _, c := range []struct {
		opts map[string]string
		err  string
	}{
		{map[string]string{"handoff": "09:00"}, "needs rotate="},
		{map[string]string{"rotate": "monthly"}, "rotate must be"},
		{map[string]string{"rotate": "0d"}, "rotate must be"},
		{map[string]string{"rotate": "91d"}, "rotate must be"},
		{map[string]string{"rotate": "daily", "handoff": "mon 09:00"}, "whole weeks"},
		{map[string]string{"rotate": "weekly", "handoff": "xyz 09:00"}, "unknown day"},
		{map[string]string{"rotate": "weekly", "handoff": "mon 9am"}, "HH:MM"},
		{map[string]string{"rotate": "weekly", "start": "next week"}, "start must be"},
	} {
		if _, err := Parse(c.opts, []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%v: %v, want %q", c.opts, err, c.err)
		}
	}
	r := mustParse(t, map[string]string{"rotate": "weekly"}, "a")
	if r.Handoff != "mon 09:00" || r.Weekday != time.Monday || r.Clock != 9*60 {
		t.Fatalf("weekly defaults: %+v", r)
	}
	if r := mustParse(t, map[string]string{"rotate": "12h"}, "a"); r.Hours != 12 || r.Handoff != "09:00" {
		t.Fatalf("hours: %+v", r)
	}
}

func TestWeekly(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	r := mustParse(t, map[string]string{"rotate": "weekly", "handoff": "mon 10:00", "start": "2026-11-02"}, "li", "wang", "zhao")
	at := func(s string) time.Time { v, _ := time.ParseInLocation("2006-01-02 15:04", s, loc); return v }
	for _, c := range []struct{ t, who, from string }{
		{"2026-11-02 10:00", "li", "2026-11-02 10:00"},
		{"2026-11-09 09:59", "li", "2026-11-02 10:00"},
		{"2026-11-09 10:00", "wang", "2026-11-09 10:00"},
		{"2026-11-20 03:00", "zhao", "2026-11-16 10:00"},
		{"2026-11-23 10:00", "li", "2026-11-23 10:00"},
		{"2026-11-01 12:00", "zhao", "2026-10-26 10:00"}, // before the start: backwards
	} {
		s := r.At(at(c.t), loc)
		if s.Member != c.who || !s.From.Equal(at(c.from)) || !s.Until.Equal(at(c.from).AddDate(0, 0, 7)) {
			t.Errorf("%s: %+v, want %s from %s", c.t, s, c.who, c.from)
		}
	}
	// A start that is not a handoff day: the first member takes the shift
	// running then.
	r2 := mustParse(t, map[string]string{"rotate": "weekly", "handoff": "mon 10:00", "start": "2026-11-04"}, "li", "wang")
	if s := r2.At(at("2026-11-04 12:00"), loc); s.Member != "li" || !s.From.Equal(at("2026-11-02 10:00")) {
		t.Fatalf("start mid-week: %+v", s)
	}
	if s := r2.At(at("2026-11-10 12:00"), loc); s.Member != "wang" {
		t.Fatalf("the week after: %+v", s)
	}
	next := r.Next(at("2026-11-10 12:00"), 4, loc)
	if len(next) != 4 || next[0].Member != "wang" || next[1].Member != "zhao" || next[3].Member != "wang" || !next[1].From.Equal(next[0].Until) {
		t.Fatalf("next: %+v", next)
	}
}

// Day shifts keep their local handoff time across a daylight saving change.
func TestDailyAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no time zone data")
	}
	r := mustParse(t, map[string]string{"rotate": "daily", "handoff": "08:00", "start": "2026-10-20"}, "a", "b")
	s := r.At(time.Date(2026, 10, 26, 12, 0, 0, 0, loc), loc) // the day after the change
	if s.From.Hour() != 8 || s.Until.Hour() != 8 || s.Member != "a" {
		t.Fatalf("across DST: %+v", s)
	}
}

func TestStandInsAndOrder(t *testing.T) {
	loc := time.UTC
	r := mustParse(t, map[string]string{"rotate": "daily", "handoff": "00:00", "start": "2026-11-01"}, "a", "b", "c")
	t0 := time.Date(2026, 11, 2, 12, 0, 0, 0, loc) // b's day
	if got := strings.Join(r.Order("ops", nil, t0, loc), ","); got != "b,c,a" {
		t.Fatalf("order: %s", got)
	}
	ovs := []Override{
		{ID: 1, Rotation: "ops", Member: "c", From: t0.Add(-time.Hour), Until: t0.Add(time.Hour)},
		{ID: 2, Rotation: "ops", Member: "a", From: t0.Add(-time.Minute), Until: t0.Add(time.Minute)},
		{ID: 3, Rotation: "other", Member: "b", From: t0.Add(-time.Hour), Until: t0.Add(time.Hour)},
	}
	if o := Active("ops", ovs, t0); o == nil || o.ID != 2 {
		t.Fatalf("newest stand-in wins: %+v", o)
	}
	if got := strings.Join(r.Order("ops", ovs, t0, loc), ","); got != "a,b,c" {
		t.Fatalf("order with a stand-in: %s", got)
	}
	if o := Active("ops", ovs, t0.Add(time.Hour)); o != nil {
		t.Fatalf("the end is exclusive: %+v", o)
	}
}
