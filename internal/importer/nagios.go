package importer

import (
	"errors"
	"fmt"
	"net"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/githubflyideas/unagikuma/internal/config"
)

// Nagios converts Nagios (or Icinga 1, Naemon) object definitions into
// monitors.bind records: hosts become PING checks, services the matching
// check (HTTP, TCP, DNS, databases) or an EXEC check that runs the same
// plugin, contacts e-mail channels, contact groups group channels and
// time periods PERIOD records. Templates (use=, register 0) are resolved.
// data is the object configuration; several files may simply be joined.
func Nagios(data []byte, existing *config.Config, o Options) (*Result, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("the configuration is not UTF-8 text")
	}
	objs, err := parseNagios(string(data))
	if err != nil {
		return nil, err
	}
	n := newNagios(objs)
	if len(n.hosts) == 0 {
		return nil, errors.New("no host definitions found: is this a Nagios object configuration?")
	}
	res := &Result{Secrets: map[string]string{}, Area: o.Area}
	n.names, n.have, n.res = newNamer(existing), existingChecks(existing), res
	n.convert(o.Area)
	if res.Monitors == 0 {
		return nil, errors.New("nothing to import: every check already exists here or is not supported")
	}
	res.Text = renderRecords(res.Records)
	return res, nil
}

// ---- object parser ----

type nagObj struct {
	typ  string
	vals map[string]string
	line int
}

func parseNagios(s string) ([]*nagObj, error) {
	var out []*nagObj
	var cur *nagObj
	for i, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if c := strings.Index(line, ";"); c >= 0 && (c == 0 || line[c-1] != '\\') {
			line = strings.TrimSpace(line[:c])
		}
		line = strings.ReplaceAll(line, `\;`, ";")
		if line == "" || line[0] == '#' {
			continue
		}
		if cur == nil {
			rest, ok := strings.CutPrefix(line, "define")
			if !ok {
				continue // cfg_file= and other main-config lines
			}
			typ := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "{"))
			if typ == "" || !strings.HasSuffix(line, "{") {
				return nil, fmt.Errorf("line %d: expected \"define TYPE {\"", i+1)
			}
			cur = &nagObj{typ: typ, vals: map[string]string{}, line: i + 1}
			continue
		}
		closing := strings.HasSuffix(line, "}")
		line = strings.TrimSpace(strings.TrimSuffix(line, "}"))
		if line != "" {
			k, v, _ := strings.Cut(strings.Join(strings.Fields(line), " "), " ")
			cur.vals[k] = strings.TrimSpace(v)
		}
		if closing {
			out = append(out, cur)
			cur = nil
		}
	}
	if cur != nil {
		return nil, fmt.Errorf("line %d: define %s is not closed", cur.line, cur.typ)
	}
	return out, nil
}

type nagios struct {
	templates  map[string]map[string]*nagObj // type → name → template
	resolved   map[string]map[string]string
	members    map[string][]string // hostgroup → hosts
	groupsOf   map[string][]string // host → hostgroups
	hosts      []*nagObj
	services   []*nagObj
	hostByName map[string]*nagObj
	groups     map[string]*nagObj // hostgroups
	commands   map[string]string
	contacts   map[string]*nagObj
	cgroups    map[string]*nagObj
	periods    map[string]*nagObj

	names *namer
	have  map[string]string
	res   *Result

	hostMon   map[string]string // host_name → monitor name
	channelOf map[string]string // "c:NAME" or "g:NAME" → channel name
	periodRec map[string]string // time period → PERIOD name ("" = always)
	policyOf  map[string]string
	notes     map[string]bool

	serviceGroups bool
	skipped       []string // object types left out
}

