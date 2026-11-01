package importer

import (
	"errors"
	"regexp"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/config"
	"gopkg.in/yaml.v3"
)

var gatusStatus = regexp.MustCompile(`^\[STATUS\]\s*==\s*([1-5][0-9][0-9])$`)

type gatusEndpoint struct {
	Name, Group, URL, Interval, Method, Body string
	Enabled                                  *bool
	Headers                                  map[string]string
	Conditions                               []string
	Client                                   struct {
		Insecure       bool
		IgnoreRedirect bool `yaml:"ignore-redirect"`
		Timeout        string
		Other          map[string]yaml.Node `yaml:",inline"`
	}
	Other map[string]yaml.Node `yaml:",inline"`
}

// Gatus imports a single local YAML document, never resolving includes or env.
func Gatus(data []byte, existing *config.Config, o Options) (*Result, error) {
	root, err := boundedYAML(data)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Endpoints []gatusEndpoint
		Other     map[string]yaml.Node `yaml:",inline"`
	}
	if root.Decode(&doc) != nil || doc.Endpoints == nil {
		return nil, errors.New("Gatus document requires an endpoints array with typed fields")
	}
	if len(doc.Endpoints) > 1000 {
		return nil, errors.New("maximum 1000 endpoints")
	}
	var mons []adapterMonitor
	for _, e := range doc.Endpoints {
		m := adapterMonitor{name: e.Name, group: e.Group, target: e.URL, interval: e.Interval, method: e.Method, body: e.Body, headers: e.Headers, timeout: e.Client.Timeout, insecure: e.Client.Insecure, noFollow: e.Client.IgnoreRedirect, disabled: e.Enabled != nil && !*e.Enabled}
		if len(e.Other) > 0 || len(e.Client.Other) > 0 {
			m.why = "unsupported endpoint/client field (including alerts, DNS, TLS files, proxy or authentication configuration)"
		}
		if len(e.Conditions) != 1 {
			m.why = "exactly one literal [STATUS] == N condition is supported"
		} else if match := gatusStatus.FindStringSubmatch(strings.TrimSpace(e.Conditions[0])); match == nil {
			m.why = "unsupported condition: only literal [STATUS] == N is equivalent"
		} else {
			m.expect = match[1]
		}
		mons = append(mons, m)
	}
	r := adapterResult("Gatus", mons, existing, o)
	if len(doc.Other) > 0 {
		r.add(Item{Kind: "configuration", Source: "top-level settings", Status: StatusLeftOut, Why: "Only endpoints are supported; global settings, external endpoints, suites, includes and alerting must be migrated separately."})
		r.finish()
	}
	return r, nil
}
