// SPDX-License-Identifier: GPL-2.0-or-later

// Package colorize applies color file rules to lines of text.
//
// The matching and rendering logic is a faithful port of Python grcat, so
// existing color files produce byte-identical output for plain text input.
package colorize

import (
	"fmt"
	"os"
	"strings"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/rx"
)

const def = colorfile.Default

// Colorizer colors a stream of lines. It carries state between lines (block
// mode and the previous color and count) so each output stream needs its own.
type Colorizer struct {
	rules    []colorfile.Rule
	prefixed [][]string // Default + color, per rule and group
	ansi     colorfile.AnsiMode

	// RunCommand runs the command of a matching rule with a command keyword.
	// When nil, commands are ignored.
	RunCommand func(cmd string)
	// Warn reports non-fatal problems such as concat failures. When nil,
	// they are ignored.
	Warn func(msg string)

	prevColor  string
	prevCount  string
	block      bool
	blockColor string
	sgr        string // the command's active SGR state in merge mode

	cline []string // reused per-byte color buffer
}

// New returns a Colorizer for the rules in f using the given ANSI mode.
func New(f *colorfile.File, ansi colorfile.AnsiMode) *Colorizer {
	c := &Colorizer{
		rules:     f.Rules,
		prefixed:  make([][]string, len(f.Rules)),
		ansi:      ansi,
		prevColor: def,
		prevCount: "more",
	}
	for i, r := range f.Rules {
		for _, col := range r.Colors {
			c.prefixed[i] = append(c.prefixed[i], def+col)
		}
	}
	return c
}

// Line colors one line, which must not include its terminator. It returns
// false when a skip rule suppresses the line.
func (c *Colorizer) Line(line string) (string, bool) {
	var escapes []escape
	if strings.IndexByte(line, esc) >= 0 {
		switch c.ansi {
		case colorfile.AnsiNative:
			return line, true
		case colorfile.AnsiStrip:
			line, _ = splitANSI(line)
		case colorfile.AnsiMerge:
			line, escapes = splitANSI(line)
		}
	}

	line, cline, skip := c.match(line)
	if skip {
		return "", false
	}
	if escapes != nil || c.sgr != "" {
		return c.renderMerge(line, cline, escapes), true
	}
	return render(line, cline), true
}

type span struct {
	start, end int
	color      string // raw color, possibly colorfile.Prev or Unchanged
	prefixed   string // def + color
}

