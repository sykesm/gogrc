// SPDX-License-Identifier: GPL-2.0-or-later

package colorize

import (
	"math"
	"strings"
)

const esc = 0x1b

// escape is an escape sequence removed from a line, positioned by its byte
// offset in the stripped line.
type escape struct {
	pos int
	seq string
}

// splitANSI removes escape sequences from s.
func splitANSI(s string) (string, []escape) {
	var b strings.Builder
	var escapes []escape
	for i := 0; i < len(s); {
		if s[i] != esc {
			b.WriteByte(s[i])
			i++
			continue
		}
		n := escapeLen(s[i:])
		escapes = append(escapes, escape{b.Len(), s[i : i+n]})
		i += n
	}
	return b.String(), escapes
}

// escapeLen returns the length of the escape sequence at the start of s.
func escapeLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[': // CSI: parameters and intermediates, then a final byte.
		j := 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
			j++
		}
		return j
	case ']', 'P', 'X', '^', '_': // String sequences end with BEL or ST.
		for j := 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == esc && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
		return len(s)
	default:
		// ESC with intermediates, e.g. the charset designation ESC ( B.
		if s[1] >= 0x20 && s[1] <= 0x2f && len(s) >= 3 {
			return 3
		}
		return 2
	}
}

// isSGR reports whether seq sets graphic rendition (colors and attributes).
func isSGR(seq string) bool {
	if len(seq) < 3 || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return false
	}
	for _, c := range seq[2 : len(seq)-1] {
		if (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// applySGR returns the SGR state after seq is applied to state.
func applySGR(state, seq string) string {
	params := seq[2 : len(seq)-1]
	switch {
	case params == "" || params == "0":
		return ""
	case strings.HasPrefix(params, "0;"):
		return "\033[" + params[2:] + "m"
	}
	return state + seq
}

// renderMerge renders line like render, re-inserting the command's escape
// sequences. Bytes gogrc leaves uncolored keep the command's colors.
func (c *Colorizer) renderMerge(line string, cline []string, escapes []escape) string {
	var b strings.Builder
	b.Grow(len(line) + 64)
	prev := ""
	ei := 0
	apply := func(pos int) {
		for ; ei < len(escapes) && escapes[ei].pos <= pos; ei++ {
			if seq := escapes[ei].seq; isSGR(seq) {
				c.sgr = applySGR(c.sgr, seq)
				prev = "" // force the color for the next byte to be written
			} else {
				b.WriteString(seq)
			}
		}
	}
	for i := 0; i < len(line); i++ {
		apply(i)
		want := cline[i]
		if want == def && c.sgr != "" {
			want = def + c.sgr
		}
		if want != prev {
			b.WriteString(want)
			prev = want
		}
		b.WriteByte(line[i])
	}
	// Trailing sequences, and any left beyond the end by a replace rule.
	apply(math.MaxInt)
	b.WriteString(def)
	return b.String()
}
