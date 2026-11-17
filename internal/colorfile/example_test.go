// SPDX-License-Identifier: GPL-2.0-or-later

package colorfile_test

import (
	"fmt"
	"strings"

	"github.com/sykesm/gogrc/internal/colorfile"
)

// Entries are separated by any line not starting with a letter. A
// "#gogrc:" directive is a comment to grc but sets options for gogrc.
func ExampleParse() {
	f, err := colorfile.Parse(strings.NewReader(`#gogrc: ansi=merge
# Errors
regexp=ERROR.*
colours=bold red
count=stop
=====
regexp=bogus(
colours=green
`), "conf.example")
	if err != nil {
		panic(err)
	}
	fmt.Println("ansi:", f.Ansi)
	for _, r := range f.Rules {
		fmt.Printf("line %d: %s count=%s colors=%q\n", r.Line, r.Regexp, r.Count, r.Colors)
	}
	fmt.Println(len(f.Warnings), "warning")
	// Output:
	// ansi: merge
	// line 3: ERROR.* count=stop colors=["\x1b[1m\x1b[31m"]
	// 1 warning
}
