package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/netscope"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/probe"
)

// openSystem opens (or keeps) a system incident; it is notified through the
// "system" policy.
func (e *Engine) openSystem(key string, now time.Time, detail string) *Incident {
	return e.openSystemAs(key, "system", e.text("system_title_"+key), detail, now)
}

// openSystemAs opens a system incident notified through the given policy.
func (e *Engine) openSystemAs(key, policy, title, detail string, now time.Time) *Incident {
	if inc := e.system[key]; inc != nil && inc.State.Open() {
		return inc
	}
	pol := e.policy(policy)
	prev := e.system[key]
	var prevState *IncidentState
	if prev != nil {
		prevState = &prev.State
	}
	inc := &Incident{Kind: IncidentSystem, System: key, Policy: pol.Name, Title: title, Reason: key, Detail: detail}
	np := e.effectivePolicy(inc, pol)
	st, notices, _ := Fail(prevState, now, key, np)
	if prev != nil && st == &prev.State {
		inc = prev
		inc.Detail = detail
	} else {
		inc.State = *st
		if prev != nil {
			e.retireIncident(prev)
		}
	}
	e.saveIncident(inc)
	e.system[key] = inc
	e.event(Event{Type: EventSystem, Severity: "critical", Message: inc.Title, Data: map[string]any{"incident_id": inc.ID, "detail": detail}})
	e.notices(inc, notices, now)
	return inc
}

// resolveSystem resolves a system incident; summary is the body of the
// recovery notice.
func (e *Engine) resolveSystem(key string, now time.Time, summary string) {
	inc := e.system[key]
	if inc == nil || !inc.State.Open() {
		return
	}
	inc.Summary = summary
	notices := Recover(&inc.State, now, e.effectivePolicy(inc, e.policy(inc.Policy)))
	e.saveIncident(inc)
	e.event(Event{Type: EventSystem, Severity: "info", Message: inc.Title + " — resolved", Data: map[string]any{"msg": "system_resolved", "title": inc.Title, "incident_id": inc.ID, "duration_s": int(now.Sub(inc.State.OpenedAt).Seconds())}})
	e.notices(inc, notices, now)
}

// onLocalNet handles local network anomaly transitions.
func (e *Engine) onLocalNet(ev LocalNetEvent, now time.Time) {
	a := ev.Anomaly
	if ev.Enter {
		e.event(Event{Type: EventSystem, Severity: "warning", Message: e.text("anomaly_"+a.Kind, a.Detail, a.Suspected),
			Data: map[string]any{"kind": a.Kind, "family": int(a.Family), "since": a.Since, "domains": a.Domains, "suspected": a.Suspected}})
		if len(e.localnet.Active()) == 1 {
			e.openSystem(SystemLocalNetwork, now, e.text("anomaly_"+a.Kind, a.Detail, a.Suspected))
		}
		return
	}
	e.event(Event{Type: EventSystem, Severity: "info", Message: e.text("anomaly_end_" + a.Kind),
		Data: map[string]any{"kind": a.Kind, "since": a.Since, "duration_s": int(now.Sub(a.Since).Seconds())}})
	if len(e.localnet.Active()) > 0 {
		return
	}
	// Re-check the affected monitors right away with normal judgement.
	affected, stillFailing := 0, 0
	delay := e.cfg.Tunables.Duration("engine.first-round-delay")
	for _, name := range e.sortedMonitorNames() {
		rt := e.states[name]
		if rt == nil || !(rt.State.Held || rt.State.Avail == StatusUnknown && rt.State.UnknownReason == UnknownLocalNetwork) {
			continue
		}
		affected++
		if rt.State.Held {
			stillFailing++
		}
		if m := e.monitors[name]; m != nil && !m.Paused && !m.Push && !rt.Running {
			e.schedule(name, now.Add(delay))
		}
	}
	var since time.Time
	if inc := e.system[SystemLocalNetwork]; inc != nil {
		since = inc.State.OpenedAt
	}
	e.resolveSystem(SystemLocalNetwork, now, e.text("outage_summary", since, now, affected, stillFailing))
}

// onInterruption handles a detected suspend or stall.
func (e *Engine) onInterruption(in Interruption, now time.Time) {
	e.lastInterruption = in
	e.event(Event{Type: EventSystem, Severity: "warning", Message: e.text("interrupted_"+in.Kind, in.Duration),
		Data: map[string]any{"kind": in.Kind, "duration_s": int(in.Duration.Seconds())}})
	if in.Kind == InterruptSuspend {
		// Every monitor gets a fresh round soon after the resume, spread so
		// the wake-up is not a burst.
		spread := e.cfg.Tunables.Duration("suspend.recheck-spread")
		for _, name := range e.sortedMonitorNames() {
			if m := e.monitors[name]; m != nil && !m.Paused && !m.Push {
				if rt := e.states[name]; rt != nil && !rt.Running {
					e.schedule(name, now.Add(Spread(name, max(spread, time.Second))))
				}
			}
		}
	}
	if in.Notify {
		e.systemNotice(now, e.text("interrupted_"+in.Kind, in.Duration))
	}
}

// systemNotice sends a one-off message through the system policy.
func (e *Engine) systemNotice(now time.Time, body string) {
	e.enqueue(&Intent{Kind: "SYSTEM", Policy: e.policy("system").Name, CreatedAt: now, DueAt: now, System: true,
		Message: notify.Message{Kind: "SYSTEM", Priority: "high", Title: e.text("system_notice_title"), Body: body, Time: now, Instance: e.cfg.Instance, Lang: e.cfg.Lang}})
}

