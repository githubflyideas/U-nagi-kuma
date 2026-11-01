package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/store"
)

const zoneAppliedSetting = "zone_applied_v1"

// RuntimeConfig returns the immutable effective model (base plus applied zones).
// Callers must not mutate it. Config remains the editable base-file model.
func (a *App) RuntimeConfig() *config.Config { return a.applied.Load() }

// Zone management methods are local administrative APIs. HTTP callers must
// authenticate and authorize zone management; imported provenance grants no trust.
// A successful ApplyZone means durable configuration publication, not that a
// check or notification has already completed.

// ZonePreview is non-mutating and never contains resolved credential values.
type ZonePreview struct {
	Missing  []string `json:"missing"`
	Monitors []string `json:"monitors"`
}

func (a *App) zoneRefKnown(ref string) bool {
	if strings.HasPrefix(ref, "env:") {
		name := strings.TrimPrefix(ref, "env:")
		return (strings.HasPrefix(name, "UNAGIKUMA_") || contains(a.opts.AllowEnv, name)) && os.Getenv(name) != ""
	}
	return strings.HasPrefix(ref, "secret:") && a.secrets.Has(strings.TrimPrefix(ref, "secret:"))
}
func (a *App) PreviewZone(raw []byte, o ZoneImportOptions) (ZonePreview, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	var out ZonePreview
	if len(raw) > MaxZoneBytes {
		return out, fmt.Errorf("zone import too large")
	}
	origin, c, _, err := config.ParseZone("preview.bind", raw, a.zones.validationOptions())
	if err != nil {
		return out, err
	}
	name := o.Name
	if name == "" && origin != nil {
		name = origin.Name
	}
	if !config.ValidName(name) {
		return out, fmt.Errorf("zone name required")
	}
	a.zones.mu.Lock()
	defer a.zones.mu.Unlock()
	if o.TargetID != "" {
		z, err := a.zones.get(o.TargetID)
		if err != nil {
			return out, err
		}
		if o.ExpectedRevision == "" || z.Revision() != o.ExpectedRevision {
			return out, ErrConflict
		}
		if z.Role(a.zones.local) == "secondary" {
			return out, ErrReadonly
		}
		if err = config.CheckSerial(c.Serial, z.Header.Serial); err != nil || c.Serial == z.Header.Serial {
			return out, fmt.Errorf("updated SOA serial must advance")
		}
	}
	if err = a.zones.nameAvailable(name, o.TargetID); err != nil {
		return out, err
	}
	missing := map[string]bool{}
	for _, m := range zoneExportRef.FindAllStringSubmatch(string(raw), -1) {
		ref := m[1] + ":" + m[2]
		if !a.zoneRefKnown(ref) {
			missing[ref] = true
		}
	}
	out.Missing = slices.Sorted(maps.Keys(missing))
	out.Monitors = []string{}
	for _, m := range c.Monitors {
		out.Monitors = append(out.Monitors, m.Name)
	}
	return out, nil
}
func (a *App) ExportZone(id string) (ZoneExport, error) { return a.zones.Export(id, a.zoneRefKnown) }

type ZoneStatus struct {
	Record          ZoneRecord `json:"record"`
	AppliedRevision string     `json:"applied_revision,omitempty"`
}

func (a *App) initZones() error {
	var err error
	a.zones, err = OpenZoneStore(a.opts.DataDir, a.cfg.current().Config.MName, a.validators())
	if err != nil {
		return err
	}
	a.zoneApplied = map[string]ZoneRecord{}
	raw, err := a.store.Setting(zoneAppliedSetting)
	if err != nil {
		return err
	}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &a.zoneApplied); err != nil {
			return fmt.Errorf("invalid applied zones: %w", err)
		}
	}
	if a.zoneApplied == nil {
		return fmt.Errorf("invalid applied zones: null manifest")
	}
	for id, z := range a.zoneApplied {
		if id != z.Header.ZoneID || !config.ValidZoneID(id) || z.Version != 1 || config.ZoneHash([]byte(z.Body)) != z.Header.ContentHash {
			return fmt.Errorf("invalid applied zone snapshot")
		}
		if _, err = a.parseRuntimeZone(z); err != nil {
			return err
		}
	}
	return nil
}

