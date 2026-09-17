package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/importers/securecrt"
	"github.com/hazyumps/ghosttycrt/internal/session"
)

// runImport converts an existing terminal client's configuration into
// sessions.toml. It is a dry run unless --write is given, and it backs the
// current file up before replacing it.
func runImport(args []string) int {
	// The source name comes first, before any flags: `gcrt import securecrt
	// --write`. Strip it before parsing, because the flag package stops at the
	// first positional argument.
	kind := "securecrt"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		kind = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("gcrt import", flag.ExitOnError)
	paths := config.DefaultPaths()
	fs.StringVar(&paths.Config, "config-dir", paths.Config, "configuration directory")
	fs.StringVar(&paths.State, "state-dir", paths.State, "state directory")
	fs.StringVar(&paths.Data, "data-dir", paths.Data, "data directory")
	fs.StringVar(&paths.Cache, "cache-dir", paths.Cache, "cache directory")

	var from string
	fs.StringVar(&from, "from", securecrt.DefaultDir(), "SecureCRT Sessions directory")
	var write bool
	fs.BoolVar(&write, "write", false, "replace sessions.toml (default is a dry run)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: gcrt import securecrt [--from DIR] [--write]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "gcrt import: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if kind != "securecrt" {
		fmt.Fprintf(os.Stderr, "gcrt import: unknown source %q (only securecrt is supported)\n", kind)
		return 2
	}

	cfg, err := config.Load(paths.ConfigFile())
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcrt:", err)
		return 1
	}

	imported, report, err := securecrt.Parse(from)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcrt:", err)
		return 1
	}
	fmt.Println("from " + from)
	for _, line := range report.Summary() {
		fmt.Println(line)
	}
	fmt.Println()

	providers := make([]string, 0, len(cfg.Credentials.Providers))
	for name := range cfg.Credentials.Providers {
		providers = append(providers, name)
	}

	current, _, err := session.Load(paths.SessionsFile(), providers)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcrt:", err)
		return 1
	}

	added, kept, dropped := diff(current.Session, imported)
	fmt.Printf("current:  %d session(s) in %s\n", len(current.Session), paths.SessionsFile())
	fmt.Printf("import:   %d session(s) — %d new, %d already present\n", len(imported), added, kept)
	if len(dropped) > 0 {
		fmt.Printf("would drop %d session(s) not in the import: %s\n", len(dropped), join(dropped))
	}

	if !write {
		fmt.Println("\ndry run — nothing written. re-run with --write to replace sessions.toml.")
		return 0
	}

	if problems := (&session.File{Session: imported}).Validate(providers); !problems.OK() {
		fmt.Fprintln(os.Stderr, "\ngcrt: refusing to write, the import does not validate:")
		for _, e := range problems.Strings() {
			fmt.Fprintln(os.Stderr, "  "+e)
		}
		return 1
	}

	backup, err := writeSessions(paths.SessionsFile(), imported)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcrt:", err)
		return 1
	}
	fmt.Printf("\nwrote %d session(s) to %s\n", len(imported), paths.SessionsFile())
	if backup != "" {
		fmt.Println("previous file backed up to " + backup)
	}
	return 0
}

func diff(current, imported []session.Session) (added, kept int, dropped []string) {
	have := map[string]session.Session{}
	for _, s := range current {
		have[s.ID] = s
	}
	incoming := map[string]bool{}
	for _, s := range imported {
		incoming[s.ID] = true
		if _, ok := have[s.ID]; ok {
			kept++
		} else {
			added++
		}
	}
	for _, s := range current {
		if !incoming[s.ID] {
			dropped = append(dropped, s.Name)
		}
	}
	sort.Strings(dropped)
	return added, kept, dropped
}

// writeSessions writes atomically, keeping a copy of whatever was there: an
// import replaces the whole file, so the previous one is worth rescuing.
func writeSessions(path string, sessions []session.Session) (backup string, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		backup = fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
		if err := copyFile(path, backup); err != nil {
			return "", err
		}
	}
	return backup, session.SaveFile(path, sessions)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func join(names []string) string {
	const max = 8
	if len(names) <= max {
		return fmt.Sprintf("%v", names)
	}
	return fmt.Sprintf("%v and %d more", names[:max], len(names)-max)
}
