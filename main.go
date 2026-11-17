// SPDX-License-Identifier: GPL-2.0-or-later

// Command gogrc is a generic colorizer compatible with grc and grcat
// configuration files.
//
// Usage:
//
//	gogrc [options] command [args...]
//	gogrc --cat NAME
//	gogrc --init bash|zsh|fish
//	gogrc --write-defaults [--force]
//
// Run "gogrc --help" for the options, and see README.md for how gogrc
// differs from grc.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/colorize"
	"github.com/sykesm/gogrc/internal/config"
	"github.com/sykesm/gogrc/internal/defaults"
	"github.com/sykesm/gogrc/internal/runner"
	"github.com/sykesm/gogrc/internal/shellinit"
	"github.com/sykesm/gogrc/internal/stream"
)

// version is set at build time by the Makefile.
var version = "0.1.0-dev"

// usage is the help text; %s is the defaults directory.
const usage = `gogrc - generic colorizer (compatible with grc 1.14)

Usage:
  gogrc [options] command [args...]
  gogrc --cat NAME            color stdin with color file NAME (like grcat)
  gogrc --init bash|zsh|fish  print shell aliases for common commands
  gogrc --write-defaults      write the stock configuration to %s

Options:
  -e, --stderr        color stderr; stdout is then left alone unless -s is given
  -s, --stdout        color stdout (the default), even with -e
  -c, --config NAME   use color file NAME instead of looking it up in grc.conf
      --color WHEN    on, off or auto (default); auto colors when stdout is a
                      terminal and NO_COLOR is not set (--colour also works)
      --no-pty        give the command pipes instead of pseudo-terminals
      --ansi MODE     native, strip or merge: what to do with lines the command
                      colored itself (overrides the color file)
      --force         with --write-defaults, overwrite existing files
  -h, --help          show this help
      --version       show the version
`

// Exit codes for gogrc's own outcomes, following shell conventions.
const (
	exitSuccess   = 0
	exitFailure   = 1
	exitUsage     = 2
	exitCannotRun = 126
	exitNotFound  = 127
)

func main() {
	a := &app{
		env:    config.FromOS(),
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		exec:   runner.Exec,
	}
	runner.Exit(a.run(os.Args))
}

// app is one invocation of gogrc.
type app struct {
	env                   config.Env
	stdin, stdout, stderr *os.File
	// exec replaces the process with a command. It returns only on failure.
	exec func(args []string) error
}

// options are the parsed command-line options.
type options struct {
	stderr, stdout bool
	config         string
	color          string
	noPTY          bool
	ansi           string
	cat            string
	init           string
	writeDefaults  bool
	force          bool
	help, version  bool
}

// run executes gogrc with the given argv (including the program name) and
// returns how gogrc should exit.
func (a *app) run(argv []string) runner.Status {
	// Run as grcat (through a link), behave like grcat.
	if filepath.Base(argv[0]) == "grcat" {
		if len(argv) != 2 {
			a.warn("usage: grcat NAME")
			return exit(exitUsage)
		}
		return exit(a.filter(argv[1], ""))
	}

	opts, args, err := parseArgs(argv[1:])
	if err != nil {
		a.warn(err.Error())
		fmt.Fprintf(a.stderr, usage, a.env.DefaultsDir())
		return exit(exitUsage)
	}
	switch {
	case opts.help:
		fmt.Fprintf(a.stdout, usage, a.env.DefaultsDir())
		return exit(exitSuccess)
	case opts.version:
		fmt.Fprintf(a.stdout, "gogrc %s\n", version)
		return exit(exitSuccess)
	case opts.writeDefaults:
		return exit(a.writeDefaults(opts.force))
	case opts.init != "":
		script, err := shellinit.Script(opts.init)
		if err != nil {
			a.warn(err.Error())
			return exit(exitUsage)
		}
		fmt.Fprint(a.stdout, script)
		return exit(exitSuccess)
	case opts.cat != "":
		return exit(a.filter(opts.cat, opts.ansi))
	case len(args) == 0:
		fmt.Fprintf(a.stderr, usage, a.env.DefaultsDir())
		return exit(exitUsage)
	}
	return a.wrap(opts, args)
}

