package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/store"
)

const nodeUsage = `usage:
  unagikuma node add NAME [--title T] [--region R] [--server URL]
                                  declare a probe node and print its install commands
  unagikuma node list              list the nodes and their tokens
  unagikuma node token NAME [--server URL]
                                  issue a new token (the old one stops working)
  unagikuma node revoke NAME       revoke the node's token

Common flags: --data DIR, --config FILE.`

func runNode(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(strings.TrimSpace(nodeUsage))
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("node "+sub, flag.ExitOnError)
	data := fs.String("data", defaultDataDir(), "data directory")
	cfgPath := fs.String("config", "", "path of monitors.bind (default <data>/monitors.bind)")
	server := fs.String("server", "", "master URL the agent connects to (default https://<this host>:8077)")
	title := fs.String("title", "", "description of the node")
	region := fs.String("region", "", "region label of the node")
	// The name may come before or after the flags.
	name := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	_ = fs.Parse(args)
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	path := *cfgPath
	if path == "" {
		path = filepath.Join(*data, "monitors.bind")
	}
	if *server == "" {
		host, _ := os.Hostname()
		*server = "https://" + net.JoinHostPort(host, "8077")
	}
	if sub != "list" && name == "" {
		return errors.New(strings.TrimSpace(nodeUsage))
	}
	switch sub {
	case "add":
		return nodeAdd(out, path, *data, name, *title, *region, *server)
	case "list":
		return nodeList(out, path, *data)
	case "token":
		cfg, err := loadConfig(path)
		if err != nil {
			return err
		}
		if cfg.FindNode(name) == nil || name == cfg.MName {
			return fmt.Errorf("node %s is not declared in %s", name, path)
		}
		return nodeIssue(out, *data, name, *server)
	case "revoke":
		st, err := store.Open(*data, store.Options{Tool: true})
		if err != nil {
			return err
		}
		defer st.Close()
		if err := st.DeleteAgentToken(name); err != nil {
			return err
		}
		cliAudit(st, "node.token.delete", name, nil)
		fmt.Fprintf(out, "Token of node %s revoked. A running server ends its session within one ping interval.\n", name)
		return nil
	}
	return errors.New(strings.TrimSpace(nodeUsage))
}

func loadConfig(path string) (*config.Config, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	_, cfg, diags := config.Load(path, src, config.Options{})
	for _, d := range diags {
		if d.Severity == config.SevError {
			return nil, fmt.Errorf("%s has errors, fix them first: %v", path, d)
		}
	}
	return cfg, nil
}

// nodeAdd writes the NODE record with a raised serial, so a running server
// applies it, then issues the node's token.
func nodeAdd(out io.Writer, path, data, name, title, region, server string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f, cfg, diags := config.Load(path, src, config.Options{})
	for _, d := range diags {
		if d.Severity == config.SevError {
			return fmt.Errorf("%s has errors, fix them first: %v", path, d)
		}
	}
	switch {
	case !config.ValidName(name):
		return fmt.Errorf("invalid node name %q", name)
	case name == cfg.MName || cfg.HasName(name):
		return fmt.Errorf("%s is already used in %s", name, path)
	}
	spec := config.RecordSpec{Name: name, Type: "NODE", Target: title}
	if region != "" {
		spec.Opts = append(spec.Opts, config.Opt{Key: "region", Value: region})
	}
	if err := f.UpsertRecord(spec); err != nil {
		return err
	}
	cur, err := f.Serial()
	if err != nil {
		return err
	}
	next, warn, err := config.NextSerial(cur, time.Now())
	if err != nil {
		return err
	}
	if warn != "" {
		fmt.Fprintln(os.Stderr, "warning:", warn)
	}
	if err := f.SetSerial(next); err != nil {
		return err
	}
	if _, _, diags := config.Load(path, f.Bytes(), config.Options{}); config.HasErrors(diags) {
		return fmt.Errorf("the new record does not validate: %v", diags[0])
	}
	if err := replaceFile(path, f.Bytes()); err != nil {
		return err
	}
	fmt.Fprintf(out, "Node %s added to %s (serial %d → %d); a running server applies it within seconds.\n", name, path, cur, next)
	return nodeIssue(out, data, name, server)
}

// nodeIssue creates a token for name and prints the install commands.
func nodeIssue(out io.Writer, data, name, server string) error {
	st, err := store.Open(data, store.Options{Tool: true})
	if err != nil {
		return err
	}
	defer st.Close()
	tok, err := st.CreateAgentToken(name)
	if err != nil {
		return err
	}
	cliAudit(st, "node.token", name, nil)
	pki, err := cluster.LoadOrCreatePKI(data, time.Now())
	if err != nil {
		return err
	}
	inst := cluster.InstallCommands(server, name, tok, pki.Fingerprint())
	fmt.Fprintf(out, "\nToken for %s (shown only now): %s\n", name, tok)
	fmt.Fprintf(out, "Master: %s (adjust if agents reach it under another address; they need this port, not a proxy)\n", server)
	fmt.Fprintf(out, "\nRun the agent on the node:\n\n%s\n\nOr install it as a systemd service:\n\n%s\n", indent(inst.Shell), indent(inst.Systemd))
	return nil
}

func nodeList(out io.Writer, path, data string) error {
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}
	st, err := store.Open(data, store.Options{Tool: true})
	if err != nil {
		return err
	}
	defer st.Close()
	tokens := map[string]store.AgentToken{}
	list, err := st.AgentTokens()
	if err != nil {
		return err
	}
	for _, t := range list {
		tokens[t.Node] = t
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTITLE\tREGION\tPROBE\tTOKEN\tLAST USED\tFROM")
	fmt.Fprintf(tw, "%s\t(this server)\t\t%v\t\t\t\n", cfg.MName, selfProbes(cfg))
	for _, n := range cfg.Nodes {
		if n.Name == cfg.MName {
			continue
		}
		tok, used, from := "none", "", ""
		if t, ok := tokens[n.Name]; ok {
			tok, from = t.Prefix+"…", t.LastAddr
			if t.LastUsed != nil {
				used = t.LastUsed.Local().Format("2006-01-02 15:04")
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%s\t%s\t%s\n", n.Name, n.Description, n.Region, n.Probe, tok, used, from)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if pki, err := cluster.LoadOrCreatePKI(data, time.Now()); err == nil {
		fmt.Fprintf(out, "\nCA fingerprint: %s\nLive connection state is shown in the web UI.\n", pki.Fingerprint())
	}
	return nil
}

func selfProbes(cfg *config.Config) bool {
	if n := cfg.FindNode(cfg.MName); n != nil {
		return n.Probe
	}
	return true
}

func indent(s string) string { return "    " + strings.ReplaceAll(s, "\n", "\n    ") }

// replaceFile writes b over path atomically, keeping its permissions.
func replaceFile(path string, b []byte) error {
	mode := os.FileMode(0o640)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".monitors.bind.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