// ImportZone persists a draft only. Applying requires its current revision.
func (a *App) ImportZone(raw []byte, o ZoneImportOptions) (ZoneRecord, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return ZoneRecord{}, ErrSuperseded
	}
	return a.zones.Import(raw, o)
}
func (a *App) RebindZone(id, expected string, binding ZoneBinding) (ZoneRecord, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return ZoneRecord{}, ErrSuperseded
	}
	return a.zones.Rebind(id, expected, binding)
}
func (a *App) ListZones() ([]ZoneStatus, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	a.zones.mu.Lock()
	defer a.zones.mu.Unlock()
	es, err := os.ReadDir(a.zones.dir)
	if err != nil {
		return nil, err
	}
	out := []ZoneStatus{}
	for _, e := range es {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		z, err := a.zones.get(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		s := ZoneStatus{Record: z}
		if v, ok := a.zoneApplied[z.Header.ZoneID]; ok {
			s.AppliedRevision = v.Revision()
		}
		out = append(out, s)
	}
	return out, nil
}

func (a *App) parseRuntimeZone(z ZoneRecord) (*config.Config, error) {
	_, c, _, err := config.ParseZone("zone.bind", []byte(z.Body), a.zones.validationOptions())
	if err != nil {
		return nil, err
	}
	if len(c.StatusPages) > 0 {
		return nil, fmt.Errorf("zone runtime does not support STATUSPAGE records")
	}
	// NODEs in zones authorize local participants, never central agents.
	// Empty assignments retain the legacy zone-local execution behavior.
	local := c.Monitors[:0]
	for _, m := range c.Monitors {
		if len(m.Nodes) == 0 || slices.Contains(m.Nodes, a.zones.local) {
			local = append(local, m)
		}
	}
	c.Monitors = local
	for _, m := range local {
		for _, parent := range m.Parents {
			if c.FindMonitor(parent) == nil {
				return nil, fmt.Errorf("local zone monitor %s requires nonlocal parent %s", m.Name, parent)
			}
		}
	}
	c.Nodes = nil
	return namespaceZone(z.Header.ZoneID, c), nil
}

// namespaceZone rewrites typed dependency edges, never arbitrary target,
// title, payload, credential or tag strings. c is a newly parsed private model.
func namespaceZone(id string, c *config.Config) *config.Config {
	prefix := "zone-" + id + "."
	name := func(s string) string {
		if s == "" {
			return s
		}
		return prefix + s
	}
	names := func(ss []string) []string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = name(s)
		}
		return out
	}
	for _, m := range c.Monitors {
		m.Name = name(m.Name)
		m.Policy = name(m.Policy)
		m.Parents = names(m.Parents)
		delete(m.Opts, "was")
	}
	for _, ch := range c.Channels {
		ch.Name = name(ch.Name)
		ch.Members = names(ch.Members)
		if ch.Period != "" {
			if strings.HasPrefix(ch.Period, "!") {
				ch.Period = "!" + name(strings.TrimPrefix(ch.Period, "!"))
			} else {
				ch.Period = name(ch.Period)
			}
		}
	}
	for _, p := range c.Policies {
		p.Name = name(p.Name)
		p.Chain = names(p.Chain)
		p.Network = names(p.Network)
		for i := range p.Escalate {
			p.Escalate[i].Channels = names(p.Escalate[i].Channels)
		}
	}
	for _, p := range c.Periods {
		p.Name = name(p.Name)
	}
	for _, m := range c.Maints {
		m.Name = name(m.Name)
		m.Monitors = names(m.Monitors)
	}
	return c
}

