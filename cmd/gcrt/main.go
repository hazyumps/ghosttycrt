package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

const version = "0.0.1-m0"

func main() {
	os.Exit(run())
}

func run() int {
	args := os.Args[1:]
	cmd := "tui"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("gcrt", flag.ExitOnError)
	paths := config.DefaultPaths()
	fs.StringVar(&paths.Config, "config-dir", paths.Config, "configuration directory")
	fs.StringVar(&paths.State, "state-dir", paths.State, "state directory")
	fs.StringVar(&paths.Data, "data-dir", paths.Data, "data directory")
	fs.StringVar(&paths.Cache, "cache-dir", paths.Cache, "cache directory")
	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: gcrt [tui|check] [flags]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if showVersion {
		fmt.Println("gcrt " + version)
		return 0
	}

	cfg, err := config.Load(paths.ConfigFile())
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcrt:", err)
		return 1
	}
	cfgProblems := cfg.Validate()

	providers := make([]string, 0, len(cfg.Credentials.Providers))
	for name := range cfg.Credentials.Providers {
		providers = append(providers, name)
	}

	file, problems, err := session.Load(paths.SessionsFile(), providers)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcrt:", err)
		return 1
	}
	for _, e := range cfgProblems {
		problems.Errors = append(problems.Errors, e)
	}

	switch cmd {
	case "check":
		return check(paths, file, problems)
	case "tui":
		client := tmux.New(tmux.Socket)
		if !client.Available() {
			fmt.Fprintln(os.Stderr, "gcrt: tmux is required and was not found on PATH.")
			fmt.Fprintln(os.Stderr, "      install it with: "+tmux.InstallHint())
			return 1
		}
		return browse(cfg, paths, file, problems, client)
	default:
		fmt.Fprintf(os.Stderr, "gcrt: unknown command %q\n", cmd)
		fs.Usage()
		return 2
	}
}

func check(paths config.Paths, file *session.File, problems session.Problems) int {
	fmt.Println("config:   " + paths.ConfigFile())
	fmt.Println("sessions: " + paths.SessionsFile())
	fmt.Printf("sessions: %d\n", len(file.Session))
	for _, line := range problems.Strings() {
		fmt.Println(line)
	}
	if !problems.OK() {
		return 1
	}
	fmt.Println("ok")
	return 0
}

func browse(cfg *config.Config, paths config.Paths, file *session.File, problems session.Problems, client *tmux.Client) int {
	p := tea.NewProgram(tui.New(cfg, file, paths, problems, client), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gcrt:", err)
		return 1
	}
	return 0
}
