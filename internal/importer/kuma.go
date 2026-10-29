// Package importer converts other monitoring tools' exports into
// monitors.bind records.
package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/config"
)

// Result is a conversion: records to add, secrets to store, and what could
// not be carried over.
type Result struct {
	Records  []config.RecordSpec `json:"-"`
	Secrets  map[string]string   `json:"-"`
	Text     string              `json:"text"` // the records as they will be written
	Monitors int                 `json:"monitors"`
	Channels int                 `json:"channels"`
	Notes    []string            `json:"notes"` // things that changed or were left out
	Area     string              `json:"area"`  // $SCOPE section the monitors go to
	Users    []KumaUser          `json:"users"` // accounts found in kuma.db
}

// Options steer an import.
type Options struct {
	// Area is the $SCOPE section imported monitors are written to, e.g.
	// "region:north" ("" = the global section). Channels and policies
	// stay global: they are shared configuration.
	Area string
}

type kumaBackup struct {
	Version          string             `json:"version"`
	NotificationList []kumaNotification `json:"notificationList"`
	MonitorList      []kumaMonitor      `json:"monitorList"`
}

type kumaNotification struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Config string `json:"config"` // JSON object as a string
	Active flag   `json:"active"`
}

type kumaTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type kumaMonitor struct {
	ID               int64           `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Type             string          `json:"type"`
	URL              string          `json:"url"`
	Method           string          `json:"method"`
	Body             string          `json:"body"`
	Headers          string          `json:"headers"`
	Hostname         string          `json:"hostname"`
	Port             int             `json:"port"`
	Interval         int             `json:"interval"`
	RetryInterval    int             `json:"retryInterval"`
	MaxRetries       int             `json:"maxretries"`
	Timeout          float64         `json:"timeout"`
	Active           flag            `json:"active"`
	Keyword          string          `json:"keyword"`
	InvertKeyword    flag            `json:"invertKeyword"`
	JSONPath         string          `json:"jsonPath"`
	ExpectedValue    string          `json:"expectedValue"`
	IgnoreTLS        flag            `json:"ignoreTls"`
	UpsideDown       flag            `json:"upsideDown"`
	MaxRedirects     *int            `json:"maxredirects"`
	AcceptedStatus   []string        `json:"accepted_statuscodes"`
	DNSResolveType   string          `json:"dns_resolve_type"`
	DNSResolveServer string          `json:"dns_resolve_server"`
	DBConn           string          `json:"databaseConnectionString"`
	DockerContainer  string          `json:"docker_container"`
	DockerHost       *int64          `json:"docker_host"`
	GRPCURL          string          `json:"grpcUrl"`
	GRPCService      string          `json:"grpcServiceName"`
	GRPCTLS          flag            `json:"grpcEnableTls"`
	BasicUser        string          `json:"basic_auth_user"`
	BasicPass        string          `json:"basic_auth_pass"`
	Parent           *int64          `json:"parent"`
	ExpiryNotify     flag            `json:"expiryNotification"`
	Notifications    map[string]bool `json:"notificationIDList"`
	Tags             []kumaTag       `json:"tags"`
}

// flag accepts Kuma's booleans, written as true/false or 1/0.
type flag bool

func (f *flag) UnmarshalJSON(b []byte) error {
	switch strings.Trim(string(b), `"`) {
	case "true", "1":
		*f = true
	default:
		*f = false
	}
	return nil
}

