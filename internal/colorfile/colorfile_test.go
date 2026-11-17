// SPDX-License-Identifier: GPL-2.0-or-later

package colorfile

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	src := `# comment
#gogrc: ansi=merge
regexp=(\d+)\(([\w.]+)\)
colours=unchanged,yellow,bold yellow
-
# User
REGEXP=uid.(\d+)
Colour=green, "\033[38;5;140m"
count=once
=====
colors=red
=====
regexp=no answer yet for icmp_seq=(\d+)
replace=TIMEOUT \1
skip=yes
`
	f, err := Parse(strings.NewReader(src), "conf.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Warnings) != 0 {
		t.Fatalf("warnings: %v", f.Warnings)
	}
	if f.Ansi != AnsiMerge {
		t.Errorf("Ansi = %v, want merge", f.Ansi)
	}
	if len(f.Rules) != 3 {
		t.Fatalf("got %d rules, want 3 (entry without regexp is dropped)", len(f.Rules))
	}

	r := f.Rules[0]
	if r.Regexp.String() != `(\d+)\(([\w.]+)\)` || r.Count != "more" || r.Line != 3 {
		t.Errorf("rule 0 = %+v", r)
	}
	if want := []string{Unchanged, "\033[33m", "\033[1m\033[33m"}; !reflect.DeepEqual(r.Colors, want) {
		t.Errorf("rule 0 colors = %q, want %q", r.Colors, want)
	}

	r = f.Rules[1]
	if r.Count != "once" {
		t.Errorf("rule 1 count = %q", r.Count)
	}
	if want := []string{"\033[32m", "\033[38;5;140m"}; !reflect.DeepEqual(r.Colors, want) {
		t.Errorf("rule 1 colors = %q, want %q", r.Colors, want)
	}

	r = f.Rules[2]
	if r.Colors != nil || !r.HasReplace || r.Replace != "TIMEOUT ${1}" || !r.HasSkip || !r.Skip {
		t.Errorf("rule 2 = %+v", r)
	}
}

func TestParseWarnings(t *testing.T) {
	src := "regexp=(oops\ncolours=red\n-\nbogus=1\nregexp=ok\ncolours=chartreuse\nnoequals\n"
	f, err := Parse(strings.NewReader(src), "conf.bad")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Rules) != 1 || f.Rules[0].Regexp.String() != "ok" {
		t.Fatalf("rules = %+v", f.Rules)
	}
	if len(f.Warnings) != 4 {
		t.Errorf("warnings = %q", f.Warnings)
	}
}

func TestUnquotePython(t *testing.T) {
	tests := map[string]string{
		`\033[1m`:   "\033[1m",
		`\x1b[2m`:   "\x1b[2m",
		`\u001b`:    "\x1b",
		`a\qb`:      `a\qb`,
		`tab\there`: "tab\there",
	}
	for in, want := range tests {
		got, err := UnquotePython(in)
		if err != nil || got != want {
			t.Errorf("UnquotePython(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := UnquotePython(`\x1`); err == nil {
		t.Error("expected error for truncated escape")
	}
}
