package app

import (
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// sink is exclusively the legacy default runtime adapter. Do not embed Store:
// adding a Store method must never silently add an unguarded engine capability.
// This is NOT a Workspace sink or permission to activate nondefault execution.
type sink struct {
	Store *store.Store
	a     *App
}

var _ engine.Sink = (*sink)(nil)
var _ engine.DecisionSink = (*sink)(nil)

func (s *sink) allowed(name string) bool { return s.Store.CheckDefaultRuntimeRecord(name) == nil }
func (s *sink) incidentAllowed(id int64) bool {
	// Nonpositive IDs include instance notices and temporary disk-full incidents.
	if id <= 0 {
		return true
	}
	inc, err := s.Store.Incident(id)
	return err == nil && inc != nil && s.allowed(inc.Monitor)
}
func (s *sink) SaveState(r engine.StateRecord) {
	if !s.allowed(r.Name) || !s.incidentAllowed(r.IncidentID) {
		return
	}
	s.Store.SaveState(r)
	s.a.emit("state", map[string]any{"name": r.Name})
}
func (s *sink) DeleteState(name string) {
	if s.allowed(name) {
		s.Store.DeleteState(name)
	}
}
func (s *sink) SaveIncident(inc *engine.Incident) error {
	if inc == nil || !s.allowed(inc.Monitor) || !s.incidentAllowed(inc.ID) {
		return store.ErrTenantAccess
	}
	err := s.Store.SaveIncident(inc)
	if err == nil {
		s.a.emit("incident", map[string]any{"id": inc.ID, "monitor": inc.Monitor})
	}
	return err
}
func (s *sink) intentAllowed(in *engine.Intent) bool {
	if in == nil || !s.allowed(in.Message.Monitor) || !s.incidentAllowed(in.IncidentID) || !s.incidentAllowed(in.Message.IncidentID) {
		return false
	}
	for _, id := range in.Covers {
		if !s.incidentAllowed(id) {
			return false
		}
	}
	return true
}
func (s *sink) SaveIntent(in *engine.Intent) error {
	if !s.intentAllowed(in) {
		return store.ErrTenantAccess
	}
	return s.Store.SaveIntent(in)
}
func (s *sink) Event(ev engine.Event) {
	if !s.allowed(ev.Monitor) {
		return
	}
	s.Store.Event(ev)
	s.a.emit("event", ev)
}
func (s *sink) Verdict(v engine.VerdictRecord) {
	if !s.allowed(v.Monitor) {
		return
	}
	s.Store.Verdict(v)
	s.a.emit("heartbeat", map[string]any{"name": v.Monitor, "cell": map[string]any{"t": v.At, "s": v.Status, "n": 1}})
}
func (s *sink) Evidence(name, node string, at time.Time, r probe.Result) {
	if s.allowed(name) {
		s.Store.Evidence(name, node, at, r)
	}
}
func (s *sink) Delivery(d engine.DeliveryRecord) {
	if s.incidentAllowed(d.IncidentID) {
		s.Store.Delivery(d)
	}
}
func (s *sink) Decision(d engine.Decision) {
	if s.allowed(d.Monitor) && s.incidentAllowed(d.IncidentID) {
		s.Store.Decision(d)
	}
}
func (s *sink) Counter(name string, n int) { s.Store.Counter(name, n) }