// Kuma converts an Uptime Kuma database (kuma.db) or JSON backup
// (Settings > Backup > Export in version 1). Names that clash with records
// of existing are renamed.
func Kuma(data []byte, existing *config.Config, o Options) (*Result, error) {
	var bk kumaBackup
	var users []KumaUser
	if isSQLite(data) {
		b, u, err := readKumaDB(data)
		if err != nil {
			return nil, err
		}
		bk, users = *b, u
	} else {
		if err := json.Unmarshal(data, &bk); err != nil {
			return nil, fmt.Errorf("not an Uptime Kuma database or backup: %w", err)
		}
		if bk.MonitorList == nil && bk.NotificationList == nil {
			return nil, errors.New("not an Uptime Kuma backup (no monitorList)")
		}
	}
	res := &Result{Secrets: map[string]string{}, Area: o.Area, Users: users}
	names := newNamer(existing)
	have := existingChecks(existing)

	// Monitors first: only channels they use are imported.
	groupName := map[int64]string{}
	for _, m := range bk.MonitorList {
		if m.Type == "group" {
			groupName[m.ID] = m.Name
		}
	}
	type pending struct {
		spec *config.RecordSpec
		set  []int64
	}
	var mons []pending
	used := map[int64]bool{}
	for _, m := range bk.MonitorList {
		if m.Type == "group" {
			continue
		}
		spec, note := kumaMonitorSpec(m, names)
		if spec == nil {
			res.Notes = append(res.Notes, note)
			continue
		}
		key := checkKey(spec.Type, spec.Target, optMap(spec.Opts))
		if spec.Type == "PUSH" {
			key = "PUSH|" + strings.ToLower(m.Name) // nothing to compare but the name
		}
		if prev, dup := have[key]; dup {
			names.release(spec.Name)
			res.Notes = append(res.Notes, fmt.Sprintf("%s: already monitored as %s, skipped", m.Name, prev))
			continue
		}
		if m.Parent != nil && groupName[*m.Parent] != "" {
			spec.Opts = append(spec.Opts, config.Opt{Key: "group", Value: groupName[*m.Parent]})
		}
		if !m.Active {
			spec.Opts = append(spec.Opts, config.Opt{Key: "disabled", Value: "yes"})
		}
		var set []int64
		for id, on := range m.Notifications {
			if n, err := strconv.ParseInt(id, 10, 64); err == nil && on {
				set = append(set, n)
				used[n] = true
			}
		}
		mons = append(mons, pending{spec, set})
		if note != "" {
			res.Notes = append(res.Notes, note)
		}
	}
	if len(mons) == 0 && len(users) == 0 {
		return nil, errors.New("nothing to import: every monitor of this backup already exists here or is not supported")
	}

	// Channels the imported monitors use.
	chanName := map[int64]string{}
	for _, n := range bk.NotificationList {
		if !used[n.ID] {
			continue
		}
		spec, secrets, note := kumaChannel(n, names)
		if spec == nil {
			res.Notes = append(res.Notes, note)
			continue
		}
		for k, v := range secrets {
			res.Secrets[k] = v
		}
		chanName[n.ID] = spec.Name
		res.Records = append(res.Records, *spec)
		res.Channels++
		if note != "" {
			res.Notes = append(res.Notes, note)
		}
	}

	// Policies: one per distinct set of channels.
	policyOf := map[string]string{}
	for _, pm := range mons {
		var set []string
		for _, id := range pm.set {
			if chanName[id] != "" {
				set = append(set, chanName[id])
			}
		}
		sort.Strings(set)
		if len(set) > 0 {
			key := strings.Join(set, ",")
			p, ok := policyOf[key]
			if !ok {
				p = names.take("kuma-policy", "kuma-policy")
				policyOf[key] = p
				res.Records = append(res.Records, config.RecordSpec{Name: p, Type: "POLICY",
					Opts: []config.Opt{{Key: "chain", Value: key}, {Key: "mode", Value: "all"}}, Comment: "from Uptime Kuma"})
			}
			pm.spec.Opts = append(pm.spec.Opts, config.Opt{Key: "policy", Value: p})
		}
		pm.spec.Area = o.Area
		res.Records = append(res.Records, *pm.spec)
		res.Monitors++
	}
	// Shared records (channels, policies) first, then the section.
	res.Text = renderRecords(res.Records)
	return res, nil
}

