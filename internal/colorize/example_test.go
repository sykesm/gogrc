// SPDX-License-Identifier: GPL-2.0-or-later

package colorize_test

import (
	"fmt"
	"strings"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/colorize"
)

// A color file colors the whole match with the first color and each group
// with the colors that follow; unchanged leaves a part as it was.
func Example() {
	f, err := colorfile.Parse(strings.NewReader(`
regexp=(\d+)\((\w+)\)
colours=unchanged,yellow,bold yellow
`), "conf.id")
	if err != nil {
		panic(err)
	}
	c := colorize.New(f, colorfile.AnsiNative)
	out, _ := c.Line("uid=501(alice)")
	fmt.Printf("%q\n", out)
	// Output:
	// "\x1b[0muid=\x1b[0m\x1b[33m501\x1b[0m(\x1b[0m\x1b[1m\x1b[33malice\x1b[0m)\x1b[0m"
}

// In merge mode, gogrc's colors replace the command's only where a rule
// matches.
func ExampleColorizer_Line_merge() {
	f, _ := colorfile.Parse(strings.NewReader("regexp=FAIL\ncolours=red\n"), "conf.test")
	c := colorize.New(f, colorfile.AnsiMerge)
	out, _ := c.Line("\x1b[32mok\x1b[0m FAIL")
	fmt.Printf("%q\n", out)
	// Output:
	// "\x1b[0m\x1b[32mok\x1b[0m \x1b[0m\x1b[31mFAIL\x1b[0m"
}