func newNagios(objs []*nagObj) *nagios {
	n := &nagios{templates: map[string]map[string]*nagObj{}, resolved: map[string]map[string]string{}, hostByName: map[string]*nagObj{}, groups: map[string]*nagObj{},
		commands: map[string]string{}, contacts: map[string]*nagObj{}, cgroups: map[string]*nagObj{}, periods: map[string]*nagObj{},
		hostMon: map[string]string{}, channelOf: map[string]string{}, periodRec: map[string]string{}, policyOf: map[string]string{},
		notes: map[string]bool{}}
	for _, o := range objs {
		if name := o.vals["name"]; name != "" {
			if n.templates[o.typ] == nil {
				n.templates[o.typ] = map[string]*nagObj{}
			}
			n.templates[o.typ][name] = o
		}
	}
	for _, o := range objs {
		if o.vals["register"] == "0" {
			continue
		}
		v := n.resolve(o)
		switch o.typ {
		case "host":
			if v["host_name"] != "" {
				r := &nagObj{typ: "host", vals: v, line: o.line}
				n.hosts = append(n.hosts, r)
				n.hostByName[v["host_name"]] = r
			}
		case "service":
			n.services = append(n.services, &nagObj{typ: "service", vals: v, line: o.line})
		case "hostgroup":
			n.groups[v["hostgroup_name"]] = &nagObj{vals: v}
		case "command":
			n.commands[v["command_name"]] = v["command_line"]
		case "contact":
			n.contacts[v["contact_name"]] = &nagObj{vals: v}
		case "contactgroup":
			n.cgroups[v["contactgroup_name"]] = &nagObj{vals: v}
		case "timeperiod":
			n.periods[v["timeperiod_name"]] = &nagObj{vals: v}
		case "servicegroup":
			n.serviceGroups = true
		default:
			if !slices.Contains(n.skipped, o.typ) {
				n.skipped = append(n.skipped, o.typ)
			}
		}
	}
	return n
}

// resolve merges an object with its templates: its own values win, then
// the templates in the order use= lists them; "+value" appends to the
// inherited value and "null" clears it.
func (n *nagios) resolve(o *nagObj) map[string]string {
	out := map[string]string{}
	for _, t := range splitList(o.vals["use"]) {
		for k, v := range n.template(o.typ, t) {
			if _, ok := out[k]; !ok {
				out[k] = v
			}
		}
	}
	for k, v := range o.vals {
		switch {
		case k == "use" || k == "name" || k == "register":
		case strings.HasPrefix(v, "+"):
			if out[k] != "" {
				out[k] += "," + v[1:]
			} else {
				out[k] = v[1:]
			}
		case v == "null":
			delete(out, k)
		default:
			out[k] = v
		}
	}
	return out
}

