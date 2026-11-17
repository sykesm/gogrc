// SPDX-License-Identifier: GPL-2.0-or-later

package colorfile

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	"github.com/sykesm/gogrc/internal/defaults"
)

// TestStockFilesParse checks that every stock color file parses cleanly.
// conf.dummy is a deliberately odd example that grcat rejects too.
func TestStockFilesParse(t *testing.T) {
	for _, name := range defaults.Names() {
		if !strings.HasPrefix(name, "conf.") {
			continue
		}
		data, err := fs.ReadFile(defaults.FS(), name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Parse(bytes.NewReader(data), name)
		if err != nil {
			t.Fatal(err)
		}
		if name == "conf.dummy" {
			continue
		}
		for _, w := range f.Warnings {
			t.Error(w)
		}
	}
}
