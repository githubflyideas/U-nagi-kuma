package cluster

import (
	"strings"
	"testing"
)

func TestInstallCommands(t *testing.T) {
	in := InstallCommands("https://m.example.com:8077", "bj", "uka_tok'en", "sha256:ab")
	for name, cmd := range map[string]string{"shell": in.Shell, "systemd": in.Systemd, "windows": in.Windows, "docker": in.Docker} {
		if !strings.Contains(cmd, "--ca-fingerprint sha256:ab") || strings.Contains(cmd, "--token ") {
			t.Errorf("%s: %s", name, cmd)
		}
		t.Logf("%s:\n%s", name, cmd)
	}
	// The token is quoted for each shell, never left bare.
	if !strings.Contains(in.Shell, `'uka_tok'\''en'`) || !strings.Contains(in.Windows, `'uka_tok''en'`) {
		t.Fatal("token quoting")
	}
	if !strings.Contains(in.Docker, "--network host") || !strings.Contains(in.Docker, " \\\n  -v ") {
		t.Fatal("docker command shape")
	}
}