func kumaMonitorSpec(m kumaMonitor, names *namer) (*config.RecordSpec, string) {
	name := names.take(m.Name, strings.ReplaceAll(m.Type, "json-query", "http"))
	spec := &config.RecordSpec{Name: name, Comment: "from Uptime Kuma"}
	var opts []config.Opt
	add := func(k, v string) {
		if v != "" {
			opts = append(opts, config.Opt{Key: k, Value: v})
		}
	}
	var notes []string
	switch m.Type {
	case "http", "keyword", "json-query":
		spec.Type, spec.Target = "HTTP", m.URL
		if m.Method != "" && !strings.EqualFold(m.Method, "GET") {
			add("method", strings.ToUpper(m.Method))
		}
		add("body", m.Body)
		if len(m.AcceptedStatus) > 0 && !(len(m.AcceptedStatus) == 1 && m.AcceptedStatus[0] == "200-299") {
			add("expect", strings.Join(m.AcceptedStatus, ","))
		}
		if m.Type == "keyword" {
			add("keyword", m.Keyword)
			if m.InvertKeyword {
				add("absent", "yes")
			}
		}
		if m.Type == "json-query" {
			add("json-path", m.JSONPath)
			add("json-expect", m.ExpectedValue)
		}
		if m.IgnoreTLS {
			add("insecure", "yes")
		}
		if m.MaxRedirects != nil && *m.MaxRedirects == 0 {
			add("no-follow", "yes")
		}
		if bool(m.ExpiryNotify) && strings.HasPrefix(m.URL, "https://") {
			add("cert-days", "14")
		}
		if m.BasicUser != "" {
			notes = append(notes, "basic authentication was not imported; add basic= with a secret")
		}
		if m.Headers != "" && m.Headers != "{}" {
			var h map[string]string
			if json.Unmarshal([]byte(m.Headers), &h) == nil {
				keys := make([]string, 0, len(h))
				for k := range h {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if strings.EqualFold(k, "Authorization") {
						notes = append(notes, "an Authorization header was not imported; add it with a secret")
						continue
					}
					add("header."+strings.ToLower(k), h[k])
				}
			}
		}
	case "port":
		spec.Type, spec.Target = "TCP", net.JoinHostPort(m.Hostname, strconv.Itoa(m.Port))
	case "ping":
		spec.Type, spec.Target = "PING", m.Hostname
	case "dns":
		server := m.DNSResolveServer
		if server == "" {
			server = "1.1.1.1"
		}
		spec.Type, spec.Target = "DNS", server
		add("query", m.Hostname)
		if m.DNSResolveType != "" && m.DNSResolveType != "A" {
			add("qtype", m.DNSResolveType)
		}
	case "postgres", "mysql", "redis":
		typ, target, extra, why := fromConnString(m.Type, m.DBConn, m.Hostname, m.Port)
		if why != "" {
			return nil, fmt.Sprintf("%s: %s", m.Name, why)
		}
		spec.Type, spec.Target = typ, target
		for _, kv := range extra {
			add(kv[0], kv[1])
		}
		if strings.Contains(m.DBConn, "@") && strings.Contains(strings.SplitN(m.DBConn, "@", 2)[0], ":") {
			notes = append(notes, "the password was not imported; add password=${secret:NAME}")
		}
	case "docker":
		if m.DockerContainer == "" {
			return nil, fmt.Sprintf("%s: no container name", m.Name)
		}
		spec.Type, spec.Target = "DOCKER", m.DockerContainer
		if m.DockerHost != nil {
			notes = append(notes, "it is checked through the Docker socket of the node that runs it")
		}
	case "grpc-keyword":
		target := strings.TrimPrefix(strings.TrimPrefix(m.GRPCURL, "https://"), "http://")
		if target == "" {
			return nil, fmt.Sprintf("%s: no gRPC address", m.Name)
		}
		spec.Type, spec.Target = "GRPC", target
		add("service", m.GRPCService)
		if m.GRPCTLS {
			add("tls", "yes")
		}
		notes = append(notes, "it uses the standard gRPC health check instead of a keyword match")
	case "push":
		spec.Type = "PUSH"
		add("period", fmt.Sprintf("%ds", max(m.Interval, 20)))
		notes = append(notes, "its push URL changes: point the sender at the new URL shown on the monitor")
	default:
		return nil, fmt.Sprintf("%s: type %q is not supported and was left out", m.Name, m.Type)
	}
	if name != m.Name {
		add("title", m.Name)
	}
	add("description", m.Description)
	if len(m.Tags) > 0 {
		var tags []string
		for _, t := range m.Tags {
			tag := slug(t.Name)
			if tag == "" {
				continue
			}
			if t.Value != "" {
				tag += ":" + strings.ReplaceAll(t.Value, ",", " ")
			}
			tags = append(tags, tag)
		}
		add("tags", strings.Join(tags, ","))
	}
	if m.Type != "push" {
		if m.Interval > 0 {
			spec.Interval = fmt.Sprintf("%ds", m.Interval)
		}
		if m.RetryInterval > 0 && m.RetryInterval != m.Interval {
			add("retry", fmt.Sprintf("%ds", m.RetryInterval))
		}
		add("attempts", strconv.Itoa(m.MaxRetries+1))
		if m.Timeout > 0 {
			add("timeout", fmt.Sprintf("%gs", m.Timeout))
		}
	}
	if m.UpsideDown {
		notes = append(notes, "upside-down mode does not exist here; it was imported as a normal check")
	}
	spec.Opts = opts
	note := ""
	if len(notes) > 0 {
		note = m.Name + ": " + strings.Join(notes, "; ")
	}
	return spec, note
}