// exit returns a Status for an exit code.
func exit(code int) runner.Status {
	return runner.Status{Code: code}
}

// wrap runs the command, coloring its output when a color file applies, and
// otherwise replaces gogrc with it.
func (a *app) wrap(opts options, args []string) runner.Status {
	if !a.colorEnabled(opts.color) {
		return a.execCommand(args)
	}
	name, given := opts.config, opts.config != ""
	if !given {
		name = a.env.Lookup(args, a.warn)
	}
	if name == "" {
		return a.execCommand(args)
	}
	file, err := a.loadColorFile(name, given)
	if err != nil {
		a.warn(err.Error())
		return a.execCommand(args)
	}
	mode := ansiMode(file, opts.ansi)

	// Like grc: stdout by default, stderr only with -e, both with -es.
	newStream := func(dst *os.File, enabled bool) runner.Stream {
		s := runner.Stream{File: dst, PTY: !opts.noPTY}
		if enabled {
			s.Colorizer = colorize.New(file, mode)
			s.Colorizer.Warn = a.warn
		}
		return s
	}
	status, err := runner.Run(runner.Config{
		Args:     args,
		Stdin:    a.stdin,
		Stdout:   newStream(a.stdout, !opts.stderr || opts.stdout),
		Stderr:   newStream(a.stderr, opts.stderr),
		Warnings: a.stderr,
	})
	if err != nil {
		a.warn(fmt.Sprintf("%s: %v", args[0], unwrapExec(err)))
		return exit(exitCodeFor(err))
	}
	return status
}

// filter colors stdin to stdout with the named color file, like grcat, and
// returns the exit code.
func (a *app) filter(name, ansi string) int {
	file, err := a.loadColorFile(name, true)
	if err != nil {
		a.warn(err.Error())
		return exitFailure
	}
	c := colorize.New(file, ansiMode(file, ansi))
	c.Warn = a.warn
	var opts stream.Options
	if term.IsTerminal(int(a.stdout.Fd())) {
		opts.PartialTimeout = stream.DefaultPartialTimeout
	}
	if err := stream.Copy(a.stdout, a.stdin, c, opts); err != nil {
		a.warn(err.Error())
		return exitFailure
	}
	return exitSuccess
}

// ansiMode returns the mode set on the command line, or else the color
// file's. The command-line value has already been validated.
func ansiMode(file *colorfile.File, flag string) colorfile.AnsiMode {
	if flag == "" {
		return file.Ansi
	}
	mode, _ := colorfile.ParseAnsiMode(flag)
	return mode
}

// loadColorFile finds and parses a color file, reporting its warnings. A
// name given on the command line that contains a slash is first tried
// relative to the current directory, like grcat; other names, and all names
// from grc.conf, are looked up in the search path only.
func (a *app) loadColorFile(name string, onCommandLine bool) (*colorfile.File, error) {
	path := name
	if !onCommandLine || !strings.ContainsRune(name, '/') || !isFile(name) {
		var err error
		if path, err = a.env.FindColorFile(name); err != nil {
			return nil, err
		}
	}
	file, err := colorfile.Load(path)
	if err != nil {
		return nil, err
	}
	for _, w := range file.Warnings {
		a.warn(w)
	}
	return file, nil
}

// isFile reports whether p exists and is not a directory.
func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// writeDefaults writes the stock configuration to the defaults directory and
// returns the exit code.
func (a *app) writeDefaults(force bool) int {
	dir := a.env.DefaultsDir()
	if dir == "" {
		a.warn("cannot determine the configuration directory (HOME is not set)")
		return exitFailure
	}
	written, skipped, err := defaults.Write(dir, force)
	if err != nil {
		a.warn(err.Error())
		return exitFailure
	}
	fmt.Fprintf(a.stdout, "Wrote %d files to %s\n", len(written), dir)
	if len(skipped) > 0 {
		fmt.Fprintf(a.stdout, "Kept %d existing files; use --force to overwrite them\n", len(skipped))
	}
	return exitSuccess
}