// match runs every rule against line and computes the escape sequence that
// should precede each byte. It may rewrite line through replace rules.
func (c *Colorizer) match(line string) (string, []string, bool) {
	var spans []span
	skip := false

	for ri := range c.rules {
		p := &c.rules[ri]
		m := matcher{re: p.Regexp, line: line, limit: matchLimit(p.Count)}
		pos := 0
		count := p.Count
		wasReplace := false
		var cur []int
		for {
			if cur = m.find(pos); cur == nil {
				break
			}
			if p.HasReplace {
				// Replace only once per line, so a replacement that matches
				// the regexp again cannot loop forever.
				if wasReplace {
					break
				}
				line = p.Regexp.ReplaceAll(line, p.Replace)
				m = matcher{re: p.Regexp, line: line, limit: m.limit}
				wasReplace = true
			}
			if p.Colors != nil {
				if count == "block" {
					c.block = true
					c.blockColor = p.Colors[0]
					count = "stop"
					break
				} else if count == "unblock" {
					c.block = false
					c.blockColor = def
					count = "stop"
				}
				spans = c.addSpans(spans, ri, cur)
				if count == "previous" {
					count = c.prevCount
				}
				if count == "stop" {
					break
				}
				if count == "more" {
					c.prevCount = "more"
					if cur[1] == pos {
						// The match consumed nothing; step past it.
						pos++
					} else {
						pos = cur[1]
					}
				} else {
					c.prevCount = "once"
					pos = len(line)
				}
			}
			if p.HasConcat {
				c.concat(p.Concat, line)
				if p.Colors == nil {
					break
				}
			}
			if p.HasCommand {
				if c.RunCommand != nil {
					c.RunCommand(p.Command)
				}
				if p.Colors == nil {
					break
				}
			}
			if p.HasSkip {
				skip = p.Skip
				if p.Colors == nil {
					break
				}
			}
			if p.Colors == nil {
				// grcat loops forever here; a replace rule would have
				// stopped at the next match.
				if wasReplace {
					cur = m.find(pos)
				}
				break
			}
			if count != "more" {
				// grcat searches again from the end of the line, which only
				// matches (forever) when the regexp matches the empty string.
				cur = nil
				break
			}
		}
		if cur != nil && count == "stop" {
			c.prevCount = "stop"
			break
		}
	}

	n := len(line)
	if cap(c.cline) < n {
		c.cline = make([]string, n, max(n, 2*cap(c.cline)))
	}
	cline := c.cline[:n]
	if c.block {
		for i := range cline {
			cline[i] = c.blockColor
		}
		return line, cline, skip
	}

	if len(spans) == 0 {
		c.prevColor = def
	}
	for i := range cline {
		cline[i] = def
	}
	first, last := false, false
	for _, s := range spans {
		start, end := max(s.start, 0), min(s.end, n)
		switch s.color {
		case colorfile.Prev:
			fill(cline[start:max(start, end)], def+c.prevColor)
		case colorfile.Unchanged:
		default:
			fill(cline[min(start, n):max(start, end)], s.prefixed)
		}
		if s.start == 0 {
			first = true
			if s.color != colorfile.Prev {
				c.prevColor = s.color
			}
		}
		if s.end == n {
			last = true
		}
	}
	if !first || !last {
		c.prevColor = def
	}
	return line, cline, skip
}

func (c *Colorizer) addSpans(spans []span, rule int, m []int) []span {
	colors, prefixed := c.rules[rule].Colors, c.prefixed[rule]
	for g := 0; g < len(m)/2; g++ {
		start, end := m[2*g], m[2*g+1]
		if start < 0 {
			continue
		}
		i := g
		if i >= len(colors) {
			i = 0
		}
		spans = append(spans, span{start, end, colors[i], prefixed[i]})
	}
	return spans
}

func (c *Colorizer) concat(path, line string) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
	if err == nil {
		_, err = f.WriteString(line + "\n")
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil && c.Warn != nil {
		c.Warn(fmt.Sprintf("concat: %v", err))
	}
}

func fill(s []string, v string) {
	for i := range s {
		s[i] = v
	}
}

// render writes line with an escape sequence wherever the color changes,
// followed by a reset.
func render(line string, cline []string) string {
	var b strings.Builder
	b.Grow(len(line) + 32)
	prev := ""
	for i := 0; i < len(line); i++ {
		if cline[i] != prev {
			b.WriteString(cline[i])
			prev = cline[i]
		}
		b.WriteByte(line[i])
	}
	b.WriteString(def)
	return b.String()
}

// matcher emulates Python's pattern.search(line, pos) over the successive
// matches of a regexp, so lookbehind and ^ see the whole line.
type matcher struct {
	re    *rx.Regexp
	line  string
	limit int // maximum number of matches needed, or -1 for all
	all   [][]int
	done  bool
	next  int
}

// matchLimit returns how many matches a rule with the given count can use:
// once, stop, block and unblock never look past the first.
func matchLimit(count string) int {
	switch count {
	case "once", "stop", "block", "unblock":
		return 1
	}
	return -1
}

func (m *matcher) find(pos int) []int {
	if !m.done {
		m.all = m.re.FindAll(m.line, m.limit)
		m.done = true
	}
	for m.next < len(m.all) && m.all[m.next][0] < pos {
		m.next++
	}
	if m.next == len(m.all) {
		return nil
	}
	return m.all[m.next]
}
