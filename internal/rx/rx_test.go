// SPDX-License-Identifier: GPL-2.0-or-later

package rx

import (
	"reflect"
	"testing"
)

func TestFindAllEngines(t *testing.T) {
	tests := []struct {
		expr string
		s    string
		want [][]int
	}{
		// RE2 path.
		{`(\d+)x`, "1x 22x", [][]int{{0, 2, 0, 1}, {3, 6, 3, 5}}},
		{`a(b)?`, "a", [][]int{{0, 1, -1, -1}}},
		// Lookbehind forces regexp2; offsets stay in bytes.
		{`(?<=\s)latest`, "img latest", [][]int{{4, 10}}},
		{`(?<=\s)(\w+)`, "é ünï", [][]int{{3, 8, 3, 8}}},
		// Non-ASCII subject with an RE2-compatible expression uses regexp2,
		// so \w is Unicode-aware like Python.
		{`\w+`, "naïve", [][]int{{0, 6}}},
		// Python treats [\b] as backspace.
		{`[:\b]x`, "a\bx", [][]int{{1, 3}}},
		{`(?P<n>a)(?P=n)`, "xaa", [][]int{{1, 3, 1, 2}}},
	}
	for _, tt := range tests {
		r, err := Compile(tt.expr)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.expr, err)
		}
		if got := r.FindAll(tt.s, -1); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q.FindAll(%q) = %v, want %v", tt.expr, tt.s, got, tt.want)
		}
	}
}

func TestInvalidUTF8Offsets(t *testing.T) {
	r := MustCompile(`(?<=a.)b`)
	if got := r.FindAll("a\xffb", -1); !reflect.DeepEqual(got, [][]int{{2, 3}}) {
		t.Errorf("got %v", got)
	}
}

func TestCompileError(t *testing.T) {
	if _, err := Compile(`(unclosed`); err == nil {
		t.Error("expected error")
	}
}

func TestPythonTemplate(t *testing.T) {
	tests := map[string]string{
		`TIMEOUT \1`:  `TIMEOUT ${1}`,
		`\g<2>-\g<x>`: `${2}-${x}`,
		`\12`:         `${12}`,
		`cost $5\t`:   "cost $$5\t",
		`\&`:          `\&`,
		`nul\0end`:    "nul\x00end",
		`trailing\`:   `trailing\`,
		`back\\slash`: `back\slash`,
	}
	for in, want := range tests {
		if got := PythonTemplate(in); got != want {
			t.Errorf("PythonTemplate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReplaceAll(t *testing.T) {
	r := MustCompile(`no answer yet for icmp_seq=(\d+)`)
	got := r.ReplaceAll("Request timeout: no answer yet for icmp_seq=4", PythonTemplate(`TIMEOUT \1`))
	if got != "Request timeout: TIMEOUT 4" {
		t.Errorf("got %q", got)
	}
}
