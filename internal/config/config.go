// SPDX-License-Identifier: GPL-2.0-or-later

// Package config locates grc.conf files and color files.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sykesm/gogrc/internal/rx"
)

// Env holds the environment that determines search paths.
type Env struct {
	Home       string // $HOME
	ConfigHome string // $XDG_CONFIG_HOME, defaulting to ~/.config
	DataHome   string // $XDG_DATA_HOME, defaulting to ~/.local/share
}

// FromOS reads Env from the process environment.
func FromOS() Env {
	e := Env{
		Home:       os.Getenv("HOME"),
		ConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		DataHome:   os.Getenv("XDG_DATA_HOME"),
	}
	if e.ConfigHome == "" && e.Home != "" {
		e.ConfigHome = filepath.Join(e.Home, ".config")
	}
	if e.DataHome == "" && e.Home != "" {
		e.DataHome = filepath.Join(e.Home, ".local", "share")
	}
	return e
}

// DefaultsDir is where --write-defaults puts the stock configuration.
func (e Env) DefaultsDir() string {
	if e.ConfigHome == "" {
		return ""
	}
	return filepath.Join(e.ConfigHome, "gogrc", "defaults")
}

// GrcConfFiles returns the grc.conf candidates in the order they are read.
// A match in a later file overrides a match in an earlier one.
func (e Env) GrcConfFiles() []string {
	files := []string{"/etc/grc.conf", "/usr/local/etc/grc.conf", "/opt/homebrew/etc/grc.conf"}
	if d := e.DefaultsDir(); d != "" {
		files = append(files, filepath.Join(d, "grc.conf"))
	}
	if e.ConfigHome != "" {
		files = append(files, filepath.Join(e.ConfigHome, "grc", "grc.conf"))
	}
	if e.Home != "" {
		files = append(files, filepath.Join(e.Home, ".grc", "grc.conf"))
	}
	if e.ConfigHome != "" {
		files = append(files, filepath.Join(e.ConfigHome, "gogrc", "grc.conf"))
	}
	return files
}

// ColorFileDirs returns the directories searched for color files, in order.
func (e Env) ColorFileDirs() []string {
	var dirs []string
	if e.ConfigHome != "" {
		dirs = append(dirs, filepath.Join(e.ConfigHome, "gogrc"))
	}
	if e.DataHome != "" {
		dirs = append(dirs, filepath.Join(e.DataHome, "grc"))
	}
	if e.ConfigHome != "" {
		dirs = append(dirs, filepath.Join(e.ConfigHome, "grc"))
	}
	if e.Home != "" {
		dirs = append(dirs, filepath.Join(e.Home, ".grc"))
	}
	if d := e.DefaultsDir(); d != "" {
		dirs = append(dirs, d)
	}
	return append(dirs, "/opt/homebrew/share/grc", "/usr/local/share/grc", "/usr/share/grc")
}

// ErrNotFound is returned when a color file cannot be located.
var ErrNotFound = errors.New("not found")

// FindColorFile resolves a color file name from a grc.conf file. Absolute
// paths are used as given. Relative names, including ones in a subdirectory
// such as "kube/conf.pods", are looked up in ColorFileDirs. Unlike grcat,
// the current directory is not searched, so running a command in a directory
// that contains a conf.* file cannot change its colors or run the file's
// command= entries.
func (e Env) FindColorFile(name string) (string, error) {
	if filepath.IsAbs(name) {
		if isFile(name) {
			return name, nil
		}
		return "", fmt.Errorf("color file %s: %w", name, ErrNotFound)
	}
	for _, dir := range e.ColorFileDirs() {
		if p := filepath.Join(dir, name); isFile(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("color file %s: %w", name, ErrNotFound)
}

// isFile reports whether p exists and is not a directory. Like grc, pipes
// and other special files are accepted.
func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// Lookup finds the color file for a command line in the grc.conf files. The
// last file with a match wins; within a file the first match wins. Unreadable
// files and bad regexps are reported through warn.
func (e Env) Lookup(args []string, warn func(string)) string {
	cmdline := strings.Join(args, " ")
	result := ""
	for _, path := range e.GrcConfFiles() {
		if !isFile(path) {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			warn(err.Error())
			continue
		}
		conf, ok, err := match(f, path, cmdline, warn)
		f.Close()
		if err != nil {
			warn(fmt.Sprintf("%s: %v", path, err))
		}
		if ok {
			result = conf
		}
	}
	return result
}

// match scans one grc.conf and returns the color file named after the first
// regexp that matches cmdline, and whether one matched. Like grc, every
// non-comment line that does not follow a matching regexp is itself tried as
// a regexp, and a matching regexp on the last line names no color file.
func match(r io.Reader, name, cmdline string, warn func(string)) (string, bool, error) {
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		l := sc.Text()
		if l == "" || l[0] == '#' {
			continue
		}
		expr := strings.TrimSpace(l)
		re, err := rx.Compile(expr)
		if err != nil {
			warn(fmt.Sprintf("%s:%d: bad regexp %q: %v", name, n, expr, err))
			continue
		}
		if re.MatchString(cmdline) {
			if !sc.Scan() {
				return "", true, sc.Err()
			}
			return strings.TrimSpace(sc.Text()), true, nil
		}
	}
	return "", false, sc.Err()
}
