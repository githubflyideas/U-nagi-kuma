package app

import (
	"sort"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/directory"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// When the shared infrastructure fails (a monitor that belongs to no
// team), the managers of the teams it affects are told as well as the
// usual policy: the teams with monitors that depend on it, or every team
// when none does. Each manager is reached through the channels their
// contact entry names (Notify via), at their own address.
//
// This is built from what the engine already has: every team gets a
// contact group of its managers' personal channels, and a shared monitor
// gets a copy of its policy whose first escalation step, right away, goes
// to the groups of the teams it affects. So the notice is capped, logged
// and reported like any escalation, and is not sent once the problem is
// acknowledged.

// managerGroupName names the contact group of a team's managers. Record
// names cannot contain ':', so it never clashes with a channel.
func managerGroupName(team string) string { return "managers:" + team }

// addManagerNotices adds the managers' channels and groups, and the
// policies of the shared monitors that affect teams, to ec.
func (a *App) addManagerNotices(cfg *config.Config, ec *engine.Config) {
	if len(cfg.Teams) == 0 {
		return
	}
	people := map[string]store.Person{}
	for _, p := range a.Directory().People() {
		if p.Username != "" && !p.Left() {
			people[p.Username] = p
		}
	}
	groups := map[string]bool{}
	for _, t := range cfg.Teams {
		var members []string
		for _, u := range t.Managers {
			p, ok := people[u]
			if !ok {
				continue
			}
			for _, via := range p.Notify {
				base, ok := ec.Channels[via]
				if !ok {
					continue
				}
				key, kind, ok := directory.AddressKey(base.Kind)
				if !ok {
					continue
				}
				addrs := p.Emails
				if kind == store.ContactPhone {
					addrs = p.Phones
				}
				if len(addrs) == 0 {
					continue
				}
				opts := make(map[string]string, len(base.Opts))
				for k, v := range base.Opts {
					opts[k] = v
				}
				opts[key] = strings.Join(addrs, ",")
				name := "manager:" + t.Name + ":" + u + ":" + via
				cs := base
				cs.Channel = notify.Channel{Name: name, Kind: base.Kind, Opts: opts}
				cs.Members, cs.Rotation, cs.NoRecipients = nil, nil, ""
				ec.Channels[name] = cs
				members = append(members, name)
			}
		}
		if len(members) > 0 {
			g := managerGroupName(t.Name)
			ec.Channels[g] = engine.ChannelSpec{Channel: notify.Channel{Name: g, Kind: "group"}, Members: members}
			groups[t.Name] = true
		}
	}
	if len(groups) == 0 {
		return
	}
	affected := affectedTeams(cfg)
	var all []string
	for t := range groups {
		all = append(all, t)
	}
	sort.Strings(all)
	for i := range ec.Monitors {
		ms := &ec.Monitors[i]
		m := cfg.FindMonitor(ms.Name)
		if m == nil || m.Team != "" {
			continue
		}
		teams := affected[m.Name]
		if len(teams) == 0 {
			teams = all
		}
		var chans []string
		for _, t := range teams {
			if groups[t] {
				chans = append(chans, managerGroupName(t))
			}
		}
		if len(chans) == 0 {
			continue
		}
		base, ok := ec.Policies[ms.Policy]
		if !ok {
			continue
		}
		name := base.Name + " + managers of " + strings.Join(teams, ", ")
		if _, done := ec.Policies[name]; !done {
			p := base
			p.Name = name
			p.Escalate = append([]engine.EscalationStep{{After: 0, Channels: chans}}, base.Escalate...)
			ec.Policies[name] = p
		}
		ms.Policy = name
	}
}

// affectedTeams maps each monitor to the teams whose monitors depend on it,
// directly or through other monitors.
func affectedTeams(cfg *config.Config) map[string][]string {
	sets := map[string]map[string]bool{}
	for _, m := range cfg.Monitors {
		if m.Team == "" {
			continue
		}
		seen := map[string]bool{}
		var up func(names []string)
		up = func(names []string) {
			for _, p := range names {
				if seen[p] {
					continue
				}
				seen[p] = true
				if sets[p] == nil {
					sets[p] = map[string]bool{}
				}
				sets[p][m.Team] = true
				if pm := cfg.FindMonitor(p); pm != nil {
					up(pm.Parents)
				}
			}
		}
		up(m.Parents)
	}
	out := map[string][]string{}
	for mon, set := range sets {
		for t := range set {
			out[mon] = append(out[mon], t)
		}
		sort.Strings(out[mon])
	}
	return out
}
