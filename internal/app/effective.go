package app

import "github.com/githubflyideas/unagikuma/internal/config"

// EffectiveConfig is the read-only configuration this site actually runs.
// Config() remains the authority's editable file, never a replica edit target.
func (a *App) EffectiveConfig() *config.Config {
	if a.isMember() {
		a.replica.mu.Lock()
		c := a.replica.cfg
		a.replica.mu.Unlock()
		if c != nil {
			return c
		}
	}
	return a.applied.Load()
}
