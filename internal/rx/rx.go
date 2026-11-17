// SPDX-License-Identifier: GPL-2.0-or-later

// Package rx provides regular expressions that behave like Python's re module
// for the patterns found in grc configuration files.
//
// Go's RE2 engine is used whenever it can compile the expression and the
// subject is ASCII. Expressions RE2 cannot compile (lookaround, [\b], ...) and
// subjects containing non-ASCII bytes are handled by regexp2, a backtracking
// engine whose \w, \d, \s and \b are Unicode-aware like Python's.
package rx

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
)

// matchTimeout bounds catastrophic backtracking in the regexp2 engine. A
// timed-out match is treated as no match.
const matchTimeout = 2 * time.Second

// Regexp is a compiled expression.
type Regexp struct {
	expr string
	re2  *regexp.Regexp // nil when RE2 cannot compile expr

	btOnce sync.Once
	bt     *regexp2.Regexp // backtracking engine, compiled lazily
	btErr  error
}

// Compile compiles a Python-syntax regular expression.
func Compile(expr string) (*Regexp, error) {
	r := &Regexp{expr: expr}
	re, err := regexp.Compile(expr)
	if err == nil {
		r.re2 = re
		return r, nil
	}
	if bt := r.backtracker(); bt == nil {
		return nil, r.btErr
	}
	return r, nil
}

// MustCompile is like Compile but panics on error.
func MustCompile(expr string) *Regexp {
	r, err := Compile(expr)
	if err != nil {
		panic(err)
	}
	return r
}

// String returns the source expression.
func (r *Regexp) String() string { return r.expr }

func (r *Regexp) backtracker() *regexp2.Regexp {
	r.btOnce.Do(func() {
		bt, err := regexp2.Compile(pythonToDotNet(r.expr), regexp2.None)
		if err != nil {
			r.btErr = err
			return
		}
		bt.MatchTimeout = matchTimeout
		r.bt = bt
	})
	return r.bt
}

// useBacktracker reports whether s should be matched with regexp2.
func (r *Regexp) useBacktracker(s string) bool {
	if r.re2 == nil {
		return true
	}
	return !isASCII(s) && r.backtracker() != nil
}

// MatchString reports whether s contains a match.
func (r *Regexp) MatchString(s string) bool {
	return len(r.FindAll(s, 1)) > 0
}

// FindAll returns up to n successive non-overlapping matches (all when n < 0).
// Each match is a slice of byte offset pairs, one pair per group starting with
// the whole match; groups that did not participate are -1, -1.
func (r *Regexp) FindAll(s string, n int) [][]int {
	if !r.useBacktracker(s) {
		return r.re2.FindAllStringSubmatchIndex(s, n)
	}

	runes := []rune(s)
	offsets := runeOffsets(s, len(runes))
	var out [][]int
	m, err := r.bt.FindRunesMatch(runes)
	for err == nil && m != nil && (n < 0 || len(out) < n) {
		groups := m.Groups()
		idx := make([]int, 2*len(groups))
		for i, g := range groups {
			if len(g.Captures) == 0 {
				idx[2*i], idx[2*i+1] = -1, -1
				continue
			}
			idx[2*i] = offsets[g.Index]
			idx[2*i+1] = offsets[g.Index+g.Length]
		}
		out = append(out, idx)
		m, err = r.bt.FindNextMatch(m)
	}
	return out
}

// ReplaceAll replaces every match in s with tmpl, a template in Go's
// expansion syntax (see PythonTemplate).
func (r *Regexp) ReplaceAll(s, tmpl string) string {
	if !r.useBacktracker(s) {
		return r.re2.ReplaceAllString(s, tmpl)
	}
	out, err := r.bt.Replace(s, tmpl, -1, -1)
	if err != nil {
		return s
	}
	return out
}

// runeOffsets maps rune indexes of s (as produced by []rune(s)) to byte
// offsets. Invalid UTF-8 bytes count as one rune each, matching Go's
// conversion.
func runeOffsets(s string, n int) []int {
	offsets := make([]int, 0, n+1)
	for i := 0; i < len(s); {
		offsets = append(offsets, i)
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return append(offsets, len(s))
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// pythonToDotNet rewrites the Python-only group syntax that regexp2 does not
// understand.
func pythonToDotNet(expr string) string {
	if !strings.Contains(expr, "(?P") {
		return expr
	}
	var b strings.Builder
	for i := 0; i < len(expr); i++ {
		switch {
		case expr[i] == '\\' && i+1 < len(expr):
			b.WriteString(expr[i : i+2])
			i++
		case strings.HasPrefix(expr[i:], "(?P<"):
			b.WriteString("(?<")
			i += 3
		case strings.HasPrefix(expr[i:], "(?P="):
			end := strings.IndexByte(expr[i:], ')')
			if end < 0 {
				b.WriteString(expr[i:])
				return b.String()
			}
			b.WriteString(`\k<`)
			b.WriteString(expr[i+4 : i+end])
			b.WriteByte('>')
			i += end
		default:
			b.WriteByte(expr[i])
		}
	}
	return b.String()
}

// PythonTemplate converts a Python re.sub replacement template (\1, \g<1>,
// \g<name>, \n, ...) into the $-based syntax used by both Go and regexp2.
func PythonTemplate(py string) string {
	var b strings.Builder
	for i := 0; i < len(py); i++ {
		c := py[i]
		if c == '$' {
			b.WriteString("$$")
			continue
		}
		if c != '\\' || i+1 == len(py) {
			b.WriteByte(c)
			continue
		}
		i++
		switch e := py[i]; {
		case e == 'g' && i+1 < len(py) && py[i+1] == '<':
			end := strings.IndexByte(py[i:], '>')
			if end < 0 {
				b.WriteString(`\g`)
				continue
			}
			writeGroupRef(&b, py[i+2:i+end])
			i += end
		case e >= '1' && e <= '9':
			j := i + 1
			if j < len(py) && py[j] >= '0' && py[j] <= '9' {
				j++
			}
			writeGroupRef(&b, py[i:j])
			i = j - 1
		case e == '0':
			// Octal escape: \0, \01, \012.
			v, j := 0, i
			for j < len(py) && j < i+3 && py[j] >= '0' && py[j] <= '7' {
				v = v*8 + int(py[j]-'0')
				j++
			}
			b.WriteByte(byte(v))
			i = j - 1
		default:
			if r, ok := simpleEscapes[e]; ok {
				b.WriteByte(r)
			} else {
				b.WriteByte('\\')
				b.WriteByte(e)
			}
		}
	}
	return b.String()
}

// writeGroupRef writes a reference to a group by number or name.
func writeGroupRef(b *strings.Builder, group string) {
	b.WriteString("${")
	b.WriteString(group)
	b.WriteByte('}')
}

// simpleEscapes maps the single-character escapes Python templates accept.
var simpleEscapes = map[byte]byte{
	'\\': '\\', 'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v',
}
