// SPDX-License-Identifier: GPL-2.0-or-later

// Package colorfile parses grcat color configuration files (conf.*).
//
// A file is a sequence of entries. Each entry is a run of keyword=value lines;
// entries are separated by any line whose first character is not a letter.
// Lines starting with # and empty lines are ignored, except for gogrc
// directives of the form "#gogrc: key=value", which Python grcat treats as
// comments.
package colorfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/sykesm/gogrc/internal/rx"
)

// AnsiMode selects what happens to lines that already contain escape
// sequences written by the wrapped command.
type AnsiMode int

const (
	// AnsiNative passes lines that contain escape sequences through untouched.
	AnsiNative AnsiMode = iota
	// AnsiStrip removes the command's escape sequences before coloring.
	AnsiStrip
	// AnsiMerge matches on the text without escape sequences and keeps the
	// command's colors wherever gogrc does not apply its own.
	AnsiMerge
)

// ParseAnsiMode parses native, strip or merge.
func ParseAnsiMode(s string) (AnsiMode, error) {
	switch strings.ToLower(s) {
	case "native":
		return AnsiNative, nil
	case "strip":
		return AnsiStrip, nil
	case "merge":
		return AnsiMerge, nil
	}
	return AnsiNative, fmt.Errorf("invalid ansi mode %q (want native, strip or merge)", s)
}

// String returns the mode's name as used in directives and --ansi.
func (m AnsiMode) String() string {
	return [...]string{"native", "strip", "merge"}[m]
}

// Rule is one entry of a color file. The Has fields record whether the
// optional keywords were present, since an empty value is meaningful.
type Rule struct {
	// Line is the line number of the entry's first keyword.
	Line int
	// Regexp is the compiled regexp keyword.
	Regexp *rx.Regexp

	// Colors holds one escape string per regexp group, starting with the
	// whole match. It is nil when the entry has no colors keyword. The
	// special values Prev and Unchanged come from the previous and
	// unchanged color names.
	Colors []string
	// Count controls repeated matching: more (the default), once, stop,
	// previous, block or unblock. Other values behave like once.
	Count string

	// HasReplace reports a replace keyword. Replace is its template,
	// converted from Python's syntax to rx's.
	HasReplace bool
	Replace    string

	// HasCommand reports a command keyword. Command is run through the
	// shell each time the rule matches.
	HasCommand bool
	Command    string

	// HasConcat reports a concat keyword. Concat names a file each matching
	// line is appended to.
	HasConcat bool
	Concat    string

	// HasSkip reports a skip keyword. Skip, when true, removes matching
	// lines from the output.
	HasSkip bool
	Skip    bool
}

// File is a parsed color file.
type File struct {
	// Name is the file's path or the name given to Parse.
	Name string
	// Rules are the file's entries that have a regexp, in order.
	Rules []Rule
	// Ansi is the mode set by a "#gogrc: ansi=" directive, or AnsiNative.
	Ansi AnsiMode
	// Warnings describes lines and entries that were skipped as invalid.
	Warnings []string
}

// Load parses the color file at path.
func Load(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f, path)
}

var keywords = map[string]string{
	"regexp":  "regexp",
	"colours": "colours",
	"colour":  "colours",
	"colors":  "colours",
	"color":   "colours",
	"count":   "count",
	"command": "command",
	"skip":    "skip",
	"replace": "replace",
	"concat":  "concat",
}

var directiveRE = regexp.MustCompile(`^#\s*gogrc:\s*([A-Za-z_]+)\s*=\s*(\S*)\s*$`)

// Parse reads a color file. Problems that make Python grcat abort (unknown
// keywords, bad regexps, unknown colors) are recorded as warnings and the
// offending line or entry is skipped.
func Parse(r io.Reader, name string) (*File, error) {
	file := &File{Name: name}
	warn := func(line int, format string, args ...any) {
		file.Warnings = append(file.Warnings, fmt.Sprintf("%s:%d: ", name, line)+fmt.Sprintf(format, args...))
	}

	type entry struct {
		line   int
		values map[string]string
		lines  map[string]int
	}
	var cur *entry
	finish := func() {
		if cur == nil {
			return
		}
		e := cur
		cur = nil
		expr, ok := e.values["regexp"]
		if !ok {
			return
		}
		re, err := rx.Compile(expr)
		if err != nil {
			warn(e.lines["regexp"], "bad regexp %q: %v", expr, err)
			return
		}
		rule := Rule{Line: e.line, Regexp: re, Count: "more"}
		if v, ok := e.values["count"]; ok {
			rule.Count = v
		}
		if v, ok := e.values["colours"]; ok {
			colors, err := ParseColors(v)
			if err != nil {
				warn(e.lines["colours"], "%v", err)
			}
			rule.Colors = colors
		}
		if v, ok := e.values["replace"]; ok {
			rule.HasReplace, rule.Replace = true, rx.PythonTemplate(v)
		}
		if v, ok := e.values["command"]; ok {
			rule.HasCommand, rule.Command = true, v
		}
		if v, ok := e.values["concat"]; ok {
			rule.HasConcat, rule.Concat = true, v
		}
		if v, ok := e.values["skip"]; ok {
			rule.HasSkip, rule.Skip = true, v == "yes" || v == "1" || v == "true"
		}
		file.Rules = append(file.Rules, rule)
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		l := strings.TrimSuffix(sc.Text(), "\r")
		if l == "" {
			continue
		}
		if l[0] == '#' {
			if m := directiveRE.FindStringSubmatch(l); m != nil {
				switch strings.ToLower(m[1]) {
				case "ansi":
					mode, err := ParseAnsiMode(m[2])
					if err != nil {
						warn(n, "%v", err)
					}
					file.Ansi = mode
				default:
					warn(n, "unknown gogrc directive %q", m[1])
				}
			}
			continue
		}
		if !isLetter(l[0]) {
			finish()
			continue
		}
		key, value, ok := strings.Cut(l, "=")
		if !ok {
			warn(n, "expected keyword=value, got %q", l)
			continue
		}
		kw, ok := keywords[strings.ToLower(key)]
		if !ok {
			warn(n, "invalid keyword %q", key)
			continue
		}
		if cur == nil {
			cur = &entry{line: n, values: map[string]string{}, lines: map[string]int{}}
		}
		cur.values[kw] = value
		cur.lines[kw] = n
	}
	finish()
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return file, nil
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