// kumaChannel maps a Kuma notification. Credentials go to secrets, the
// record refers to them.
func kumaChannel(n kumaNotification, names *namer) (*config.RecordSpec, map[string]string, string) {
	var c map[string]any
	if err := json.Unmarshal([]byte(n.Config), &c); err != nil {
		return nil, nil, fmt.Sprintf("notification %s: unreadable settings", n.Name)
	}
	str := func(k string) string {
		switch v := c[k].(type) {
		case string:
			return v
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		}
		return ""
	}
	name := names.take(n.Name, "channel")
	secrets := map[string]string{}
	opts := []config.Opt{}
	secret := func(key, value string) {
		if value == "" {
			return
		}
		sn := "kuma_" + strings.ReplaceAll(name, "-", "_") + "_" + strings.ReplaceAll(key, "-", "_")
		secrets[sn] = value
		opts = append(opts, config.Opt{Key: key, Value: "${secret:" + sn + "}"})
	}
	plain := func(key, value string) {
		if value != "" {
			opts = append(opts, config.Opt{Key: key, Value: value})
		}
	}
	kind := ""
	switch str("type") {
	case "telegram":
		kind = "telegram"
		secret("token", str("telegramBotToken"))
		plain("chat", str("telegramChatID"))
	case "slack":
		kind = "slack"
		secret("url", str("slackwebhookURL"))
	case "discord":
		kind = "discord"
		secret("url", str("discordWebhookUrl"))
	case "webhook":
		kind = "webhook"
		secret("url", str("webhookURL"))
	case "feishu":
		kind = "feishu"
		secret("url", str("feishuWebHookUrl"))
	case "DingDing":
		kind = "dingtalk"
		secret("url", str("webHookUrl"))
		secret("secret", str("secretKey"))
	case "WeCom":
		kind = "wecom"
		secret("url", "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key="+str("weComBotKey"))
	case "smtp":
		kind = "email"
		plain("host", str("smtpHost"))
		plain("port", str("smtpPort"))
		plain("username", str("smtpUsername"))
		secret("password", str("smtpPassword"))
		plain("from", str("smtpFrom"))
		plain("to", str("smtpTo"))
		if str("smtpSecure") == "true" {
			plain("tls", "tls")
		}
	default:
		return nil, nil, fmt.Sprintf("notification %s: type %q is not supported and was left out", n.Name, str("type"))
	}
	note := ""
	if !n.Active {
		note = fmt.Sprintf("notification %s was disabled in Uptime Kuma; it is imported as an active channel", n.Name)
	}
	return &config.RecordSpec{Name: name, Type: "CHANNEL", Target: kind, Opts: opts, Comment: "from Uptime Kuma"}, secrets, note
}

