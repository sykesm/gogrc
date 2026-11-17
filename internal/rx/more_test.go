// SPDX-License-Identifier: GPL-2.0-or-later

package rx

import (
	"strings"
	"testing"
)

func TestMustCompilePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustCompile did not panic on a bad expression")
		}
	}()
	MustCompile(`(`)
}

func TestStringAndMatchString(t *testing.T) {
	r := MustCompile(`(?<=a)b`)
	if r.String() != `(?<=a)b` {
		t.Errorf("String() = %q", r.String())
	}
	if !r.MatchString("ab") || r.MatchString("cb") {
		t.Error("MatchString gave the wrong answer")
	}
}

func TestReplaceAllEngines(t *testing.T) {
	tests := []struct {
		expr, s, tmpl, want string
	}{
		{`(\d+)`, "a1b22", "<${1}>", "a<1>b<22>"},  // RE2
		{`(?<=a)(\d)`, "a1b2", "[${1}]", "a[1]b2"}, // regexp2: lookbehind
		{`(\w+)`, "é x", "<${1}>", "<é> <x>"},      // regexp2: non-ASCII
		{`(?P<n>x)`, "axb", "${n}${n}", "axxb"},    // named group
		{`x`, "a$x", "$$", "a$$"},                  // literal dollar
	}
	for _, tt := range tests {
		if got := MustCompile(tt.expr).ReplaceAll(tt.s, tt.tmpl); got != tt.want {
			t.Errorf("%q.ReplaceAll(%q, %q) = %q, want %q", tt.expr, tt.s, tt.tmpl, got, tt.want)
		}
	}
}

func TestReplaceAllTimeout(t *testing.T) {
	// Catastrophic backtracking hits the match timeout; the input comes back
	// unchanged instead of hanging.
	r := MustCompile(`(?<=x)(a+)+$`)
	r.backtracker().MatchTimeout = 1
	s := "x" + strings.Repeat("a", 30) + "!"
	if got := r.ReplaceAll(s, "y"); got != s {
		t.Errorf("got %q", got)
	}
	if got := r.FindAll(s, -1); got != nil {
		t.Errorf("FindAll = %v, want no matches", got)
	}
}

func TestFindAllLimit(t *testing.T) {
	r := MustCompile(`(?<=-)\d`)
	if got := r.FindAll("-1-2-3", 2); len(got) != 2 {
		t.Errorf("got %d matches, want 2", len(got))
	}
}

func TestPythonToDotNet(t *testing.T) {
	tests := map[string]string{
		`plain`:          `plain`,
		`(?P<a>x)(?P=a)`: `(?<a>x)\k<a>`,
		`\(?P<a>x)`:      `\(?P<a>x)`, // escaped paren is not a group
		`(?P<a>x)(?P=a`:  `(?<a>x)(?P=a`,
		`(?P<a>x)\`:      `(?<a>x)\`,
	}
	for in, want := range tests {
		if got := pythonToDotNet(in); got != want {
			t.Errorf("pythonToDotNet(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPythonTemplateUnterminatedGroup(t *testing.T) {
	if got := PythonTemplate(`\g<1`); got != `\g<1` {
		t.Errorf("got %q", got)
	}
}
