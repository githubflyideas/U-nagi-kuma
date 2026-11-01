package app

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/config"
)

// ZoneExport contains portable configuration, never local trust or credentials.
// Missing uses qualified references (secret:NAME or env:NAME). A nil resolver
// reports every dependency; it never resolves or reads a secret value.
type ZoneExport struct {
	Text    string   `json:"text"`
	Missing []string `json:"missing"`
}

var zoneExportRef = regexp.MustCompile(`\$\{(secret|env):([A-Za-z0-9_.-]+)\}`)

// Export fails closed for opaque targets and unsupported directives. In
// particular, a URL path can be a credential even without a known query key.
// Unknown option values become secret references rather than being guessed safe.
func (s *ZoneStore) Export(id string, known func(string) bool) (ZoneExport, error) {
	z, err := s.Get(id)
	if err != nil {
		return ZoneExport{}, err
	}
	f, ds := config.Parse("export.bind", []byte(z.Body))
	if config.HasErrors(ds) {
		return ZoneExport{}, fmt.Errorf("invalid export source")
	}
	missing := map[string]bool{}
	refs := func(value string) {
		for _, m := range zoneExportRef.FindAllStringSubmatch(value, -1) {
			ref := m[1] + ":" + m[2]
			if known == nil || !known(ref) {
				missing[ref] = true
			}
		}
	}
	// These options are structural configuration, not arbitrary payload strings.
	safe := map[string]bool{}
	for _, k := range strings.Fields("interval retry timeout attempts recover-attempts degraded quorum on-insufficient on-filtered on-node-loss scope policy partial-escalate disabled grace parent nodes channels repeat max recover") {
		safe[k] = true
	}
	// Reserve all existing references before generating replacements, so a new
	// placeholder cannot accidentally select an unrelated existing credential.
	reserved := map[string]bool{}
	for _, m := range zoneExportRef.FindAllStringSubmatch(z.Body, -1) {
		reserved[m[1]+":"+m[2]] = true
	}
	counter := 0
	placeholder := func() string {
		for {
			counter++
			ref := fmt.Sprintf("secret:export-%s-%d", id, counter)
			if !reserved[ref] {
				reserved[ref] = true
				missing[ref] = true
				return "${" + ref + "}"
			}
		}
	}
	var out strings.Builder
	for i := range f.Stmts {
		stmt := &f.Stmts[i]
		r, ok := config.StmtRecord(stmt)
		if !ok {
			return ZoneExport{}, fmt.Errorf("export requires unsupported directives to be removed explicitly")
		}
		if r.Type == "SOA" {
			for j, t := range stmt.Tokens {
				if j > 0 {
					out.WriteByte(' ')
				}
				out.WriteString(t.Raw)
			}
			out.WriteByte('\n')
			fmt.Fprintf(&out, "$ZONE zone_id=%s name=%s generation=%d", z.Header.ZoneID, z.Header.Name, z.Header.Generation)
			if z.Header.SourceNode != "" {
				fmt.Fprintf(&out, " source_node=%s", z.Header.SourceNode)
			}
			out.WriteByte('\n')
			continue
		}
		if r.TargetOK {
			switch r.Type {
			case "CHANNEL": // Validated channel kind, not a credential.
			case "HTTP", "HTTPS":
				u, e := url.Parse(r.Target)
				if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" {
					return ZoneExport{}, fmt.Errorf("export refuses opaque monitor target; use a reviewed portable configuration")
				}
			default:
				return ZoneExport{}, fmt.Errorf("export refuses unsupported target type")
			}
		}
		opts := append([]config.Opt(nil), r.Opts...)
		for j := range opts {
			o := &opts[j]
			if !safe[o.Key] && o.Value != "" {
				if zoneExportRef.MatchString(o.Value) {
					// Partial references can hide a second plaintext credential. Do not
					// silently drop the reference or export the surrounding literal.
					if zoneExportRef.FindString(o.Value) != o.Value {
						return ZoneExport{}, fmt.Errorf("export refuses mixed literal credential reference")
					}
				} else {
					o.Value = placeholder()
				}
			}
			refs(o.Value)
		}
		out.WriteString((config.RecordSpec{Name: r.Name, Interval: r.Interval, Type: r.Type, Target: r.Target, Opts: opts}).Render())
		out.WriteByte('\n')
	}
	text := out.String()
	if _, _, _, err = config.ParseZone("export.bind", []byte(text), s.validationOptions()); err != nil {
		return ZoneExport{}, fmt.Errorf("redacted configuration is not portable; replace opaque values with references")
	}
	result := ZoneExport{Text: text, Missing: make([]string, 0, len(missing))}
	for ref := range missing {
		result.Missing = append(result.Missing, ref)
	}
	slices.Sort(result.Missing)
	return result, nil
}
