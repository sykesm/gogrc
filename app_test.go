// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/sykesm/gogrc/internal/config"
	"github.com/sykesm/gogrc/internal/runner"
)

// testApp is an app with files for its standard streams and a fake exec.
type testApp struct {
	*app
	t        *testing.T
	execArgs []string
	execErr  error
}

func newTestApp(t *testing.T, stdin string) *testApp {
	t.Helper()
	home := t.TempDir()
	dir := t.TempDir()
	in := filepath.Join(dir, "stdin")
	if err := os.WriteFile(in, []byte(stdin), 0o644); err != nil {
		t.Fatal(err)
	}
	open := func(name string, flag int) *os.File {
		f, err := os.OpenFile(filepath.Join(dir, name), flag, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	ta := &testApp{t: t}
	ta.app = &app{
		env:    config.Env{Home: home, ConfigHome: filepath.Join(home, ".config"), DataHome: filepath.Join(home, ".local", "share")},
		stdin:  open("stdin", os.O_RDONLY),
		stdout: open("stdout", os.O_RDWR|os.O_CREATE),
		stderr: open("stderr", os.O_RDWR|os.O_CREATE),
		exec: func(args []string) error {
			ta.execArgs = args
			return ta.execErr
		},
	}
	t.Setenv("NO_COLOR", "")
	return ta
}

func (ta *testApp) read(f *os.File) string {
	ta.t.Helper()
	data, err := os.ReadFile(f.Name())
	if err != nil {
		ta.t.Fatal(err)
	}
	return string(data)
}

func (ta *testApp) stdoutText() string { return ta.read(ta.stdout) }
func (ta *testApp) stderrText() string { return ta.read(ta.stderr) }

// writeConf writes a file under the user's grc configuration directory.
func (ta *testApp) writeConf(name, content string) {
	ta.t.Helper()
	dir := filepath.Join(ta.env.ConfigHome, "grc")
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		ta.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		ta.t.Fatal(err)
	}
}

const testColors = "regexp=ERROR\ncolours=red\n"

func TestHelpAndVersion(t *testing.T) {
	ta := newTestApp(t, "")
	if st := ta.run([]string{"gogrc", "--help"}); st.Code != 0 {
		t.Errorf("--help: %+v", st)
	}
	if st := ta.run([]string{"gogrc", "--version"}); st.Code != 0 {
		t.Errorf("--version: %+v", st)
	}
	out := ta.stdoutText()
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "gogrc "+version) {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(out, ta.env.DefaultsDir()) {
		t.Errorf("help does not mention the defaults directory %s", ta.env.DefaultsDir())
	}
}

func TestUsageErrors(t *testing.T) {
	for _, argv := range [][]string{
		{"gogrc"},
		{"gogrc", "--bogus"},
		{"gogrc", "--init", "csh"},
		{"grcat"},
		{"/usr/local/bin/grcat", "a", "b"},
	} {
		ta := newTestApp(t, "")
		if st := ta.run(argv); st.Code != exitUsage {
			t.Errorf("%q: status %+v, want %d", argv, st, exitUsage)
		}
		if ta.stderrText() == "" {
			t.Errorf("%q: nothing on stderr", argv)
		}
	}
}

func TestInit(t *testing.T) {
	ta := newTestApp(t, "")
	if st := ta.run([]string{"gogrc", "--init", "zsh"}); st.Code != 0 {
		t.Fatalf("status %+v", st)
	}
	if out := ta.stdoutText(); !strings.Contains(out, `alias "$cmd=gogrc $cmd"`) {
		t.Errorf("stdout = %q", out)
	}
}

func TestWriteDefaults(t *testing.T) {
	ta := newTestApp(t, "")
	if st := ta.run([]string{"gogrc", "--write-defaults"}); st.Code != 0 {
		t.Fatalf("status %+v: %s", st, ta.stderrText())
	}
	if st := ta.run([]string{"gogrc", "--write-defaults"}); st.Code != 0 {
		t.Fatalf("second run: status %+v", st)
	}
	out := ta.stdoutText()
	if !strings.Contains(out, "Wrote 90 files") || !strings.Contains(out, "Kept 90 existing files") {
		t.Errorf("stdout = %q", out)
	}
	if _, err := os.Stat(filepath.Join(ta.env.DefaultsDir(), "grc.conf")); err != nil {
		t.Error(err)
	}
}

func TestWriteDefaultsFailures(t *testing.T) {
	ta := newTestApp(t, "")
	ta.env = config.Env{}
	if st := ta.run([]string{"gogrc", "--write-defaults"}); st.Code != exitFailure {
		t.Errorf("without HOME: status %+v", st)
	}

	ta = newTestApp(t, "")
	// A file where the configuration directory should be.
	if err := os.WriteFile(ta.env.ConfigHome, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if st := ta.run([]string{"gogrc", "--write-defaults"}); st.Code != exitFailure {
		t.Errorf("unwritable: status %+v", st)
	}
}

func TestCat(t *testing.T) {
	ta := newTestApp(t, "an ERROR\nfine\n")
	ta.writeConf("conf.test", testColors)
	if st := ta.run([]string{"gogrc", "--cat", "conf.test"}); st.Code != 0 {
		t.Fatalf("status %+v: %s", st, ta.stderrText())
	}
	want := "\033[0man \033[0m\033[31mERROR\033[0m\n\033[0mfine\033[0m\n"
	if got := ta.stdoutText(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestCatWithAnsiOverride(t *testing.T) {
	ta := newTestApp(t, "\033[1mERROR\033[0m\n")
	ta.writeConf("conf.test", testColors)
	if st := ta.run([]string{"gogrc", "--ansi", "strip", "--cat", "conf.test"}); st.Code != 0 {
		t.Fatalf("status %+v", st)
	}
	if got, want := ta.stdoutText(), "\033[0m\033[31mERROR\033[0m\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunAsGrcat(t *testing.T) {
	ta := newTestApp(t, "ERROR\n")
	ta.writeConf("conf.test", "#gogrc: ansi=strip\n"+testColors)
	if st := ta.run([]string{"/opt/bin/grcat", "conf.test"}); st.Code != 0 {
		t.Fatalf("status %+v", st)
	}
	if got, want := ta.stdoutText(), "\033[0m\033[31mERROR\033[0m\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestColorFilePaths(t *testing.T) {
	want := "\033[0m\033[31mERROR\033[0m\n"

	// grc.conf may name a color file in a subdirectory of the search path.
	ta := newTestApp(t, "")
	ta.writeConf("grc.conf", "^echo\nkube/conf.test\n")
	ta.writeConf("kube/conf.test", testColors)
	if st := ta.run([]string{"gogrc", "--color=on", "echo", "ERROR"}); st.Code != 0 {
		t.Fatalf("status %+v: %s", st, ta.stderrText())
	}
	if got := ta.stdoutText(); got != want {
		t.Errorf("subdirectory: stdout = %q, want %q", got, want)
	}

	// On the command line, a name with a slash is relative to the current
	// directory.
	cwd := t.TempDir()
	t.Chdir(cwd)
	if err := os.MkdirAll(filepath.Join(cwd, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "local", "conf.test"), []byte(testColors), 0o644); err != nil {
		t.Fatal(err)
	}
	ta = newTestApp(t, "ERROR\n")
	if st := ta.run([]string{"gogrc", "--cat", "local/conf.test"}); st.Code != 0 {
		t.Fatalf("status %+v: %s", st, ta.stderrText())
	}
	if got := ta.stdoutText(); got != want {
		t.Errorf("command line path: stdout = %q, want %q", got, want)
	}
	// ... and otherwise looked up in the search path, as grcat does.
	ta = newTestApp(t, "ERROR\n")
	ta.writeConf("kube/conf.test", testColors)
	if st := ta.run([]string{"gogrc", "--cat", "kube/conf.test"}); st.Code != 0 {
		t.Fatalf("status %+v: %s", st, ta.stderrText())
	}
	if got := ta.stdoutText(); got != want {
		t.Errorf("command line subdirectory: stdout = %q, want %q", got, want)
	}

	// A grc.conf entry is never resolved against the current directory.
	ta = newTestApp(t, "")
	ta.writeConf("grc.conf", "^echo\nlocal/conf.test\n")
	if st := ta.run([]string{"gogrc", "--color=on", "echo", "ERROR"}); st.Code != 0 {
		t.Fatalf("status %+v", st)
	}
	if ta.execArgs == nil {
		t.Errorf("colored with a color file from the current directory: stdout = %q", ta.stdoutText())
	}
}

func TestCatFailures(t *testing.T) {
	ta := newTestApp(t, "")
	if st := ta.run([]string{"gogrc", "--cat", "conf.missing"}); st.Code != exitFailure {
		t.Errorf("missing file: status %+v", st)
	}

	ta = newTestApp(t, "")
	ta.writeConf("conf.test", "regexp=(\ncolours=red\nbogus=1\n")
	if st := ta.run([]string{"gogrc", "--cat", "conf.test"}); st.Code != 0 {
		t.Errorf("file with warnings: status %+v", st)
	}
	if got := ta.stderrText(); !strings.Contains(got, "bad regexp") || !strings.Contains(got, "invalid keyword") {
		t.Errorf("stderr = %q", got)
	}

	ta = newTestApp(t, "ERROR\n")
	ta.writeConf("conf.test", testColors)
	ta.stdout.Close() // writing fails
	if st := ta.run([]string{"gogrc", "--cat", "conf.test"}); st.Code != exitFailure {
		t.Errorf("write error: status %+v", st)
	}
}

func TestWrapExecsWithoutColor(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"color off", []string{"gogrc", "--color=off", "-c", "conf.test", "echo"}},
		{"auto and not a terminal", []string{"gogrc", "-c", "conf.test", "echo"}},
		{"no grc.conf match", []string{"gogrc", "--color=on", "echo", "hi"}},
		{"missing color file", []string{"gogrc", "--color=on", "-c", "conf.missing", "echo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ta := newTestApp(t, "")
			ta.writeConf("conf.test", testColors)
			if st := ta.run(tc.argv); st.Code != 0 {
				t.Fatalf("status %+v", st)
			}
			if ta.execArgs == nil || ta.execArgs[0] != "echo" {
				t.Errorf("exec args = %q, want the command", ta.execArgs)
			}
		})
	}
}

func TestWrapExecFailure(t *testing.T) {
	ta := newTestApp(t, "")
	ta.execErr = &exec.Error{Name: "nope", Err: exec.ErrNotFound}
	if st := ta.run([]string{"gogrc", "--color=off", "nope"}); st.Code != exitNotFound {
		t.Errorf("status %+v, want %d", st, exitNotFound)
	}
	if got := ta.stderrText(); got != "gogrc: nope: executable file not found in $PATH\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestWrapColorsCommand(t *testing.T) {
	ta := newTestApp(t, "")
	ta.writeConf("grc.conf", "^sh\nconf.test\n")
	ta.writeConf("conf.test", testColors)
	st := ta.run([]string{"gogrc", "--color=on", "-es", "sh", "-c", "echo ERROR; echo ERROR >&2; exit 4"})
	if st.Code != 4 {
		t.Errorf("status %+v, want 4", st)
	}
	want := "\033[0m\033[31mERROR\033[0m\n"
	if got := ta.stdoutText(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := ta.stderrText(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if ta.execArgs != nil {
		t.Errorf("exec called with %q", ta.execArgs)
	}
}

func TestWrapStartFailure(t *testing.T) {
	ta := newTestApp(t, "")
	ta.writeConf("conf.test", testColors)
	st := ta.run([]string{"gogrc", "--color=on", "-c", "conf.test", "gogrc-no-such-command"})
	if st.Code != exitNotFound {
		t.Errorf("status %+v, want %d", st, exitNotFound)
	}
	st = ta.run([]string{"gogrc", "--color=on", "-c", "conf.test", t.TempDir()})
	if st.Code != exitCannotRun {
		t.Errorf("directory: status %+v, want %d", st, exitCannotRun)
	}
}

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{exec.ErrNotFound, exitNotFound},
		{&os.PathError{Op: "exec", Path: "x", Err: os.ErrNotExist}, exitNotFound},
		{&os.PathError{Op: "exec", Path: "x", Err: syscall.EACCES}, exitCannotRun},
		{errors.New("other"), exitCannotRun},
	}
	for _, tt := range tests {
		if got := exitCodeFor(tt.err); got != tt.want {
			t.Errorf("exitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestParseArgs(t *testing.T) {
	o, args, err := parseArgs([]string{"-esc", "conf.x", "--colour=on", "--ansi", "merge", "--pty", "ls", "-l"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.stderr || !o.stdout || o.config != "conf.x" || o.color != "on" || o.ansi != "merge" {
		t.Errorf("options = %+v", o)
	}
	if strings.Join(args, " ") != "ls -l" {
		t.Errorf("args = %q", args)
	}

	o, args, err = parseArgs([]string{"-cconf.y", "-h", "--no-pty", "--", "-weird"})
	if err != nil || o.config != "conf.y" || !o.help || !o.noPTY || len(args) != 1 || args[0] != "-weird" {
		t.Errorf("parseArgs = %+v, %q, %v", o, args, err)
	}
	if _, args, _ := parseArgs([]string{"-", "x"}); len(args) != 2 {
		t.Errorf(`"-" should end the options; args = %q`, args)
	}

	for _, bad := range [][]string{
		{"--color=sometimes", "ls"}, {"-x", "ls"}, {"--ansi=loud", "ls"}, {"--pty=1"},
		{"--config"}, {"-c"}, {"--nope"},
	} {
		if _, _, err := parseArgs(bad); err == nil {
			t.Errorf("parseArgs(%q) succeeded", bad)
		}
	}
}

func TestExit(t *testing.T) {
	if got := exit(3); got != (runner.Status{Code: 3}) {
		t.Errorf("exit(3) = %+v", got)
	}
}
