// SPDX-License-Identifier: GPL-2.0-or-later

package colorize

import (
	"strings"
	"testing"

	"github.com/sykesm/gogrc/internal/colorfile"
)

const (
	red   = "\033[31m"
	green = "\033[32m"
	bold  = "\033[1m"
)

func newColorizer(t *testing.T, src string, mode colorfile.AnsiMode) *Colorizer {
	t.Helper()
	f, err := colorfile.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Warnings) > 0 {
		t.Fatalf("warnings: %v", f.Warnings)
	}
	return New(f, mode)
}

func check(t *testing.T, c *Colorizer, in, want string) {
	t.Helper()
	got, ok := c.Line(in)
	if !ok {
		t.Fatalf("Line(%q) skipped", in)
	}
	if got != want {
		t.Errorf("Line(%q)\n got %q\nwant %q", in, got, want)
	}
}

func TestBasicMatching(t *testing.T) {
	c := newColorizer(t, "regexp=b\ncolours=red\n", colorfile.AnsiNative)
	// Matches the grcat output observed byte-for-byte.
	check(t, c, "abcb", def+"a"+def+red+"b"+def+"c"+def+red+"b"+def)
	check(t, c, "", def)
	check(t, c, "a\rb\r", def+"a\r"+def+red+"b"+def+"\r"+def)
}

func TestGroupsAndUnchanged(t *testing.T) {
	c := newColorizer(t, "regexp=(\\d+)\\((\\w+)\\)\ncolours=unchanged,green,bold red\n", colorfile.AnsiNative)
	check(t, c, "uid=501(me)", def+"uid="+def+green+"501"+def+"("+def+bold+red+"me"+def+")"+def)
}

func TestMissingGroupColorsUseGroupZero(t *testing.T) {
	c := newColorizer(t, "regexp=a(b)\ncolours=red\n", colorfile.AnsiNative)
	check(t, c, "ab", def+red+"ab"+def)
}

func TestCountOnceAndStop(t *testing.T) {
	c := newColorizer(t, "regexp=x\ncolours=red\ncount=once\n-\nregexp=y\ncolours=green\ncount=stop\n-\nregexp=z\ncolours=red\n", colorfile.AnsiNative)
	check(t, c, "xx", def+red+"x"+def+"x"+def)
	// stop prevents later rules from running.
	check(t, c, "yz", def+green+"y"+def+"z"+def)
	check(t, c, "z", def+red+"z"+def)
}

func TestBlock(t *testing.T) {
	c := newColorizer(t, "regexp=BEGIN\ncolours=red\ncount=block\n-\nregexp=END\ncolours=green\ncount=unblock\n", colorfile.AnsiNative)
	check(t, c, "a", def+"a"+def)
	check(t, c, "BEGIN x", red+"BEGIN x"+def)
	check(t, c, "inside", red+"inside"+def)
	check(t, c, "END", def+green+"END"+def)
	check(t, c, "after", def+"after"+def)
}

func TestPrevious(t *testing.T) {
	c := newColorizer(t, "regexp=^E.*\ncolours=red\n-\nregexp=^\\s+.*\ncolours=previous\n", colorfile.AnsiNative)
	check(t, c, "Error", def+red+"Error"+def)
	check(t, c, "  more", def+red+"  more"+def)
	check(t, c, "plain", def+"plain"+def)
	// grcat prefixes the reset previous color with another reset.
	check(t, c, "  more", def+def+"  more"+def)
}

func TestReplaceAndSkip(t *testing.T) {
	c := newColorizer(t, "regexp=seq=(\\d+)\nreplace=TIMEOUT \\1\n-\nregexp=TIMEOUT\ncolours=red\n-\nregexp=^hide\nskip=yes\n", colorfile.AnsiNative)
	check(t, c, "no seq=4", def+"no "+def+red+"TIMEOUT"+def+" 4"+def)
	if _, ok := c.Line("hide me"); ok {
		t.Error("expected skip")
	}
}

func TestEmptyMatchesTerminate(t *testing.T) {
	// grcat hangs on these; gogrc must not.
	c := newColorizer(t, "regexp=x*\ncolours=red\ncount=once\n-\nregexp=y*\n", colorfile.AnsiNative)
	check(t, c, "ab", def+"ab"+def)
}

func TestLookbehindOffsets(t *testing.T) {
	c := newColorizer(t, "regexp=(?<=\\s)latest\ncolours=red\n", colorfile.AnsiNative)
	check(t, c, "é latest", def+"é "+def+red+"latest"+def)
}

func TestAnsiModes(t *testing.T) {
	const conf = "regexp=ERR\ncolours=red\n"
	in := "\033[1mERR\033[0m ok \033[32mgo\033[0m"

	native := newColorizer(t, conf, colorfile.AnsiNative)
	check(t, native, in, in)

	strip := newColorizer(t, conf, colorfile.AnsiStrip)
	check(t, strip, in, def+red+"ERR"+def+" ok go"+def)

	merge := newColorizer(t, conf, colorfile.AnsiMerge)
	check(t, merge, in, def+red+"ERR"+def+" ok "+def+green+"go"+def)

	// The command's colors carry over to the next line.
	merge = newColorizer(t, conf, colorfile.AnsiMerge)
	check(t, merge, "\033[32mstart", def+green+"start"+def)
	check(t, merge, "end ERR", def+green+"end "+def+red+"ERR"+def)
}

func TestSplitANSI(t *testing.T) {
	s, escapes := splitANSI("a\033[1;31mb\033]0;title\007c\033(Bd\033[K")
	if s != "abcd" {
		t.Errorf("stripped = %q", s)
	}
	want := []escape{{1, "\033[1;31m"}, {2, "\033]0;title\007"}, {3, "\033(B"}, {4, "\033[K"}}
	if len(escapes) != len(want) {
		t.Fatalf("escapes = %+v", escapes)
	}
	for i := range want {
		if escapes[i] != want[i] {
			t.Errorf("escape %d = %+v, want %+v", i, escapes[i], want[i])
		}
	}
}