// template returns a resolved template, once; a template that uses
// itself resolves to nothing the second time round.
func (n *nagios) template(typ, name string) map[string]string {
	key := typ + "\x00" + name
	if v, ok := n.resolved[key]; ok {
		return v
	}
	n.resolved[key] = nil
	tpl := n.templates[typ][name]
	if tpl == nil {
		return nil
	}
	v := n.resolve(tpl)
	n.resolved[key] = v
	return v
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// ---- conversion ----

func (n *nagios) note(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if !n.notes[s] {
		n.notes[s] = true
		n.res.Notes = append(n.res.Notes, s)
	}
}

// hostsOf expands a service's host_name and hostgroup_name lists ("*"
// is every host, "!x" excludes x).
func (n *nagios) hostsOf(v map[string]string) []string {
	in, out := map[string]bool{}, map[string]bool{}
	add := func(name string) {
		if x, ok := strings.CutPrefix(name, "!"); ok {
			out[x] = true
		} else {
			in[name] = true
		}
	}
	for _, h := range splitList(v["host_name"]) {
		if h == "*" {
			for name := range n.hostByName {
				in[name] = true
			}
			continue
		}
		add(h)
	}
	for _, g := range splitList(v["hostgroup_name"]) {
		neg := strings.HasPrefix(g, "!")
		for _, h := range n.groupMembers(strings.TrimPrefix(g, "!")) {
			if neg {
				out[h] = true
			} else {
				in[h] = true
			}
		}
	}
	var hosts []string
	for h := range in {
		if !out[h] {
			hosts = append(hosts, h)
		}
	}
	sort.Strings(hosts)
	return hosts
}

// groupMembers returns a host group's hosts, including nested groups
// and hosts naming the group in hostgroups=.
func (n *nagios) groupMembers(group string) []string {
	if n.members == nil {
		n.members = map[string][]string{}
		direct := map[string][]string{}
		for _, h := range n.hosts {
			for _, hg := range splitList(h.vals["hostgroups"]) {
				direct[hg] = append(direct[hg], h.vals["host_name"])
			}
		}
		for name := range n.groups {
			seen, hosts := map[string]bool{}, map[string]bool{}
			var walk func(string)
			walk = func(g string) {
				if seen[g] {
					return
				}
				seen[g] = true
				for _, h := range direct[g] {
					hosts[h] = true
				}
				if gr := n.groups[g]; gr != nil {
					for _, h := range splitList(gr.vals["members"]) {
						hosts[h] = true
					}
					for _, sub := range splitList(gr.vals["hostgroup_members"]) {
						walk(sub)
					}
				}
			}
			walk(name)
			list := make([]string, 0, len(hosts))
			for h := range hosts {
				list = append(list, h)
			}
			sort.Strings(list)
			n.members[name] = list
		}
	}
	return n.members[group]
}

func (n *nagios) hostGroupsOf(host string) []string {
	if n.groupsOf == nil {
		n.groupsOf = map[string][]string{}
		for name := range n.groups {
			for _, h := range n.groupMembers(name) {
				n.groupsOf[h] = append(n.groupsOf[h], name)
			}
		}
		for _, gs := range n.groupsOf {
			sort.Strings(gs)
		}
	}
	return n.groupsOf[host]
}

// maxImported bounds one import (services times hosts can be large).
const maxImported = 20000

func (n *nagios) convert(area string) {
	type mon struct {
		spec   *config.RecordSpec
		policy string
	}
	var mons []mon
	// Hosts first: services name them as parents.
	for _, h := range n.hosts {
		v := h.vals
		spec, why := n.hostSpec(v)
		if spec == nil {
			n.note("host %s: %s", v["host_name"], why)
			continue
		}
		mons = append(mons, mon{spec, n.policy(v, "host")})
	}
	for _, h := range n.hosts {
		if name := n.hostMon[h.vals["host_name"]]; name != "" {
			var parents []string
			for _, p := range splitList(h.vals["parents"]) {
				if pm := n.hostMon[p]; pm != "" {
					parents = append(parents, pm)
				}
			}
			if len(parents) > 0 {
				for _, m := range mons {
					if m.spec.Name == name {
						m.spec.Opts = append(m.spec.Opts, config.Opt{Key: "parent", Value: strings.Join(parents, ",")})
					}
				}
			}
		}
	}
	for _, s := range n.services {
		if len(mons) >= maxImported {
			n.note("only the first %d checks were imported", maxImported)
			break
		}
		hosts := n.hostsOf(s.vals)
		if len(hosts) == 0 {
			n.note("service %q (line %d) applies to no known host", s.vals["service_description"], s.line)
		}
		for _, host := range hosts {
			h := n.hostByName[host]
			if h == nil {
				n.note("service %q: host %s is not defined", s.vals["service_description"], host)
				continue
			}
			v := map[string]string{}
			for k, x := range s.vals {
				v[k] = x
			}
			// Services without contacts inherit the host's.
			if v["contacts"] == "" && v["contact_groups"] == "" {
				v["contacts"], v["contact_groups"] = h.vals["contacts"], h.vals["contact_groups"]
			}
			spec, why := n.serviceSpec(v, h.vals)
			if spec == nil {
				n.note("%s / %s: %s", host, v["service_description"], why)
				continue
			}
			mons = append(mons, mon{spec, n.policy(v, "service")})
		}
	}
	for _, m := range mons {
		if m.policy != "" {
			m.spec.Opts = append(m.spec.Opts, config.Opt{Key: "policy", Value: m.policy})
		}
		m.spec.Area = area
		n.res.Records = append(n.res.Records, *m.spec)
		n.res.Monitors++
	}
	if n.serviceGroups {
		n.note("service groups were not imported; use tags= to group services")
	}
	for _, t := range n.skipped {
		n.note("%s definitions are not imported", t)
	}
}

// common sets what hosts and services share: interval, attempts, title,
// group, the event handler and disabled checks.
func (n *nagios) common(spec *config.RecordSpec, v, host map[string]string, display string) {
	add := func(k, val string) {
		if val != "" {
			spec.Opts = append(spec.Opts, config.Opt{Key: k, Value: val})
		}
	}
	unit := 60.0
	if f, err := strconv.ParseFloat(v["interval_length"], 64); err == nil && f > 0 {
		unit = f
	}
	minutes := func(k string) string {
		f, err := strconv.ParseFloat(v[k], 64)
		if err != nil || f <= 0 {
			return ""
		}
		return strconv.Itoa(max(int(f*unit), 10)) + "s"
	}
	if iv := minutes("check_interval"); iv != "" {
		spec.Interval = iv
	} else if iv := minutes("normal_check_interval"); iv != "" {
		spec.Interval = iv
	}
	if r := minutes("retry_interval"); r != "" && r != spec.Interval {
		add("retry", r)
	} else if r := minutes("retry_check_interval"); r != "" && r != spec.Interval {
		add("retry", r)
	}
	if a, err := strconv.Atoi(v["max_check_attempts"]); err == nil && a > 0 {
		add("attempts", strconv.Itoa(min(a, 100)))
	}
	if spec.Name != display {
		add("title", truncate(display, 64))
	}
	add("device", host["host_name"])
	if gs := n.hostGroupsOf(host["host_name"]); len(gs) > 0 {
		add("group", gs[0])
	}
	add("description", strings.TrimSpace(v["notes"]+" "+v["notes_url"]))
	if v["active_checks_enabled"] == "0" {
		if v["passive_checks_enabled"] != "0" {
			n.note("%s: passive checks are not imported; it was imported disabled", display)
		}
		add("disabled", "yes")
	}
	if h := v["event_handler"]; h != "" && v["event_handler_enabled"] != "0" {
		if cmd, why := n.command(h, host, true); cmd != "" {
			add("handler", cmd)
			n.note("event handler %s: copy %s into the check-scripts directory", h, strings.Fields(cmd)[0])
		} else {
			n.note("%s: event handler %s left out: %s", display, h, why)
		}
	}
	spec.Comment = "from Nagios"
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (n *nagios) hostSpec(v map[string]string) (*config.RecordSpec, string) {
	host := v["host_name"]
	addr := v["address"]
	if addr == "" {
		addr = host
	}
	spec := &config.RecordSpec{Type: "PING", Target: addr}
	if cmd := v["check_command"]; cmd != "" && !isPingCommand(cmd, n.commands) {
		typ, target, opts, why := n.check(cmd, v)
		if typ == "" {
			n.note("host %s: check %s replaced by ping (%s)", host, cmd, why)
		} else {
			spec.Type, spec.Target, spec.Opts = typ, target, opts
		}
	}
	key := checkKey(spec.Type, spec.Target, optMap(spec.Opts))
	if prev, dup := n.have[key]; dup {
		n.hostMon[host] = prev // services still depend on it
		return nil, "already monitored as " + prev + ", skipped"
	}
	spec.Name = n.names.take(host, "host")
	n.hostMon[host] = spec.Name
	n.have[key] = spec.Name
	display := host
	if a := v["alias"]; a != "" && a != host {
		display = a
	}
	n.common(spec, v, v, display)
	return spec, ""
}

func isPingCommand(cmd string, commands map[string]string) bool {
	name, _, _ := strings.Cut(cmd, "!")
	if strings.Contains(name, "host-alive") || strings.Contains(name, "ping") {
		return true
	}
	words := strings.Fields(commands[name])
	return len(words) > 0 && (path.Base(words[0]) == "check_ping" || path.Base(words[0]) == "check_icmp")
}

func (n *nagios) serviceSpec(v, host map[string]string) (*config.RecordSpec, string) {
	desc := v["service_description"]
	if desc == "" {
		return nil, "no service_description"
	}
	typ, target, opts, why := n.check(v["check_command"], host)
	if typ == "" {
		return nil, why
	}
	spec := &config.RecordSpec{Type: typ, Target: target, Opts: opts}
	key := checkKey(typ, target, optMap(opts))
	if prev, dup := n.have[key]; dup {
		return nil, "already monitored as " + prev + ", skipped"
	}
	spec.Name = n.names.take(host["host_name"]+"-"+desc, "service")
	n.have[key] = spec.Name
	n.common(spec, v, host, host["host_name"]+" / "+desc)
	if p := n.hostMon[host["host_name"]]; p != "" && p != spec.Name {
		// A service is unreachable, not failed, while its host is down.
		spec.Opts = append(spec.Opts, config.Opt{Key: "parent", Value: p})
	}
	return spec, ""
}

var macroRe = regexp.MustCompile(`\$[A-Z0-9_]+\$`)

// command resolves "name!arg1!arg2" into the plugin's command line with
// $ARGn$, $USERn$ (a path: dropped) and the host macros substituted. For
// event handlers the state macros are kept: they are filled in when it
// runs.
func (n *nagios) command(call string, host map[string]string, handler bool) (string, string) {
	parts := strings.Split(call, "!")
	line, ok := n.commands[parts[0]]
	if !ok {
		return "", "command " + parts[0] + " is not defined"
	}
	addr := host["address"]
	if addr == "" {
		addr = host["host_name"]
	}
	var unknown []string
	line = macroRe.ReplaceAllStringFunc(line, func(m string) string {
		name := m[1 : len(m)-1]
		switch {
		case strings.HasPrefix(name, "ARG"):
			if i, err := strconv.Atoi(name[3:]); err == nil && i < len(parts) {
				return parts[i]
			}
			return ""
		case strings.HasPrefix(name, "USER"):
			return "" // $USER1$ is the plugin directory
		case name == "HOSTADDRESS":
			return addr
		case name == "HOSTNAME":
			return host["host_name"]
		case name == "HOSTALIAS":
			return host["alias"]
		case handler && (name == "SERVICESTATE" || name == "SERVICESTATETYPE" || name == "SERVICEATTEMPT" || name == "SERVICEDESC" ||
			name == "HOSTSTATE" || name == "HOSTSTATETYPE" || name == "HOSTATTEMPT"):
			return "$" + strings.Replace(name, "HOST", "SERVICE", 1) + "$"
		}
		unknown = append(unknown, m)
		return m
	})
	if len(unknown) > 0 {
		return "", "macros " + strings.Join(unknown, " ") + " are not supported"
	}
	words := shellWords(line)
	if len(words) == 0 {
		return "", "empty command line"
	}
	words[0] = path.Base(words[0])
	for i, w := range words {
		if i > 0 && (w == "" || strings.ContainsAny(w, " \t'\"\\")) {
			words[i] = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
		}
	}
	return strings.Join(words, " "), ""
}

// shellWords splits a command line the way Nagios's shell would, without
// expanding anything.
func shellWords(s string) []string {
	var out []string
	var cur strings.Builder
	in, q := false, byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q != 0 && c == q:
			q = 0
		case q == '\'':
			cur.WriteByte(c)
		case c == '\\' && i+1 < len(s) && q != '\'':
			i++
			cur.WriteByte(s[i])
			in = true
		case q == 0 && (c == '\'' || c == '"'):
			q, in = c, true
		case q == 0 && (c == ' ' || c == '\t'):
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}

// flags reads a plugin's short and long options ("-H x", "--port=80").
func flags(words []string, withValue string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(words); i++ {
		w := words[i]
		if !strings.HasPrefix(w, "-") {
			continue
		}
		k, v, hasEq := strings.Cut(strings.TrimLeft(w, "-"), "=")
		if !strings.HasPrefix(w, "--") && len(k) > 1 && strings.Contains(withValue, " "+k[:1]+" ") {
			k, v, hasEq = k[:1], k[1:], true // -H10.0.0.1
		}
		if !hasEq && strings.Contains(withValue, " "+k+" ") && i+1 < len(words) {
			i++
			v = words[i]
		}
		out[k] = v
	}
	return out
}

func pick(f map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := f[k]; ok {
			return v
		}
	}
	return ""
}

var plainPorts = map[string]string{"check_ssh": "22", "check_smtp": "25", "check_ftp": "21", "check_imap": "143", "check_pop": "110",
	"check_simap": "993", "check_spop": "995", "check_ssmtp": "465", "check_ldap": "389", "check_ldaps": "636", "check_jabber": "5222",
	"check_nntp": "119", "check_clamd": "3310"}

// check maps a check command to a monitor type, target and options.
func (n *nagios) check(call string, host map[string]string) (typ, target string, opts []config.Opt, why string) {
	if call == "" {
		return "", "", nil, "no check_command"
	}
	line, why := n.command(call, host, false)
	if line == "" {
		return "", "", nil, why
	}
	words := shellWords(line)
	plugin, args := words[0], words[1:]
	addr := host["address"]
	if addr == "" {
		addr = host["host_name"]
	}
	add := func(k, v string) {
		if v != "" {
			opts = append(opts, config.Opt{Key: k, Value: v})
		}
	}
	const valued = " H I u p s e j w c t C a A k P l d D 4 6 r R T b f m M N o O "
	f := flags(args, valued+"hostname IPaddress url uri port string expect method warning critical timeout certificate expect-address query-address server database logname username user password ")
	h := pick(f, "I", "IPaddress", "H", "hostname")
	if h == "" {
		h = addr
	}
	switch plugin {
	case "check_ping", "check_icmp", "check_fping":
		return "PING", h, nil, ""
	case "check_http", "check_curl":
		vhost := pick(f, "H", "hostname")
		if vhost == "" {
			vhost = h
		}
		scheme, port := "http", pick(f, "p", "port")
		if _, ok := f["S"]; ok {
			scheme = "https"
		}
		if _, ok := f["ssl"]; ok {
			scheme = "https"
		}
		hostport := vhost
		if port != "" && !(scheme == "http" && port == "80") && !(scheme == "https" && port == "443") {
			hostport = net.JoinHostPort(vhost, port)
		} else if strings.Contains(vhost, ":") {
			hostport = "[" + vhost + "]"
		}
		uri := pick(f, "u", "url", "uri")
		if !strings.HasPrefix(uri, "/") {
			uri = "/" + uri
		}
		add("keyword", pick(f, "s", "string"))
		if e := pick(f, "e", "expect"); e != "" {
			var codes []string
			for _, x := range strings.Split(e, ",") {
				if c := strings.TrimSpace(x); len(c) >= 3 && c[0] >= '1' && c[0] <= '5' {
					codes = append(codes, c[:3])
				} else if c, ok := strings.CutPrefix(c, "HTTP/1.1 "); ok && len(c) >= 3 {
					codes = append(codes, c[:3])
				}
			}
			add("expect", strings.Join(codes, ","))
		}
		if m := pick(f, "j", "method"); m != "" && !strings.EqualFold(m, "GET") {
			add("method", strings.ToUpper(m))
		}
		if c := pick(f, "C", "certificate"); c != "" {
			days, crit, _ := strings.Cut(c, ",")
			add("cert-days", days)
			add("cert-critical-days", crit)
		}
		if pick(f, "a") != "" {
			n.note("check_http -a (basic authentication) was not imported; add basic= with a secret")
		}
		return "HTTP", scheme + "://" + hostport + uri, opts, ""
	case "check_tcp":
		port := pick(f, "p", "port")
		if port == "" {
			return "", "", nil, "check_tcp without -p"
		}
		return "TCP", net.JoinHostPort(h, port), opts, ""
	case "check_dns":
		query := pick(f, "H", "hostname")
		server := pick(f, "s", "server")
		if server == "" {
			return "", "", nil, "check_dns without -s uses the system resolver; add a DNS check with the server to ask"
		}
		add("query", query)
		add("expect", strings.ReplaceAll(pick(f, "a", "expect-address"), " ", ""))
		return "DNS", server, opts, ""
	case "check_ntp", "check_ntp_time", "check_ntp_peer":
		return "NTP", h, nil, ""
	case "check_mysql":
		port := pick(f, "P", "port")
		if port == "" {
			port = "3306"
		}
		add("user", pick(f, "u", "username", "user"))
		add("database", pick(f, "d", "database"))
		if pick(f, "p", "password") != "" {
			n.note("database passwords were not imported; add password=${secret:NAME}")
		}
		return "MYSQL", net.JoinHostPort(h, port), opts, ""
	case "check_pgsql":
		port := pick(f, "P", "port")
		if port == "" {
			port = "5432"
		}
		add("user", pick(f, "l", "logname"))
		add("database", pick(f, "d", "database"))
		if pick(f, "p", "password") != "" {
			n.note("database passwords were not imported; add password=${secret:NAME}")
		}
		return "POSTGRES", net.JoinHostPort(h, port), opts, ""
	}
	if port, ok := plainPorts[plugin]; ok {
		if p := pick(f, "p", "port"); p != "" {
			port = p
		}
		return "TCP", net.JoinHostPort(h, port), nil, ""
	}
	// Anything else runs the same plugin.
	n.note("EXEC checks run plugins from the check-scripts directory: copy %s there", plugin)
	add("address", addr)
	return "EXEC", line, opts, ""
}

// ---- notification: contacts, groups, periods, policies ----

// policy returns the policy for an object's contacts and notification
// settings, creating channels, groups, periods and the policy as needed.
func (n *nagios) policy(v map[string]string, kind string) string {
	var chain []string
	seen := map[string]bool{}
	for _, c := range splitList(v["contacts"]) {
		if ch := n.contactChannel(c, kind); ch != "" && !seen[ch] {
			seen[ch] = true
			chain = append(chain, ch)
		}
	}
	for _, g := range splitList(v["contact_groups"]) {
		if ch := n.groupChannel(g, kind); ch != "" && !seen[ch] {
			seen[ch] = true
			chain = append(chain, ch)
		}
	}
	if len(chain) == 0 || v["notifications_enabled"] == "0" {
		return ""
	}
	sort.Strings(chain)
	var opts []config.Opt
	opts = append(opts, config.Opt{Key: "chain", Value: strings.Join(chain, ",")}, config.Opt{Key: "mode", Value: "all"})
	unit := 60.0
	if f, err := strconv.ParseFloat(v["interval_length"], 64); err == nil && f > 0 {
		unit = f
	}
	if f, err := strconv.ParseFloat(v["notification_interval"], 64); err == nil {
		if f == 0 {
			opts = append(opts, config.Opt{Key: "max", Value: "1"})
		} else if s := int(f * unit); s >= 60 {
			opts = append(opts, config.Opt{Key: "repeat", Value: strconv.Itoa(s) + "s"})
		}
	}
	if f, err := strconv.ParseFloat(v["first_notification_delay"], 64); err == nil && f > 0 {
		opts = append(opts, config.Opt{Key: "delay", Value: strconv.Itoa(int(f*unit)) + "s"})
	}
	if no := v["notification_options"]; no != "" && !strings.Contains(no, "r") {
		opts = append(opts, config.Opt{Key: "recovery", Value: "no"})
	}
	var key strings.Builder
	for _, o := range opts {
		key.WriteString(o.Key + "=" + o.Value + " ")
	}
	if p, ok := n.policyOf[key.String()]; ok {
		return p
	}
	p := n.names.take("nagios-policy", "nagios-policy")
	n.policyOf[key.String()] = p
	n.res.Records = append(n.res.Records, config.RecordSpec{Name: p, Type: "POLICY", Opts: opts, Comment: "from Nagios"})
	return p
}

func (n *nagios) contactChannel(name, kind string) string {
	if ch, ok := n.channelOf["c:"+name]; ok {
		return ch
	}
	n.channelOf["c:"+name] = ""
	c := n.contacts[name]
	if c == nil {
		n.note("contact %s is not defined", name)
		return ""
	}
	v := c.vals
	if v[kind+"_notifications_enabled"] == "0" {
		return ""
	}
	if v["email"] == "" {
		n.note("contact %s has no e-mail address and was left out", name)
		return ""
	}
	if v["pager"] != "" {
		n.note("contact %s: the pager number was not imported; add an SMS or voice channel", name)
	}
	ch := n.names.take(name, "contact")
	opts := []config.Opt{{Key: "host", Value: "localhost"}, {Key: "port", Value: "25"}, {Key: "tls", Value: "none"},
		{Key: "from", Value: "unagikuma@localhost"}, {Key: "to", Value: v["email"]}}
	if p := n.period(v["service_notification_period"]); p != "" {
		opts = append(opts, config.Opt{Key: "period", Value: p})
	}
	n.note("e-mail channels send through the local mail server (localhost:25) like Nagios; change host= and from= if needed")
	n.res.Records = append(n.res.Records, config.RecordSpec{Name: ch, Type: "CHANNEL", Target: "email", Opts: opts, Comment: "from Nagios contact " + name})
	n.res.Channels++
	n.channelOf["c:"+name] = ch
	return ch
}

func (n *nagios) groupChannel(name, kind string) string {
	if ch, ok := n.channelOf["g:"+name]; ok {
		return ch
	}
	n.channelOf["g:"+name] = ""
	g := n.cgroups[name]
	if g == nil {
		n.note("contact group %s is not defined", name)
		return ""
	}
	var members []string
	seen := map[string]bool{}
	visited := map[string]bool{}
	var walk func(string)
	walk = func(group string) {
		cg := n.cgroups[group]
		if cg == nil || visited[group] {
			return
		}
		visited[group] = true
		for _, c := range splitList(cg.vals["members"]) {
			if ch := n.contactChannel(c, kind); ch != "" && !seen[ch] {
				seen[ch] = true
				members = append(members, ch)
			}
		}
		for _, sub := range splitList(cg.vals["contactgroup_members"]) {
			walk(sub)
		}
	}
	walk(name)
	// Contacts naming the group in their contactgroups= are members too.
	var extra []string
	for cname, c := range n.contacts {
		for _, cg := range splitList(c.vals["contactgroups"]) {
			if cg == name {
				extra = append(extra, cname)
			}
		}
	}
	sort.Strings(extra)
	for _, c := range extra {
		if ch := n.contactChannel(c, kind); ch != "" && !seen[ch] {
			seen[ch] = true
			members = append(members, ch)
		}
	}
	if len(members) == 0 {
		n.note("contact group %s has no usable members", name)
		return ""
	}
	if len(members) == 1 {
		n.channelOf["g:"+name] = members[0]
		return members[0]
	}
	ch := n.names.take(name, "contacts")
	n.res.Records = append(n.res.Records, config.RecordSpec{Name: ch, Type: "CHANNEL", Target: "group",
		Opts: []config.Opt{{Key: "members", Value: strings.Join(members, ",")}}, Comment: "from Nagios contact group " + name})
	n.res.Channels++
	n.channelOf["g:"+name] = ch
	return ch
}

var nagDays = []struct{ long, short string }{{"monday", "mon"}, {"tuesday", "tue"}, {"wednesday", "wed"}, {"thursday", "thu"},
	{"friday", "fri"}, {"saturday", "sat"}, {"sunday", "sun"}}

// period converts a time period into a PERIOD record and returns its name
// ("" when it is always on or cannot be converted).
func (n *nagios) period(name string) string {
	if name == "" {
		return ""
	}
	if p, ok := n.periodRec[name]; ok {
		return p
	}
	n.periodRec[name] = ""
	tp := n.periods[name]
	if tp == nil {
		n.note("time period %s is not defined; the channel is always used", name)
		return ""
	}
	var rules []string
	always := true
	for _, d := range nagDays {
		ranges := tp.vals[d.long]
		if ranges == "" {
			always = false
			continue
		}
		for _, r := range splitList(ranges) {
			if r != "00:00-24:00" {
				always = false
			}
			rules = append(rules, d.short+" "+r)
		}
	}
	for k := range tp.vals {
		known := k == "timeperiod_name" || k == "alias"
		for _, d := range nagDays {
			known = known || k == d.long
		}
		if !known {
			n.note("time period %s: %q (dates and exclusions) was not imported", name, k)
		}
	}
	if always {
		return ""
	}
	if len(rules) == 0 {
		n.note("time period %s has no weekly ranges; the channel is always used", name)
		return ""
	}
	rec := n.names.take(name, "period")
	n.res.Records = append(n.res.Records, config.RecordSpec{Name: rec, Type: "PERIOD", Target: strings.Join(rules, "; "), Comment: "from Nagios"})
	n.periodRec[name] = rec
	return rec
}

// renderRecords writes shared records first, then each $SCOPE section.
func renderRecords(recs []config.RecordSpec) string {
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Area == "" && recs[j].Area != "" })
	var b strings.Builder
	area := ""
	for _, r := range recs {
		if r.Area != area {
			area = r.Area
			b.WriteString("$SCOPE " + area + "\n")
		}
		b.WriteString(r.Render() + "\n")
	}
	return b.String()
}
