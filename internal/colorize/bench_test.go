// SPDX-License-Identifier: GPL-2.0-or-later

package colorize

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/defaults"
)

func BenchmarkLine(b *testing.B) {
	for _, name := range []string{"conf.log", "conf.ps", "conf.dockerimages"} {
		b.Run(name, func(b *testing.B) {
			data, err := fs.ReadFile(defaults.FS(), name)
			if err != nil {
				b.Fatal(err)
			}
			f, err := colorfile.Parse(strings.NewReader(string(data)), name)
			if err != nil {
				b.Fatal(err)
			}
			lines := strings.Split(string(readCorpus(b, "../../testdata/corpus")), "\n")
			c := New(f, colorfile.AnsiNative)
			b.ResetTimer()
			for b.Loop() {
				for _, l := range lines {
					c.Line(l)
				}
			}
		})
	}
}
