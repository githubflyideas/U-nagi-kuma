package app

import (
	"context"
	"fmt"
	"time"
)

// ZoneSyncOptions belongs to the local subscriber, never a portable zone.
// Keep Token in a local secret store. Interval defaults to five minutes and
// supports up to one hour. This is configuration polling, not discovery or votes.
type ZoneSyncOptions struct {
	Endpoint string
	Token    string
	Interval time.Duration
}

// RunZoneSync is a blocking worker with an explicit caller-owned lifecycle.
// Start at most one per local zone; cancel and JOIN before closing App.Run or
// changing endpoint/token. Every iteration re-reads the authorized binding.
// Request timeout is ten seconds; failures retry with 1s..1m capped exponential
// backoff. Failure never removes the last-good applied runtime snapshot.
func (a *App) RunZoneSync(ctx context.Context, id string, o ZoneSyncOptions) error {
	if o.Interval == 0 {
		o.Interval = 5 * time.Minute
	}
	if o.Interval < time.Second || o.Interval > time.Hour {
		return fmt.Errorf("zone sync interval must be 1s..1h")
	}
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := a.SyncZone(ctx, id, o.Endpoint, o.Token)
		wait := o.Interval
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Do not log endpoint, token or untrusted response bodies.
			a.log.Warn("zone configuration sync unavailable; retaining last-good local checks", "zone", id)
			wait = backoff
			backoff = min(backoff*2, time.Minute)
		} else {
			backoff = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
