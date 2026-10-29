package app

import (
	"testing"

	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/tunables"
)

// TLS checks watch certificate expiry by default; a monitor's own
// thresholds win, and 0 written on it turns one off.
func TestCertDefaults(t *testing.T) {
	ts, _ := tunables.New(nil)
	for _, c := range []struct {
		spec       probe.Spec
		warn, crit string
	}{
		{probe.Spec{Type: "HTTP", Target: "https://example.com/"}, "21", "7"},
		{probe.Spec{Type: "HTTP", Target: "http://example.com/"}, "", ""},
		{probe.Spec{Type: "TCP", Target: "10.0.0.1:993", Opts: map[string]string{"tls": "yes"}}, "21", "7"},
		{probe.Spec{Type: "TCP", Target: "10.0.0.1:22"}, "", ""},
		{probe.Spec{Type: "HTTP", Target: "https://x/", Opts: map[string]string{"cert-days": "30", "cert-critical-days": "0"}}, "30", "0"},
	} {
		certDefaults(&c.spec, ts)
		if c.spec.Opts["cert-days"] != c.warn || c.spec.Opts["cert-critical-days"] != c.crit {
			t.Errorf("%s %s: %v", c.spec.Type, c.spec.Target, c.spec.Opts)
		}
	}
	off, _ := tunables.New(map[string]string{"probe.cert-warn-days": "0", "probe.cert-critical-days": "0"})
	s := probe.Spec{Type: "HTTP", Target: "https://example.com/"}
	certDefaults(&s, off)
	if len(s.Opts) != 0 {
		t.Errorf("defaults turned off: %v", s.Opts)
	}
}
