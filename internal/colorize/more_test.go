// SPDX-License-Identifier: GPL-2.0-or-later

package colorize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sykesm/gogrc/internal/colorfile"
)

func TestConcat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	c := newColorizer(t, "regexp=ERROR\nconcat="+path+"\n", colorfile.AnsiNative)
	c.Line("first ERROR")
	c.Line("fine")
	c.Line("second ERROR")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first ERROR\nsecond ERROR\n" {
		t.Errorf("concat file = %q", data)
	}

	var warnings []string
	c = newColorizer(t, "regexp=x\ncolours=red\nconcat="+t.TempDir()+"\n", colorfile.AnsiNative)
	c.Warn = func(msg string) { warnings = append(warnings, msg) }
	check(t, c, "x", def+red+"x"+def)
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "concat:") {
		t.Errorf("warnings = %q", warnings)
	}
	c.Warn = nil
	c.Line("x") // no Warn: the failure is ignored
}

func TestCommand(t *testing.T) {
	var ran []string
	c := newColorizer(t, "regexp=deploy\ncommand=notify\n-\nregexp=x\ncolours=red\ncommand=beep\n", colorfile.AnsiNative)
	c.RunCommand = func(cmd string) { ran = append(ran, cmd) }
	c.Line("deploy x x")
	// A rule without colors runs once; with colors, once per match.
	if strings.Join(ran, ",") != "notify,beep,beep" {
		t.Errorf("ran %q", ran)
	}
	c.RunCommand = nil
	c.Line("deploy") // no RunCommand: commands are ignored
}

func TestSkipWithColors(t *testing.T) {
	c := newColorizer(t, "regexp=x\ncolours=red\nskip=yes\n-\nregexp=y\ncolours=red\nskip=no\n", colorfile.AnsiNative)
	if _, ok := c.Line("x"); ok {
		t.Error("line with x not skipped")
	}
	// A later skip=no rule that matches clears the skip.
	check(t, c, "x y", def+red+"x"+def+" "+def+red+"y"+def)
}

func TestUnknownCountActsLikeOnce(t *testing.T) {
	c := newColorizer(t, "regexp=x\ncolours=red\ncount=sometimes\n", colorfile.AnsiNative)
	check(t, c, "xx", def+red+"x"+def+"x"+def)
}

func TestPreviousCount(t *testing.T) {
	c := newColorizer(t, "regexp=a\ncolours=red\ncount=stop\n-\nregexp=b\ncolours=green\ncount=previous\n-\nregexp=c\ncolours=red\n", colorfile.AnsiNative)
	// count=previous takes the count of the previous match: after a stop,
	// b stops too, so c is not colored.
	check(t, c, "a", def+red+"a"+def)
	check(t, c, "bc", def+green+"b"+def+"c"+def)
	// The stop carries over to later lines through count=previous: b is
	// colored once and stops again, so c is never reached (verified against
	// Python grcat).
	check(t, c, "cbb", def+"c"+def+green+"b"+def+"b"+def)
}

func TestReplaceWithoutColorsAndStop(t *testing.T) {
	// The stop only applies when the rewritten line still matches.
	c := newColorizer(t, "regexp=a\nreplace=b\ncount=stop\n-\nregexp=b\ncolours=red\n", colorfile.AnsiNative)
	check(t, c, "a", def+red+"b"+def)
	c = newColorizer(t, "regexp=a+\nreplace=a\ncount=stop\n-\nregexp=a\ncolours=red\n", colorfile.AnsiNative)
	check(t, c, "aa", def+"a"+def)
}

func TestReplaceAndColorsMore(t *testing.T) {
	c := newColorizer(t, "regexp=(\\d+)\nreplace=#\\1\ncolours=green\n", colorfile.AnsiNative)
	// Every match is replaced, but only the first is colored, at its
	// position in the original line (verified against Python grcat).
	check(t, c, "a1 b2", def+"a"+def+green+"#"+def+"1 b#2"+def)
}

func TestBlockWithNoneColor(t *testing.T) {
	c := newColorizer(t, "regexp=BEGIN\ncolours=none\ncount=block\n", colorfile.AnsiNative)
	check(t, c, "BEGIN", "BEGIN"+def)
}

func TestMergeEdgeCases(t *testing.T) {
	c := newColorizer(t, "regexp=x\ncolours=red\n", colorfile.AnsiMerge)
	// Non-SGR sequences stay in place; "0;" resets the command's state.
	check(t, c, "\033[2Kx\033[0;32my", "\033[2K"+def+red+"x"+def+green+"y"+def)
	// Escapes left beyond the end by a replace rule are still written.
	c = newColorizer(t, "regexp=long\nreplace=s\n", colorfile.AnsiMerge)
	check(t, c, "long\033[K", def+"s\033[K"+def)
}

func TestEscapeLen(t *testing.T) {
	tests := map[string]int{
		"\033":           1,
		"\033[":          2,
		"\033[12":        4, // no final byte
		"\033]0;t\033\\": 7,
		"\033]0;t":       5, // unterminated string
		"\033(":          2,
		"\033(B":         3,
		"\033c":          2,
	}
	for in, want := range tests {
		if got := escapeLen(in); got != want {
			t.Errorf("escapeLen(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIsSGR(t *testing.T) {
	for seq, want := range map[string]bool{
		"\033[m": true, "\033[1;38:5:2m": true, "\033[2K": false, "\033[?1m": false, "\033]": false,
	} {
		if got := isSGR(seq); got != want {
			t.Errorf("isSGR(%q) = %v", seq, got)
		}
	}
}
