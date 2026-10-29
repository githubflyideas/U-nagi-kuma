package app

import (
	"context"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/directory"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/store"
	"github.com/githubflyideas/unagikuma/internal/tunables"
)

// Directory returns the contact directory the channels are checked against.
func (a *App) Directory() *directory.Directory {
	if d := a.dir.Load(); d != nil {
		return d
	}
	return directory.New(nil, nil, 0)
}

func (a *App) loadDirectory(ts tunables.Set) {
	people, err := a.store.People()
	if err != nil {
		a.log.Warn("contact directory", "err", err)
	}
	bounces, err := a.store.Bounces()
	if err != nil {
		a.log.Warn("bounces", "err", err)
	}
	a.dir.Store(directory.New(people, bounces, ts.Int("contacts.bounce-threshold")))
}

// DirectoryChanged reloads the directory and re-applies the channels, so
// someone marked as having left stops receiving at once.
func (a *App) DirectoryChanged() {
	a.reapply() // reloads the directory
	a.emit("directory", nil)
}

// send delivers through the notifier and counts the recipients refused
// as nonexistent; once one crosses the threshold the channels are
// re-applied without it.
func (a *App) send(ctx context.Context, ch notify.Channel, m *notify.Message) notify.Attempt {
	at := a.sender.Send(ctx, ch, m)
	if len(at.Bounces) == 0 {
		return at
	}
	_, kind, ok := directory.AddressKey(ch.Kind)
	if !ok {
		return at
	}
	for _, b := range at.Bounces {
		if err := a.store.RecordBounce(kind, b.Address, b.Reason); err != nil {
			a.log.Warn("recording a bounce", "address", b.Address, "err", err)
		}
		a.log.Warn("recipient refused as nonexistent", "channel", ch.Name, "address", b.Address, "reason", b.Reason)
	}
	// The engine is mid-delivery here: re-apply from another goroutine,
	// once for a burst of bounces.
	if a.dirReload.CompareAndSwap(false, true) {
		go func() {
			defer a.dirReload.Store(false)
			a.DirectoryChanged()
		}()
	}
	return at
}

// ChannelIssues lists the directory problems of every channel that sends
// to people, by channel name.
func (a *App) ChannelIssues() map[string][]directory.Issue {
	out := map[string][]directory.Issue{}
	c := a.applied.Load()
	if c == nil {
		return out
	}
	d := a.Directory()
	for _, ch := range c.Channels {
		key, kind, ok := directory.AddressKey(ch.Kind)
		if !ok || strings.Contains(ch.Opts[key], "${") {
			continue
		}
		if _, issues := d.Resolve(kind, ch.Opts[key]); len(issues) > 0 {
			out[ch.Name] = issues
		}
	}
	return out
}

// UnknownAddresses lists, per kind, the channel addresses the directory
// does not know (what "import from channels" would add).
func (a *App) UnknownAddresses() map[string][]string {
	out := map[string][]string{}
	c := a.applied.Load()
	if c == nil {
		return out
	}
	d := a.Directory()
	seen := map[string]bool{}
	for _, ch := range c.Channels {
		key, kind, ok := directory.AddressKey(ch.Kind)
		if !ok || strings.Contains(ch.Opts[key], "${") {
			continue
		}
		for _, v := range d.Unknown(kind, ch.Opts[key]) {
			k := kind + "|" + strings.ToLower(v)
			if kind == store.ContactPhone {
				k = kind + "|" + store.NormalizePhone(v)
			}
			if !seen[k] {
				seen[k] = true
				out[kind] = append(out[kind], v)
			}
		}
	}
	return out
}
