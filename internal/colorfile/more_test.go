// SPDX-License-Identifier: GPL-2.0-or-later

package colorfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnsiMode(t *testing.T) {
	for _, mode := range []AnsiMode{AnsiNative, AnsiStrip, AnsiMerge} {
		got, err := ParseAnsiMode(strings.ToUpper(mode.String()))
		if err != nil || got != mode {
			t.Errorf("ParseAnsiMode(%q) = %v, %v", mode.String(), got, err)
		}
	}
	if _, err := ParseAnsiMode("loud"); err == nil {
		t.Error("ParseAnsiMode accepted an invalid mode")
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.x")
	if err := os.WriteFile(path, []byte("regexp=x\ncolor=xterm_208\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != path || len(f.Rules) != 1 || f.Rules[0].Colors[0] != "\033[38;5;208m" {
		t.Errorf("Load = %+v", f)
	}
	if _, err := Load(path + ".missing"); err == nil {
		t.Error("Load of a missing file succeeded")
	}
}

func TestDirectiveWarnings(t *testing.T) {
	f, err := Parse(strings.NewReader("#gogrc: ansi=loud\n#gogrc: shout=yes\n# gogrc is great\n"), "conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Warnings) != 2 || f.Ansi != AnsiNative {
		t.Errorf("warnings = %q, ansi = %v", f.Warnings, f.Ansi)
	}
}

func TestParseLineTooLong(t *testing.T) {
	long := "regexp=" + strings.Repeat("x", 2*1024*1024) + "\n"
	if _, err := Parse(strings.NewReader(long), "conf"); err == nil {
		t.Error("Parse accepted a line longer than the scanner buffer")
	}
}

func TestCRLFAndRepeatedKeywords(t *testing.T) {
	f, err := Parse(strings.NewReader("regexp=a\r\nregexp=b\r\n\r\ncolours=red\r\n"), "conf")
	if err != nil {
		t.Fatal(err)
	}
	// The later regexp wins, and the blank line does not end the entry.
	if len(f.Rules) != 1 || f.Rules[0].Regexp.String() != "b" || f.Rules[0].Colors[0] != "\033[31m" {
		t.Errorf("rules = %+v", f.Rules)
	}
}

func TestUnquotePythonEscapes(t *testing.T) {
	tests := map[string]string{
		`\\ \' \"`:        `\ ' "`,
		`\a\b\f\n\r\t\v`:  "\a\b\f\n\r\t\v",
		`\101\0`:          "A\x00",
		`\x41é\U0001F600`: "Aé😀",
		"line\\\ncont":    "linecont",
		`end\`:            `end\`,
	}
	for in, want := range tests {
		got, err := UnquotePython(in)
		if err != nil || got != want {
			t.Errorf("UnquotePython(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`\xzz`, `\ud800`, `\U0011FFFF`} {
		if _, err := UnquotePython(bad); err == nil {
			t.Errorf("UnquotePython(%q) succeeded", bad)
		}
	}
}

func TestParseColorsQuoted(t *testing.T) {
	colors, err := ParseColors(`"\033[38;5;140m" bold, "\xzz"`)
	if err == nil {
		t.Error("expected an error for the bad escape")
	}
	if colors[0] != "\033[38;5;140m\033[1m" || colors[1] != "" {
		t.Errorf("colors = %q", colors)
	}
}