// colorEnabled interprets the --color value. "auto" (or no value) colors
// when stdout is a terminal and NO_COLOR is not set.
func (a *app) colorEnabled(when string) bool {
	switch when {
	case "on":
		return true
	case "off":
		return false
	}
	return term.IsTerminal(int(a.stdout.Fd())) && os.Getenv("NO_COLOR") == ""
}

// execCommand replaces gogrc with the command. It returns only on failure.
func (a *app) execCommand(args []string) runner.Status {
	err := a.exec(args)
	if err == nil {
		return exit(exitSuccess)
	}
	a.warn(fmt.Sprintf("%s: %v", args[0], unwrapExec(err)))
	return exit(exitCodeFor(err))
}

// exitCodeFor returns the shell's exit code for a command that could not be
// started: 127 when it was not found, 126 otherwise.
func exitCodeFor(err error) int {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return exitNotFound
	}
	return exitCannotRun
}

// unwrapExec drops the command name exec.Error adds, since gogrc's own
// messages already start with it.
func unwrapExec(err error) error {
	if e, ok := errors.AsType[*exec.Error](err); ok {
		return e.Err
	}
	return err
}

// warn reports a problem on stderr.
func (a *app) warn(msg string) {
	fmt.Fprintf(a.stderr, "gogrc: %s\n", msg)
}

// parseArgs parses options up to the first non-option argument, like grc's
// getopt. Short options may be combined (-es) and option values may follow
// as a separate argument or after "=".
func parseArgs(argv []string) (options, []string, error) {
	var o options
	strOpts := map[string]*string{
		"config": &o.config, "color": &o.color, "colour": &o.color,
		"ansi": &o.ansi, "cat": &o.cat, "init": &o.init,
	}
	boolOpts := map[string]*bool{
		"stderr": &o.stderr, "stdout": &o.stdout, "no-pty": &o.noPTY,
		"write-defaults": &o.writeDefaults, "force": &o.force,
		"help": &o.help, "version": &o.version,
	}
	var pty bool
	boolOpts["pty"] = &pty // grc's experimental flag; a pty is now the default

	i := 0
	for ; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if name, ok := strings.CutPrefix(arg, "--"); ok {
			name, value, hasValue := strings.Cut(name, "=")
			if p, ok := boolOpts[name]; ok {
				if hasValue {
					return o, nil, fmt.Errorf("option --%s takes no value", name)
				}
				*p = true
				continue
			}
			p, ok := strOpts[name]
			if !ok {
				return o, nil, fmt.Errorf("unknown option --%s", name)
			}
			if !hasValue {
				if i+1 == len(argv) {
					return o, nil, fmt.Errorf("option --%s requires a value", name)
				}
				i++
				value = argv[i]
			}
			*p = value
			continue
		}
		for j := 1; j < len(arg); j++ {
			switch arg[j] {
			case 'e':
				o.stderr = true
			case 's':
				o.stdout = true
			case 'h':
				o.help = true
			case 'c':
				switch {
				case j+1 < len(arg):
					o.config = arg[j+1:]
				case i+1 < len(argv):
					i++
					o.config = argv[i]
				default:
					return o, nil, errors.New("option -c requires a value")
				}
				j = len(arg)
			default:
				return o, nil, fmt.Errorf("unknown option -%c", arg[j])
			}
		}
	}

	switch o.color {
	case "", "auto", "on", "off":
	default:
		return o, nil, fmt.Errorf("invalid --color value %q (want on, off or auto)", o.color)
	}
	if o.ansi != "" {
		if _, err := colorfile.ParseAnsiMode(o.ansi); err != nil {
			return o, nil, err
		}
	}
	return o, argv[i:], nil
}