// StartupNotice reports how the previous run ended: an abnormal exit (no
// clean shutdown marker) is notified; a clean restart only when
// suspend.notify-restart is on. lastBeat is the last liveness timestamp.
func (e *Engine) StartupNotice(clean bool, lastBeat time.Time) {
	f := func() {
		now := e.now()
		if clean {
			e.event(Event{Type: EventSystem, Severity: "info", Message: e.text("restarted"), Data: map[string]any{"msg": "restarted"}})
			if e.cfg.Tunables.Bool("suspend.notify-restart") {
				e.systemNotice(now, e.text("restarted"))
			}
			return
		}
		down := time.Duration(0)
		if !lastBeat.IsZero() {
			down = now.Sub(lastBeat)
		}
		msg := e.text("abnormal_exit", lastBeat, down)
		e.event(Event{Type: EventSystem, Severity: "critical", Message: msg})
		e.systemNotice(now, msg)
	}
	if !e.started.Load() {
		f()
		return
	}
	e.do(f)
}

// onOverload opens or resolves the "host overloaded" system incident.
func (e *Engine) onOverload(ev OverloadEvent, now time.Time) {
	if ev.Enter {
		e.openSystem(SystemOverload, now, e.text("overload", ev.Ratio, ev.Rounds))
		return
	}
	e.resolveSystem(SystemOverload, now, "")
}

// Push records a call to a PUSH monitor's URL (Kuma compatible): up or
// down, an optional message and latency. remote is the caller's address,
// used to judge whether missed pushes could be our own outage.
func (e *Engine) Push(name string, up bool, msg string, ping time.Duration, remote string) bool {
	found := false
	e.do(func() {
		m := e.monitors[name]
		rt := e.states[name]
		if m == nil || rt == nil || !m.Push || m.Paused {
			return
		}
		found = true
		now := e.now()
		rt.LastPush = now
		if a, err := parseAddr(remote); err == nil {
			rt.PushScope = netscope.OfAddr(a)
		}
		res := probe.Result{Vote: probe.VoteOK, Reason: probe.ReasonOK, Detail: msg, Latency: ping}
		if !up {
			res = probe.Result{Vote: probe.VoteFail, Reason: probe.ReasonAssertFailed, Detail: msg}
		}
		e.onResult(roundResult{name: name, defHash: m.DefHash, start: now, end: now, due: now, res: res, push: true})
	})
	return found
}

// checkPushDeadlines fails PUSH monitors whose push is overdue.
func (e *Engine) checkPushDeadlines(now time.Time) {
	for _, name := range e.sortedMonitorNames() {
		m := e.monitors[name]
		rt := e.states[name]
		if m == nil || rt == nil || !m.Push || m.Paused {
			continue
		}
		// A full period from the last push, or from when watching began
		// (this process, this definition, or resuming it), whichever is
		// later.
		last := rt.LastPush
		for _, t := range []time.Time{e.booted, rt.Since} {
			if t.After(last) {
				last = t
			}
		}
		deadline := last.Add(m.PushPeriod + m.PushGrace)
		if now.Before(deadline) || (!rt.LastCheck.IsZero() && !rt.LastCheck.Before(deadline) && now.Sub(rt.LastCheck) < m.PushPeriod) {
			continue
		}
		res := probe.Result{Vote: probe.VoteFail, Reason: probe.ReasonTimeout, Detail: fmt.Sprintf("no push for %s", now.Sub(last).Round(time.Second))}
		e.onResult(roundResult{name: name, defHash: m.DefHash, start: now, end: now, due: now, res: res, push: true})
	}
}

// scheduleReminder computes the next daily open-incident reminder.
func (e *Engine) scheduleReminder(now time.Time) {
	h, mi := e.cfg.Tunables.TimeOfDay("notify.daily-reminder-at")
	e.nextReminder = NextDaily(now, h, mi, e.cfg.Location)
}

// dailyReminder sends one digest of still-open incidents per day.
func (e *Engine) dailyReminder(now time.Time) {
	if e.nextReminder.IsZero() || now.Before(e.nextReminder) {
		return
	}
	e.scheduleReminder(now)
	if !e.cfg.Tunables.Bool("notify.daily-reminder") {
		return
	}
	var lines []string
	var open []*Incident
	for _, inc := range e.incidents {
		if inc.State.Open() && !inc.State.Drill && inc.Kind != IncidentLow {
			open = append(open, inc)
		}
	}
	if len(open) == 0 {
		return
	}
	sort.Slice(open, func(i, j int) bool { return open[i].State.OpenedAt.Before(open[j].State.OpenedAt) })
	for _, inc := range open {
		lines = append(lines, e.text("reminder_line", inc.Title, now.Sub(inc.State.OpenedAt), inc.State.AckedBy, inc.State.NotifyCount))
	}
	e.enqueue(&Intent{Kind: "DIGEST", Policy: e.policy("system").Name, CreatedAt: now, DueAt: now, System: true,
		Message: notify.Message{Kind: "DIGEST", Priority: "high", Title: e.text("reminder_title", len(open)), Body: strings.Join(lines, "\n"), Time: now, Instance: e.cfg.Instance, Lang: e.cfg.Lang}})
}
