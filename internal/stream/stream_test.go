// SPDX-License-Identifier: GPL-2.0-or-later

package stream

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/colorize"
)

const (
	def = colorfile.Default
	red = "\033[31m"
)

func colorizer(t *testing.T) *colorize.Colorizer {
	t.Helper()
	f, err := colorfile.Parse(strings.NewReader("regexp=b$\ncolours=red\n-\nregexp=^skip\nskip=yes\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	return colorize.New(f, colorfile.AnsiNative)
}

func TestCopy(t *testing.T) {
	var out bytes.Buffer
	in := "ab\nskip me\nab\r\nlast b"
	if err := Copy(&out, strings.NewReader(in), colorizer(t), Options{}); err != nil {
		t.Fatal(err)
	}
	want := def + "a" + def + red + "b" + def + "\n" +
		def + "a" + def + red + "b" + def + "\r\n" +
		def + "last " + def + red + "b" + def + "\n"
	if out.String() != want {
		t.Errorf("got  %q\nwant %q", out.String(), want)
	}
}

// slowReader returns its chunks one at a time, pausing before each.
type slowReader struct {
	chunks []string
	delay  time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

type timedWriter struct {
	start  time.Time
	writes []string
	at     []time.Duration
}

func (w *timedWriter) Write(p []byte) (int, error) {
	w.writes = append(w.writes, string(p))
	w.at = append(w.at, time.Since(w.start))
	return len(p), nil
}

func TestPartialLineIsFlushed(t *testing.T) {
	r := &slowReader{chunks: []string{"Password: ", "secret\n"}, delay: 300 * time.Millisecond}
	w := &timedWriter{start: time.Now()}
	if err := Copy(w, r, colorizer(t), Options{PartialTimeout: 20 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if len(w.writes) != 2 || w.writes[0] != def+"Password: "+def {
		t.Fatalf("writes = %q", w.writes)
	}
	if w.at[0] > 450*time.Millisecond {
		t.Errorf("prompt written after %v, want before the rest of the line arrives", w.at[0])
	}
}
