package app

import (
	"fmt"
	"github.com/githubflyideas/unagikuma/internal/config"
)

// restoreDefaultRuntime excludes known foreign/unregistered records before the
// name-keyed legacy engine can coalesce, expose, or dispatch them. This is not a
// scoped business-record migration: nondefault runtime remains unavailable.
func (a *App) restoreDefaultRuntime() error {
	states, err := a.store.States()
	if err != nil {
		return err
	}
	incidents, err := a.store.OpenIncidents()
	if err != nil {
		return err
	}
	intents, err := a.store.PendingIntents()
	if err != nil {
		return err
	}
	boundary := &sink{Store: a.store, a: a}
	ss := states[:0]
	for _, r := range states {
		if boundary.allowed(r.Name) && boundary.incidentAllowed(r.IncidentID) {
			ss = append(ss, r)
		}
	}
	ii := incidents[:0]
	for _, r := range incidents {
		if boundary.allowed(r.Monitor) {
			ii = append(ii, r)
		}
	}
	nn := intents[:0]
	for _, r := range intents {
		if boundary.intentAllowed(r) {
			nn = append(nn, r)
		}
	}
	a.engine.Restore(ss, ii, nn)
	return nil
}

// baseValidators checks editable base names, not unnamespaced Zone-local names.
func (a *App) baseValidators() config.Options {
	o := a.validators()
	validate := o.ValidateMonitor
	o.ValidateMonitor = func(m *config.Monitor) error {
		if err := a.store.CheckDefaultRuntimeName(m.Name); err != nil {
			return fmt.Errorf("monitor is unavailable to the default runtime")
		}
		return validate(m)
	}
	return o
}