// composeRuntime compiles zones independently so legacy base NODE routing
// cannot distribute local zone checks to central agents.
func (a *App) composeRuntime(base *config.Config, zones map[string]ZoneRecord, strict ...bool) (*config.Config, engine.Config, error) {
	combined := *base
	combined.Monitors = append([]*config.Monitor(nil), base.Monitors...)
	combined.Channels = append([]*config.Channel(nil), base.Channels...)
	combined.Policies = append([]*config.Policy(nil), base.Policies...)
	combined.Periods = append([]*config.Period(nil), base.Periods...)
	combined.Maints = append([]*config.Maint(nil), base.Maints...)
	ec := a.engineConfig(base)
	ids := slices.Sorted(maps.Keys(zones))
	for _, id := range ids {
		c, err := a.parseRuntimeZone(zones[id])
		if err != nil {
			return nil, engine.Config{}, err
		}
		for _, m := range c.Monitors {
			if combined.HasName(m.Name) {
				return nil, engine.Config{}, fmt.Errorf("zone runtime name collision")
			}
		}
		resolve := a.resolver()
		// Cache each reference, including values used by scrub lists. Compilation
		// consumes this same resolver; there is no post-persistence resolution.
		cached := map[string]string{}
		failures := map[string]error{}
		original := resolve
		resolve = func(s string) (string, error) {
			var failure error
			out := refRe.ReplaceAllStringFunc(s, func(ref string) string {
				if v, ok := cached[ref]; ok {
					return v
				}
				if err, ok := failures[ref]; ok {
					failure = err
					return ""
				}
				m := refRe.FindStringSubmatch(ref)
				if !a.zoneRefKnown(m[1] + ":" + m[2]) {
					failure = fmt.Errorf("missing zone dependency %s:%s", m[1], m[2])
					failures[ref] = failure
					return ""
				}
				v, err := original(ref)
				if err != nil {
					failure = fmt.Errorf("unavailable zone dependency %s:%s", m[1], m[2])
					failures[ref] = failure
					return ""
				}
				cached[ref] = v
				return v
			})
			return out, failure
		}
		if len(strict) > 0 && strict[0] {
			for _, m := range c.Monitors {
				spec, err := ProbeSpec(m, resolve)
				if err != nil {
					return nil, engine.Config{}, err
				}
				if err = probe.Validate(spec); err != nil {
					return nil, engine.Config{}, fmt.Errorf("invalid resolved zone monitor %s", m.Name)
				}
			}
			for _, ch := range c.Channels {
				opts := map[string]string{}
				for k, v := range ch.Opts {
					rv, err := resolve(v)
					if err != nil {
						return nil, engine.Config{}, err
					}
					opts[k] = rv
				}
				if ch.Kind != "group" {
					if err := ValidateChannel(ch.Kind, opts); err != nil {
						return nil, engine.Config{}, fmt.Errorf("invalid resolved zone channel %s", ch.Name)
					}
				}
			}
		}
		zc := a.engineConfigResolved(c, resolve)
		// A missing optional authentication header is still a dependency
		// failure. Do not let the sender treat its empty value as anonymous
		// delivery; other channels/zones remain available.
		for _, ch := range c.Channels {
			for _, value := range ch.Opts {
				if _, err := resolve(value); err != nil {
					cs := zc.Channels[ch.Name]
					cs.NoRecipients = "zone channel dependency unavailable"
					zc.Channels[ch.Name] = cs
					break
				}
			}
		}
		ec.Monitors = append(ec.Monitors, zc.Monitors...)
		ec.Maints = append(ec.Maints, zc.Maints...)
		for k, v := range zc.Channels {
			if _, ok := ec.Channels[k]; ok {
				return nil, engine.Config{}, fmt.Errorf("zone channel collision")
			}
			ec.Channels[k] = v
		}
		for k, v := range zc.Policies {
			if _, ok := ec.Policies[k]; ok {
				return nil, engine.Config{}, fmt.Errorf("zone policy collision")
			}
			ec.Policies[k] = v
		}
		combined.Monitors = append(combined.Monitors, c.Monitors...)
		combined.Channels = append(combined.Channels, c.Channels...)
		combined.Policies = append(combined.Policies, c.Policies...)
		combined.Periods = append(combined.Periods, c.Periods...)
		combined.Maints = append(combined.Maints, c.Maints...)
	}
	return &combined, ec, nil
}

func runtimeDefs(combined *config.Config) []store.MonitorDef {
	defs := make([]store.MonitorDef, 0, len(combined.Monitors))
	for _, m := range combined.Monitors {
		defs = append(defs, store.MonitorDef{Name: m.Name, DefHash: m.DefHash})
	}
	return defs
}

func (a *App) installRuntime(base, combined *config.Config, ec engine.Config) {
	if err := a.store.SyncMonitors(runtimeDefs(combined)); err != nil {
		a.log.Error("monitor ids", "err", err)
		return
	}
	a.publishRuntime(base, combined, ec)
}

// publishRuntime must only follow a committed identity registration.
func (a *App) publishRuntime(base, combined *config.Config, ec engine.Config) {
	a.baseApplied = base
	a.applied.Store(combined)
	if a.SupersededBy() != nil {
		ec.Monitors, ec.Maints = nil, nil
	}
	a.engine.Apply(ec)
	a.master.SetConfigs(agentConfigs(base, ec))
	go a.syncStandbys()
}

// ApplyZone atomically saves the exact applied snapshot, independently of
// subsequent draft imports or source rebinding. No upstream is contacted.
func (a *App) ApplyZone(id, expected string) (ZoneRecord, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return ZoneRecord{}, ErrSuperseded
	}
	z, err := a.zones.Get(id)
	if err != nil {
		return ZoneRecord{}, err
	}
	if expected == "" || expected != z.Revision() {
		return ZoneRecord{}, ErrConflict
	}
	next := maps.Clone(a.zoneApplied)
	next[id] = z
	combined, ec, err := a.composeRuntime(a.baseApplied, next, true)
	if err != nil {
		return ZoneRecord{}, err
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return ZoneRecord{}, err
	}
	if err = a.store.SyncDefaultRuntimeSnapshot(runtimeDefs(combined), zoneAppliedSetting, string(raw)); err != nil {
		return ZoneRecord{}, err
	}
	a.zoneApplied = next
	a.publishRuntime(a.baseApplied, combined, ec)
	return z, nil
}
