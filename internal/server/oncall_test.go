package server

import (
	"strconv"
	"testing"
	"time"
)

const oncallBind = testBind + `li     IN CHANNEL log
wang   IN CHANNEL log
oncall IN CHANNEL group members=li,wang rotate=weekly handoff="mon 09:00"
pager  IN POLICY chain=oncall
`

func TestOnCallAPI(t *testing.T) {
	e := newEnv(t, oncallBind, nil)
	e.setup()
	type rotation struct {
		Name, Now, Rotate, Handoff string
		Members                    []struct{ Channel string }
		Shifts                     []struct {
			Member   string
			StandIns []struct{ Member string } `json:"stand_ins"`
		}
		StandIn  *struct{ ID int64 } `json:"stand_in"`
		Policies []string
	}
	var view struct {
		Rotations []rotation
		Stats     struct {
			Members []struct{ Rotation, Member string }
		}
	}
	if c := e.do("GET", "/api/v1/oncall", nil, &view); c != 200 || len(view.Rotations) != 1 {
		t.Fatalf("view: %d %+v", c, view)
	}
	ro := view.Rotations[0]
	if ro.Name != "oncall" || ro.Rotate != "weekly" || ro.Handoff != "mon 09:00" || len(ro.Members) != 2 || len(ro.Shifts) != 8 ||
		ro.StandIn != nil || len(ro.Policies) != 1 || ro.Policies[0] != "pager" || len(view.Stats.Members) != 2 {
		t.Fatalf("rotation: %+v stats %+v", ro, view.Stats)
	}
	other := "li"
	if ro.Now == "li" {
		other = "wang"
	}
	for _, bad := range []map[string]any{
		{"rotation": "nope", "member": "li", "until": time.Now().Add(time.Hour)},
		{"rotation": "oncall", "member": "boss", "until": time.Now().Add(time.Hour)},
		{"rotation": "oncall", "member": "li", "until": time.Now().Add(-time.Hour)},
	} {
		if c := e.do("POST", "/api/v1/oncall/overrides", bad, nil); c != 400 {
			t.Errorf("%v: %d", bad, c)
		}
	}
	var o struct{ ID int64 }
	if c := e.do("POST", "/api/v1/oncall/overrides", map[string]any{"rotation": "oncall", "member": other, "until": time.Now().Add(2 * time.Hour), "note": "swap"}, &o); c != 200 || o.ID == 0 {
		t.Fatalf("stand-in: %d %+v", c, o)
	}
	e.do("GET", "/api/v1/oncall", nil, &view)
	ro = view.Rotations[0]
	if ro.Now != other || ro.StandIn == nil || ro.StandIn.ID != o.ID || len(ro.Shifts[0].StandIns) != 1 {
		t.Fatalf("after the stand-in: %+v", ro)
	}
	// The engine notifies the stand-in first.
	if got := e.app.Engine().OnCallOrder("oncall", time.Now()); len(got) != 2 || got[0] != other {
		t.Fatalf("engine order: %v", got)
	}
	if c := e.do("DELETE", "/api/v1/oncall/overrides/"+strconv.FormatInt(o.ID, 10), nil, nil); c != 200 {
		t.Fatalf("delete: %d", c)
	}
	if c := e.do("DELETE", "/api/v1/oncall/overrides/"+strconv.FormatInt(o.ID, 10), nil, nil); c != 404 {
		t.Fatalf("delete twice: %d", c)
	}
	e.do("GET", "/api/v1/oncall", nil, &view)
	if view.Rotations[0].Now == other {
		t.Fatal("the stand-in is still on call")
	}
}
