package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/importer"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// runImport converts another tool's data into monitors.bind records and
// accounts.
func runImport(args []string) error {
	const usage = "usage: unagikuma import kuma [--data DIR] [--area SECTION] [--role ROLE] [--dry-run] kuma.db|BACKUP.json\n" +
		"       unagikuma import nagios [--data DIR] [--area SECTION] [--dry-run] objects.cfg|DIR [objects.cfg|DIR]"
	if len(args) == 0 || (args[0] != "kuma" && args[0] != "nagios") {
		return errors.New(usage)
	}
	source := args[0]
	fs := flag.NewFlagSet("import "+source, flag.ExitOnError)
	data := fs.String("data", defaultDataDir(), "data directory")
	cfgPath := fs.String("config", "", "path of monitors.bind (default <data>/monitors.bind)")
	dry := fs.Bool("dry-run", false, "only print what would be added")
	area := fs.String("area", "", "$SCOPE section for the monitors, e.g. region:north (default: global)")
	role := fs.String("role", store.RoleOperator, "role of Uptime Kuma's accounts: admin, operator or viewer (limited to --area)")
	_ = fs.Parse(args[1:])
	if fs.NArg() == 0 || (source == "kuma" && fs.NArg() != 1) {
		return errors.New(usage)
	}
	if *cfgPath == "" {
		*cfgPath = filepath.Join(*data, "monitors.bind")
	}
	var raw []byte
	var err error
	if source == "nagios" {
		raw, err = readNagios(fs.Args())
	} else {
		raw, err = os.ReadFile(fs.Arg(0))
	}
	if err != nil {
		return err
	}
	src, err := os.ReadFile(*cfgPath)
	if err != nil {
		return err
	}
	f, cfg, diags := config.Load(*cfgPath, src, config.Options{})
	if config.HasErrors(diags) {
		return fmt.Errorf("%s has errors, fix them first: %v", *cfgPath, diags[0])
	}
	if !store.ValidRole(*role) {
		return fmt.Errorf("unknown role %q", *role)
	}
	var res *importer.Result
	if source == "nagios" {
		res, err = importer.Nagios(raw, cfg, importer.Options{Area: *area})
	} else {
		res, err = importer.Kuma(raw, cfg, importer.Options{Area: *area})
	}
	if err != nil {
		return err
	}
	fmt.Print(res.Text)
	for _, n := range res.Notes {
		fmt.Fprintln(os.Stderr, "note:", n)
	}
	for _, u := range res.Users {
		fmt.Fprintf(os.Stderr, "account: %s (active %v)\n", u.Username, u.Active)
	}
	if *dry {
		fmt.Fprintf(os.Stderr, "\n%d monitors, %d channels and %d accounts would be added (dry run).\n", res.Monitors, res.Channels, len(res.Users))
		return nil
	}
	st, err := store.Open(*data, store.Options{Tool: true})
	if err != nil {
		return err
	}
	defer st.Close()
	kek, err := store.LoadOrCreateKey(*data)
	if err != nil {
		return err
	}
	sec := st.Secrets(kek)
	for name := range res.Secrets {
		if sec.Has(name) {
			return fmt.Errorf("secret %s already exists; was this backup imported before?", name)
		}
	}
	for _, r := range res.Records {
		if err := f.UpsertRecord(r); err != nil {
			return err
		}
	}
	cur, _ := f.Serial()
	next, _, err := config.NextSerial(cur, time.Now())
	if err != nil {
		return err
	}
	if err := f.SetSerial(next); err != nil {
		return err
	}
	known := func(name string) bool { _, ok := res.Secrets[name]; return ok || sec.Has(name) }
	if _, _, diags := config.Load(*cfgPath, f.Bytes(), config.Options{SecretKnown: known}); config.HasErrors(diags) {
		return fmt.Errorf("the imported records do not validate: %v", diags[0])
	}
	for name, value := range res.Secrets {
		if err := sec.Put(name, value, nil); err != nil {
			return err
		}
	}
	if err := replaceFile(*cfgPath, f.Bytes()); err != nil {
		return err
	}
	var areas []string
	if *area != "" && *role != store.RoleAdmin {
		areas = []string{*area}
	}
	for _, u := range res.Users {
		if !u.Active {
			continue
		}
		if _, err := st.CreateUserHash(u.Username, u.Hash, *role, areas); err != nil {
			fmt.Fprintf(os.Stderr, "account %s not added: %v\n", u.Username, err)
		} else {
			fmt.Fprintf(os.Stderr, "account %s added as %s; the Uptime Kuma password keeps working\n", u.Username, *role)
		}
	}
	fmt.Fprintf(os.Stderr, "\nAdded %d monitors and %d channels to %s (serial %d → %d); credentials are stored as secrets.\n",
		res.Monitors, res.Channels, *cfgPath, cur, next)
	return nil
}

// readNagios joins Nagios object files; a directory contributes its *.cfg
// files, recursively, like cfg_dir=.
func readNagios(paths []string) ([]byte, error) {
	var out []byte
	for _, p := range paths {
		err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || (path != p && filepath.Ext(path) != ".cfg") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out = append(append(out, b...), '\n')
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
