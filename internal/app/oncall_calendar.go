package app

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/githubflyideas/unagikuma/internal/oncall"
)

// CalendarSegment is a stretch of time with one person on call: the
// scheduled member, or a stand-in covering them.
type CalendarSegment struct {
	Member    string    `json:"member"`             // who is notified first
	Scheduled string    `json:"scheduled"`          // whose shift it is
	StandIn   int64     `json:"stand_in,omitempty"` // the stand-in id, when one covers it
	Note      string    `json:"note,omitempty"`     // the stand-in's note
	From      time.Time `json:"from"`
	Until     time.Time `json:"until"`
}

// CalendarRotation is one rotation's calendar.
type CalendarRotation struct {
	Name     string            `json:"name"`
	TZ       string            `json:"tz"`
	Members  []OnCallMember    `json:"members"`
	Segments []CalendarSegment `json:"segments"`
}

// maxCalendar bounds a calendar request.
const maxCalendar = 92 * 24 * time.Hour

// OnCallCalendar lays out who is on call in [from, to) for every rotation,
// stand-ins applied, cut where a shift or a stand-in begins or ends.
func (a *App) OnCallCalendar(from, to time.Time) ([]CalendarRotation, error) {
	if !to.After(from) || to.Sub(from) > maxCalendar {
		return nil, fmt.Errorf("%w: the calendar covers up to 92 days", ErrOnCall)
	}
	cfg := a.Config().Config
	os, err := a.store.Overrides(from)
	if err != nil {
		return nil, err
	}
	dir := a.Directory()
	out := []CalendarRotation{}
	for name, r := range rotations(cfg) {
		cr := CalendarRotation{Name: name, TZ: rotationZone(r, a.opts.Location), Segments: []CalendarSegment{}}
		for _, m := range r.Members {
			cr.Members = append(cr.Members, memberOf(cfg, dir, m))
		}
		cr.Segments = calendar(r, name, os, from, to, a.opts.Location)
		out = append(out, cr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// rotationZone names the time zone a rotation hands off in.
func rotationZone(r *oncall.Rotation, loc *time.Location) string {
	if r.TZ != "" {
		return r.TZ
	}
	if loc == nil {
		loc = time.Local
	}
	return loc.String()
}

func calendar(r *oncall.Rotation, name string, os []oncall.Override, from, to time.Time, loc *time.Location) []CalendarSegment {
	cuts := []time.Time{from, to}
	for _, s := range r.Next(from, 4000, loc) {
		if !s.From.Before(to) {
			break
		}
		if s.From.After(from) {
			cuts = append(cuts, s.From)
		}
	}
	for _, o := range os {
		if o.Rotation != name {
			continue
		}
		for _, t := range []time.Time{o.From, o.Until} {
			if t.After(from) && t.Before(to) {
				cuts = append(cuts, t)
			}
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].Before(cuts[j]) })
	cuts = slices.CompactFunc(cuts, func(x, y time.Time) bool { return x.Equal(y) })
	var segs []CalendarSegment
	for i := 0; i+1 < len(cuts); i++ {
		at := cuts[i]
		sg := CalendarSegment{Scheduled: r.At(at, loc).Member, From: at, Until: cuts[i+1]}
		sg.Member = sg.Scheduled
		if o := oncall.Active(name, os, at); o != nil {
			sg.Member, sg.StandIn, sg.Note = o.Member, o.ID, o.Note
		}
		if n := len(segs); n > 0 && segs[n-1].Member == sg.Member && segs[n-1].Scheduled == sg.Scheduled && segs[n-1].StandIn == sg.StandIn {
			segs[n-1].Until = sg.Until
			continue
		}
		segs = append(segs, sg)
	}
	if segs == nil {
		segs = []CalendarSegment{}
	}
	return segs
}

// SwapShifts swaps two shifts of a rotation: the member of the shift
// running at b covers the shift running at a, and the other way round, as
// a pair of stand-ins. The order of the rotation does not change.
func (a *App) SwapShifts(rotation string, atA, atB time.Time, by, note string) ([]oncall.Override, error) {
	r := rotations(a.Config().Config)[rotation]
	if r == nil {
		return nil, fmt.Errorf("%w: %s is not an on-call rotation", ErrOnCall, rotation)
	}
	loc, now := a.opts.Location, a.now()
	sa, sb := r.At(atA, loc), r.At(atB, loc)
	switch {
	case sa.Member == sb.Member:
		return nil, fmt.Errorf("%w: both shifts are %s's", ErrOnCall, sa.Member)
	case !sa.Until.After(now) || !sb.Until.After(now):
		return nil, fmt.Errorf("%w: a shift that is over cannot be swapped", ErrOnCall)
	}
	var out []oncall.Override
	for _, o := range []oncall.Override{
		{Rotation: rotation, Member: sb.Member, From: sa.From, Until: sa.Until, By: by, Note: note, CreatedAt: now},
		{Rotation: rotation, Member: sa.Member, From: sb.From, Until: sb.Until, By: by, Note: note, CreatedAt: now},
	} {
		saved, err := a.store.AddOverride(o)
		if err != nil {
			return out, err
		}
		out = append(out, saved)
	}
	a.reapply()
	a.emit("oncall", nil)
	return out, nil
}
