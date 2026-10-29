package config

import (
	"strconv"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/keywords"
	"github.com/githubflyideas/unagikuma/internal/schedule"
	"github.com/githubflyideas/unagikuma/internal/tunables"
)

// Severity of a diagnostic.
const (
	SevError   = "error"
	SevWarning = "warning"
)

// Diagnostic is a configuration error or warning at a position.
type Diagnostic struct {
	Pos      Pos    `json:"pos"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Msg      string `json:"message"`
}

func (d Diagnostic) Error() string { return d.Pos.String() + ": " + d.Severity + ": " + d.Msg }

// HasErrors reports whether any diagnostic is an error.
func HasErrors(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == SevError {
			return true
		}
	}
	return false
}

// Defaults are the SOA timing defaults.
type Defaults struct {
	Interval time.Duration
	Retry    time.Duration
	Timeout  time.Duration
	Attempts int
}

// Tag is a key[:value] label.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

func (t Tag) String() string {
	if t.Value == "" {
		return t.Key
	}
	return t.Key + ":" + t.Value
}

// TagMeta is the $TAG presentation of a tag key (or key:value).
type TagMeta struct {
	Color       string
	Description string
}

// Config is the validated semantic model of a file.
type Config struct {
	Serial   uint32
	MName    string // master node name (without trailing dot)
	RName    string // administrator e-mail (ops@example.com)
	Defaults Defaults
	Quorum   string
	Dialect  keywords.Dialect
	Tunables tunables.Set
	Tags     map[string]TagMeta

	Monitors    []*Monitor
	Nodes       []*Node
	Channels    []*Channel
	Policies    []*Policy
	StatusPages []*StatusPage
	Maints      []*Maint
	Periods     []*Period
}

// Period is a weekly time period channels are limited to.
type Period struct {
	Name  string
	Spec  schedule.Period
	Pos   Pos
	Lines [2]int
}

// Escalation notifies more channels while an incident stays unacknowledged.
type Escalation struct {
	After    time.Duration
	Channels []string
}

// Maint is a maintenance window: while it is open the selected monitors
// are not notified and their failures do not count against uptime.
type Maint struct {
	Name     string
	Title    string
	Select   *Selector
	Schedule schedule.Spec
	Notify   bool     // announce start and end
	Monitors []string // selected at load time
	Pos      Pos
	Lines    [2]int
}

// Monitor is one check.
type Monitor struct {
	Name        string
	Type        string // HTTP TCP PING UDP PUSH (DNS/NTP/SNMP/SYSLOG normalised to UDP)
	Written     string // type as written (DNS, NTP, ...)
	Target      string
	Title       string
	Description string
	Group       string
	Tags        []Tag

	Interval        time.Duration
	Retry           time.Duration
	Timeout         time.Duration
	Attempts        int
	RecoverAttempts int

	Nodes           []string // nodes that check it (regions expanded); empty = every probing node
	Area            string   // the $SCOPE it is written under, "" when global
	Parents         []string // monitors this one depends on (parent=)
	Device          string   // host it is listed under (device=), "" = its target host
	parentPos       Pos
	Quorum          string
	OnInsufficient  string
	OnFiltered      string
	OnNodeLoss      string
	PartialEscalate time.Duration
	Scope           string // auto | lan | wan
	Policy          string
	Degraded        time.Duration
	CertDays        int
	CertCritical    int
	Authorized      bool
	Disabled        bool

	// Push monitors.
	Period time.Duration
	Grace  time.Duration

	// Opts holds every effective option (after $DEFAULT), including the
	// type-specific ones the probe interprets. References are unresolved.
	Opts map[string]string
	// Refs lists ${secret:...} and ${env:...} references used.
	Refs []string
	UDP  *UDPTarget

	Pos   Pos
	Lines [2]int // statement span

	DefHash, ParamHash, MetaHash string
}

// UDPTarget is the resolved UDP target.
type UDPTarget struct {
	Host     string
	Port     int
	Keyword  string // canonical keyword, "" for raw ports
	Template string
	Depth    keywords.Depth
	Entry    *keywords.Entry
}

// Node is a probe node.
type Node struct {
	Name        string
	Description string
	Probe       bool
	AnchorsLAN  []string
	AnchorsWAN  []string
	NoAnchors   bool
	LocalDetect bool
	Region      string
	Policy      string
	NodeTimeout time.Duration
	// Standby is the address (https://host:port) at which this node can
	// take over as master; it keeps a copy of the master's state.
	Standby string
	Opts    map[string]string
	Pos     Pos
	Lines   [2]int
}

// Channel is a notification channel.
type Channel struct {
	Name    string
	Kind    string
	Opts    map[string]string // kind-specific options (refs unresolved)
	Retries int
	Rate    string
	// Period limits the channel to a time period ("!name" = outside it).
	Period string
	// Members are the channels of a "group" channel, all notified at once.
	Members []string
	Refs    []string
	Pos     Pos
	Lines   [2]int
}

// Policy is a notification policy. Unset numbers are filled from the
// tunables; the raw strings are kept for display.
type Policy struct {
	Name             string
	Chain            []string
	Mode             string // fallback | all
	Repeat           time.Duration
	Max              int
	Delay            time.Duration
	ReopenWindow     time.Duration
	Recovery         bool
	NotifyDegraded   bool
	AckExpire        time.Duration
	AckClearOnChange bool
	RecoveryAfterAck bool
	Escalate         []Escalation
	Opts             map[string]string
	Implicit         bool // "default"/"system" not written in the file
	Pos              Pos
	Lines            [2]int
}

// StatusPage is a public status page.
type StatusPage struct {
	Name    string
	Title   string
	Select  string
	Windows []string
	Path    string
	Logo    string
	Footer  string
	Theme   string
	// Domain serves the page at the root of its own host name.
	Domain string
	// Subscribe names the e-mail channel subscriptions are sent through
	// ("" = no subscriptions).
	Subscribe    string
	subscribePos Pos
	Pos          Pos
	Lines        [2]int
}

// Monitor types and their shorthands.
var monitorTypes = map[string]bool{"HTTP": true, "TCP": true, "PING": true, "UDP": true, "DNS": true, "NTP": true, "SNMP": true, "SYSLOG": true, "PUSH": true, "EXEC": true,
	"REDIS": true, "POSTGRES": true, "MYSQL": true, "GRPC": true, "DOCKER": true, "DOMAIN": true}

// IsMonitorType reports whether t (upper case) is a monitor record type.
func IsMonitorType(t string) bool { return monitorTypes[t] }

// FindMonitor returns a monitor by name.
func (c *Config) FindMonitor(name string) *Monitor {
	for _, m := range c.Monitors {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// HasName reports whether a record of any type is called name: names are
// unique across record types.
func (c *Config) HasName(name string) bool {
	if c.FindMonitor(name) != nil || c.FindChannel(name) != nil || c.FindNode(name) != nil {
		return true
	}
	if p := c.FindPolicy(name); p != nil && !p.Implicit {
		return true
	}
	for _, sp := range c.StatusPages {
		if sp.Name == name {
			return true
		}
	}
	return false
}

// FindNode returns a node by name.
func (c *Config) FindNode(name string) *Node {
	for _, n := range c.Nodes {
		if n.Name == name {
			return n
		}
	}
	return nil
}

// FindPolicy returns a policy by name.
func (c *Config) FindPolicy(name string) *Policy {
	for _, p := range c.Policies {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// FindChannel returns a channel by name.
// FindPeriod returns the named time period, or nil.
func (c *Config) FindPeriod(name string) *Period {
	for _, p := range c.Periods {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (c *Config) FindChannel(name string) *Channel {
	for _, ch := range c.Channels {
		if ch.Name == name {
			return ch
		}
	}
	return nil
}

// EscalateText writes escalation steps back in the escalate= form.
func EscalateText(steps []Escalation) string {
	parts := make([]string, len(steps))
	for i, s := range steps {
		parts[i] = shortDuration(s.After) + ":" + strings.Join(s.Channels, "+")
	}
	return strings.Join(parts, ",")
}

func shortDuration(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	case d%time.Minute == 0:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	}
	return strconv.Itoa(int(d/time.Second)) + "s"
}
