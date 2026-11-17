// SPDX-License-Identifier: GPL-2.0-or-later

package rx_test

import (
	"fmt"

	"github.com/sykesm/gogrc/internal/rx"
)

// Lookbehind is not supported by Go's regexp package; rx handles it with a
// backtracking engine. Offsets are bytes, as with regexp.
func ExampleRegexp_FindAll() {
	re := rx.MustCompile(`(?<=\s)(\d+)MB`)
	fmt.Println(re.FindAll("size 187MB", -1))
	// Output:
	// [[5 10 5 8]]
}

// PythonTemplate converts grc's replace syntax for ReplaceAll.
func ExamplePythonTemplate() {
	re := rx.MustCompile(`no answer yet for icmp_seq=(\d+)`)
	fmt.Println(re.ReplaceAll("no answer yet for icmp_seq=3", rx.PythonTemplate(`TIMEOUT \1`)))
	// Output:
	// TIMEOUT 3
}
