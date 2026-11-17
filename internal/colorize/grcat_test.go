// SPDX-License-Identifier: GPL-2.0-or-later

package colorize

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/defaults"
)

// TestMatchesPythonGrcat runs Python grcat and gogrc over the test corpus with
// every stock color file and requires byte-identical output. Set
// GOGRC_DIFF_CONFS to a directory to also check the conf.* files in it.
func TestMatchesPythonGrcat(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	grcat, err := filepath.Abs("../../testdata/grcat.py")
	if err != nil {
		t.Fatal(err)
	}
	corpus := readCorpus(t, "../../testdata/corpus")

	dir := t.TempDir()
	confs := map[string]string{} // test name -> path
	for _, name := range defaults.Names() {
		if !strings.HasPrefix(name, "conf.") {
			continue
		}
		data, err := fs.ReadFile(defaults.FS(), name)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		confs[name] = path
	}
	if extra := os.Getenv("GOGRC_DIFF_CONFS"); extra != "" {
		matches, _ := filepath.Glob(filepath.Join(extra, "conf.*"))
		for _, path := range matches {
			confs["extra/"+filepath.Base(path)] = path
		}
	}

	names := make([]string, 0, len(confs))
	for name := range confs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := confs[name]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			cmd := exec.Command(python, "-I", grcat, path)
			cmd.Env = append(os.Environ(), "PYTHONUTF8=1")
			cmd.Stdin = bytes.NewReader(corpus)
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				msg := strings.TrimSpace(stderr.String())
				if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
					msg = msg[i+1:]
				}
				t.Skipf("python grcat fails on this file: %s", msg)
			}

			f, err := colorfile.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			c := New(f, colorfile.AnsiNative)
			var got bytes.Buffer
			for _, line := range strings.SplitAfter(string(corpus), "\n") {
				if line == "" {
					continue
				}
				if out, ok := c.Line(strings.TrimSuffix(line, "\n")); ok {
					got.WriteString(out)
					got.WriteByte('\n')
				}
			}

			want := stdout.Bytes()
			if bytes.Equal(got.Bytes(), want) {
				return
			}
			gotLines := strings.Split(got.String(), "\n")
			wantLines := strings.Split(string(want), "\n")
			for i := 0; i < max(len(gotLines), len(wantLines)); i++ {
				var g, w string
				if i < len(gotLines) {
					g = gotLines[i]
				}
				if i < len(wantLines) {
					w = wantLines[i]
				}
				if g != w {
					t.Fatalf("output line %d differs\n got %q\nwant %q", i+1, g, w)
				}
			}
		})
	}
}

func readCorpus(t testing.TB, dir string) []byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no corpus in %s: %v", dir, err)
	}
	sort.Strings(paths)
	var corpus []byte
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		corpus = append(corpus, data...)
	}
	return corpus
}
