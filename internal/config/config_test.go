// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	conf := `# comment
^([/\w\.]+\/)?ping[236]?\b
conf.ping

(^|[/\w\.]+/)go\s+.*test
conf.gotest

^([/\w\.]+\/)?systemctl\b(?!\s+edit\b)
conf.systemctl
`
	tests := map[string]string{
		"ping -c 1 host":         "conf.ping",
		"/sbin/ping6 host":       "conf.ping",
		"go test ./...":          "conf.gotest",
		"systemctl status nginx": "conf.systemctl",
		"systemctl edit nginx":   "",
		"ls -l":                  "",
	}
	for cmdline, want := range tests {
		got, _, err := match(strings.NewReader(conf), "grc.conf", cmdline, func(s string) { t.Error(s) })
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("match(%q) = %q, want %q", cmdline, got, want)
		}
	}
}

func TestLookupLaterFileWins(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, ConfigHome: filepath.Join(home, ".config")}
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(env.DefaultsDir(), "grc.conf"), "^make\nconf.gcc\n^ls\nconf.ls\n")
	write(filepath.Join(home, ".config", "grc", "grc.conf"), "^make\nconf.mine\n")

	warn := func(s string) { t.Error(s) }
	if got := env.Lookup([]string{"make", "all"}, warn); got != "conf.mine" {
		t.Errorf("make: got %q", got)
	}
	if got := env.Lookup([]string{"ls"}, warn); got != "conf.ls" {
		t.Errorf("ls: got %q", got)
	}
}

func TestFindColorFile(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, ConfigHome: filepath.Join(home, ".config")}
	for _, dir := range []string{env.DefaultsDir(), filepath.Join(home, ".grc")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "conf.x"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := env.FindColorFile("conf.x")
	if err != nil || got != filepath.Join(home, ".grc", "conf.x") {
		t.Errorf("got %q, %v; want ~/.grc to win over defaults", got, err)
	}
	if _, err := env.FindColorFile("conf.missing"); err == nil {
		t.Error("expected error")
	}
}

func TestFindColorFilePaths(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, ConfigHome: filepath.Join(home, ".config")}
	sub := filepath.Join(env.ConfigHome, "grc", "kube")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "conf.pods"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// Like grcat, a name in a subdirectory is found in the search path.
	if got, err := env.FindColorFile("kube/conf.pods"); err != nil || got != filepath.Join(sub, "conf.pods") {
		t.Errorf("subdirectory: got %q, %v", got, err)
	}
	abs := filepath.Join(sub, "conf.pods")
	if got, err := env.FindColorFile(abs); err != nil || got != abs {
		t.Errorf("absolute: got %q, %v", got, err)
	}
	if _, err := env.FindColorFile(filepath.Join(sub, "conf.missing")); err == nil {
		t.Error("missing absolute path was found")
	}

	// Unlike grcat, the current directory is not searched.
	cwd := t.TempDir()
	t.Chdir(cwd)
	if err := os.WriteFile(filepath.Join(cwd, "conf.here"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := env.FindColorFile("conf.here"); err == nil {
		t.Errorf("found %q in the current directory", got)
	}
}