// namer turns display names into unique record names.
type namer struct {
	existing *config.Config
	used     map[string]bool
	next     map[string]int // next suffix to try per base name
}

func newNamer(existing *config.Config) *namer {
	return &namer{existing: existing, used: map[string]bool{}, next: map[string]int{}}
}

var slugRe = regexp.MustCompile(`[^a-z0-9_.-]+`)

func slug(s string) string {
	s = slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	return strings.Trim(s, "-.")
}

// take names a record after its display name, or after fallback when the
// display name has nothing usable (e.g. it is written in Chinese).
func (n *namer) take(display, fallback string) string {
	base := slug(display)
	if base == "" || !config.ValidName(base) {
		base = fallback
	}
	if len(base) > 55 {
		base = base[:55]
	}
	name := base
	for i := max(n.next[base], 2); n.used[name] || (n.existing != nil && (n.existing.HasName(name) || name == n.existing.MName)); i++ {
		name = fmt.Sprintf("%s-%d", base, i)
		n.next[base] = i + 1
	}
	n.used[name] = true
	return name
}

func optMap(opts []config.Opt) map[string]string {
	m := map[string]string{}
	for _, o := range opts {
		m[o.Key] = o.Value
	}
	return m
}

// checkKey identifies what a monitor checks, to find ones that exist.
func checkKey(typ, target string, opts map[string]string) string {
	typ = strings.ToUpper(typ)
	switch typ {
	case "HTTP":
		return "HTTP|" + strings.TrimRight(strings.ToLower(target), "/") + "|" + opts["keyword"] + "|" + opts["json-path"]
	case "DNS":
		return "DNS|" + strings.ToLower(target) + "|" + strings.ToLower(opts["query"])
	}
	return typ + "|" + strings.ToLower(target)
}

// existingChecks maps the checks of a configuration to monitor names.
func existingChecks(cfg *config.Config) map[string]string {
	out := map[string]string{}
	if cfg == nil {
		return out
	}
	for _, m := range cfg.Monitors {
		typ, target := m.Written, m.Target
		if m.Written == "DNS" && m.UDP != nil {
			target = m.UDP.Host
		}
		key := checkKey(typ, target, m.Opts)
		if m.Type == "PUSH" {
			key = "PUSH|" + strings.ToLower(m.Title)
		}
		out[key] = m.Name
	}
	return out
}

func (n *namer) release(name string) { delete(n.used, name) }

// fromConnString turns Kuma's database connection string into a service
// check: the address, user and database, never the password.
func fromConnString(kind, conn, host string, port int) (typ, target string, extra [][2]string, why string) {
	typ = map[string]string{"postgres": "POSTGRES", "mysql": "MYSQL", "redis": "REDIS"}[kind]
	if conn == "" {
		if host == "" {
			return "", "", nil, "no connection string"
		}
		target = host
		if port > 0 {
			target = net.JoinHostPort(host, strconv.Itoa(port))
		}
		return typ, target, nil, ""
	}
	u, err := url.Parse(conn)
	if err != nil || u.Host == "" {
		return "", "", nil, "connection string not understood"
	}
	target = u.Host
	if u.User != nil && u.User.Username() != "" {
		extra = append(extra, [2]string{"user", u.User.Username()})
	}
	if db := strings.Trim(u.Path, "/"); db != "" && kind != "redis" {
		extra = append(extra, [2]string{"database", db})
	}
	switch {
	case u.Scheme == "rediss":
		extra = append(extra, [2]string{"tls", "yes"})
	case kind == "postgres" && u.Query().Get("sslmode") != "" && u.Query().Get("sslmode") != "disable":
		extra = append(extra, [2]string{"ssl", "yes"})
	}
	return typ, target, extra, ""
}
