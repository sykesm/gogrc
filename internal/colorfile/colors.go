// SPDX-License-Identifier: GPL-2.0-or-later

package colorfile

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Default resets all attributes.
const Default = "\033[0m"

// Sentinel values produced by the previous and unchanged color names.
const (
	Prev      = "prev"
	Unchanged = "unchanged"
)

// Names maps grc color names to escape sequences.
var Names = map[string]string{
	"none":      "",
	"default":   Default,
	"bold":      "\033[1m",
	"underline": "\033[4m",
	"blink":     "\033[5m",
	"reverse":   "\033[7m",
	"concealed": "\033[8m",

	"black":   "\033[30m",
	"red":     "\033[31m",
	"green":   "\033[32m",
	"yellow":  "\033[33m",
	"blue":    "\033[34m",
	"magenta": "\033[35m",
	"cyan":    "\033[36m",
	"white":   "\033[37m",

	"on_black":   "\033[40m",
	"on_red":     "\033[41m",
	"on_green":   "\033[42m",
	"on_yellow":  "\033[43m",
	"on_blue":    "\033[44m",
	"on_magenta": "\033[45m",
	"on_cyan":    "\033[46m",
	"on_white":   "\033[47m",

	"beep":      "\007",
	"previous":  Prev,
	"unchanged": Unchanged,

	// Non-standard attributes supported by some terminals.
	"dark":          "\033[2m",
	"italic":        "\033[3m",
	"rapidblink":    "\033[6m",
	"strikethrough": "\033[9m",

	// aixterm bright colors, prefixed with the standard codes for terminals
	// that lack them.
	"bright_black":   "\033[30;90m",
	"bright_red":     "\033[31;91m",
	"bright_green":   "\033[32;92m",
	"bright_yellow":  "\033[33;93m",
	"bright_blue":    "\033[34;94m",
	"bright_magenta": "\033[35;95m",
	"bright_cyan":    "\033[36;96m",
	"bright_white":   "\033[37;97m",

	"on_bright_black":   "\033[40;100m",
	"on_bright_red":     "\033[41;101m",
	"on_bright_green":   "\033[42;102m",
	"on_bright_yellow":  "\033[43;103m",
	"on_bright_blue":    "\033[44;104m",
	"on_bright_magenta": "\033[45;105m",
	"on_bright_cyan":    "\033[46;106m",
	"on_bright_white":   "\033[47;107m",
}

// xterm 256-color names, as added in grc 1.14.
func init() {
	for i := range 256 {
		Names[fmt.Sprintf("xterm_%d", i)] = fmt.Sprintf("\033[38;5;%dm", i)
		Names[fmt.Sprintf("on_xterm_%d", i)] = fmt.Sprintf("\033[48;5;%dm", i)
	}
}

// ParseColors parses a colors value: comma-separated groups of
// whitespace-separated color names or double-quoted Python string literals.
// Unknown colors are reported and contribute nothing.
func ParseColors(value string) ([]string, error) {
	var errs []string
	groups := strings.Split(value, ",")
	colors := make([]string, len(groups))
	for i, group := range groups {
		var b strings.Builder
		for word := range strings.FieldsSeq(group) {
			c, err := color(word)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			b.WriteString(c)
		}
		colors[i] = b.String()
	}
	if errs != nil {
		return colors, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return colors, nil
}

func color(word string) (string, error) {
	if c, ok := Names[word]; ok {
		return c, nil
	}
	if len(word) >= 2 && word[0] == '"' && word[len(word)-1] == '"' {
		return UnquotePython(word[1 : len(word)-1])
	}
	return "", fmt.Errorf("bad color %q", word)
}

// UnquotePython decodes the escape sequences of a Python string literal body.
func UnquotePython(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case '\n':
		case '\\', '\'', '"':
			b.WriteByte(c)
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j := i
			for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
				j++
			}
			v, _ := strconv.ParseUint(s[i:j], 8, 32)
			b.WriteRune(rune(v))
			i = j - 1
		case 'x', 'u', 'U':
			n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[c]
			if i+1+n > len(s) {
				return "", fmt.Errorf("truncated \\%c escape in %q", c, s)
			}
			v, err := strconv.ParseUint(s[i+1:i+1+n], 16, 32)
			if err != nil || !utf8.ValidRune(rune(v)) {
				return "", fmt.Errorf("bad \\%c escape in %q", c, s)
			}
			b.WriteRune(rune(v))
			i += n
		default:
			b.WriteByte('\\')
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}
