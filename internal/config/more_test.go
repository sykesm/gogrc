// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFromOS(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	e := FromOS()
	if e != (Env{Home: "/home/u", ConfigHome: "/home/u/.config", DataHome: "/home/u/.local/share"}) {
		t.Errorf("FromOS() = %+v", e)
	}
	t.Setenv("XDG_CONFIG_HOME", "/c")
	t.Setenv("XDG_DATA_HOME", "/d")
	if e := FromOS(); e.ConfigHome != "/c" || e.DataHome != "/d" {
		t.Errorf("FromOS() = %+v", e)
	}
	if got := e.DefaultsDir(); got != "/home/u/.config/gogrc/defaults" {
		t.Errorf("DefaultsDir() = %q", got)
	}
}

func TestEmptyEnv(t *testing.T) {
	var e Env
	if e.DefaultsDir() != "" {
		t.Error("DefaultsDir without HOME is not empty")
	}
	if got := e.GrcConfFiles(); len(got) != 3 {
		t.Errorf("GrcConfFiles() = %q, want only system files", got)
	}
	if got := e.ColorFileDirs(); len(got) != 3 {
		t.Errorf("ColorFileDirs() = %q, want only system directories", got)
	}
}

func TestSearchOrder(t *testing.T) {
	e := Env{Home: "/h", ConfigHome: "/h/.config", DataHome: "/h/.local/share"}
	wantConf := []string{
		"/etc/grc.conf", "/usr/local/etc/grc.conf", "/opt/homebrew/etc/grc.conf",
		"/h/.config/gogrc/defaults/grc.conf", "/h/.config/grc/grc.conf",
		"/h/.grc/grc.conf", "/h/.config/gogrc/grc.conf",
	}
	if got := e.GrcConfFiles(); !slices.Equal(got, wantConf) {
		t.Errorf("GrcConfFiles() = %q", got)
	}
	wantDirs := []string{
		"/h/.config/gogrc", "/h/.local/share/grc", "/h/.config/grc", "/h/.grc",
		"/h/.config/gogrc/defaults", "/opt/homebrew/share/grc", "/usr/local/share/grc", "/usr/share/grc",
	}
	if got := e.ColorFileDirs(); !slices.Equal(got, wantDirs) {
		t.Errorf("ColorFileDirs() = %q", got)
	}
}

func TestFindColorFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.x")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var e Env
	if got, err := e.FindColorFile(path); err != nil || got != path {
		t.Errorf("FindColorFile(%q) = %q, %v", path, got, err)
	}
	if _, err := e.FindColorFile(path + ".missing"); err == nil {
		t.Error("found a missing file")
	}
	if _, err := e.FindColorFile(filepath.Dir(path) + "/"); err == nil {
		t.Error("accepted a directory")
	}
}

func TestLookupWarnings(t *testing.T) {
	home := t.TempDir()
	e := Env{Home: home, ConfigHome: filepath.Join(home, ".config")}
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	// A bad regexp, then a match on the last line, which names no file.
	write(filepath.Join(home, ".config", "grc", "grc.conf"), "(\nconf.x\n^make\n", 0o644)
	// An unreadable file.
	write(filepath.Join(home, ".grc", "grc.conf"), "^ls\nconf.ls\n", 0)
	// A line too long for the scanner.
	write(filepath.Join(home, ".config", "gogrc", "grc.conf"), strings.Repeat("x", 128*1024)+"\n", 0o644)

	var warnings []string
	got := e.Lookup([]string{"make"}, func(s string) { warnings = append(warnings, s) })
	if got != "" {
		t.Errorf("Lookup = %q, want no color file", got)
	}
	if len(warnings) != 3 {
		t.Errorf("warnings = %q, want bad regexp, unreadable file and scanner error", warnings)
	}
}
