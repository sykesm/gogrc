// SPDX-License-Identifier: GPL-2.0-or-later

package stream

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/colorize"
)

func TestCommandOutputIsOrdered(t *testing.T) {
	f, err := colorfile.Parse(strings.NewReader("regexp=^go$\ncommand=echo ran\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Copy(&out, strings.NewReader("before\ngo\nafter\n"), colorize.New(f, colorfile.AnsiNative), Options{}); err != nil {
		t.Fatal(err)
	}
	want := def + "before" + def + "\nran\n" + def + "go" + def + "\n" + def + "after" + def + "\n"
	if out.String() != want {
		t.Errorf("got  %q\nwant %q", out.String(), want)
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestCopyErrors(t *testing.T) {
	boom := errors.New("boom")
	if err := Copy(io.Discard, errReader{boom}, colorizer(t), Options{}); !errors.Is(err, boom) {
		t.Errorf("read error = %v, want %v", err, boom)
	}
	for _, eof := range []error{io.EOF, syscall.EIO, os.ErrClosed} {
		if err := Copy(io.Discard, errReader{eof}, colorizer(t), Options{}); err != nil {
			t.Errorf("Copy with %v = %v, want nil", eof, err)
		}
	}
	// Write errors surface from a complete line, the final partial line and
	// the final flush.
	for _, in := range []string{strings.Repeat("ab\n", 30000), "partial", ""} {
		err := Copy(errWriter{}, strings.NewReader(in), colorizer(t), Options{})
		if in == "" {
			if err != nil {
				t.Errorf("empty input: %v", err)
			}
			continue
		}
		if err == nil {
			t.Errorf("Copy(%.10q...) to a failing writer succeeded", in)
		}
	}
}

func TestPartialFlushWriteError(t *testing.T) {
	r := &slowReader{chunks: []string{"prompt: ", "rest\n"}, delay: 100 * time.Millisecond}
	if err := Copy(errWriter{}, r, colorizer(t), Options{PartialTimeout: 10 * time.Millisecond}); err == nil {
		t.Error("write error during a partial flush was not returned")
	}
}

func TestPartialTimerResets(t *testing.T) {
	// Each chunk without a newline restarts the timer; a newline cancels it.
	r := &slowReader{chunks: []string{"a", "b", "\n"}, delay: 5 * time.Millisecond}
	var out bytes.Buffer
	if err := Copy(&out, r, colorizer(t), Options{PartialTimeout: 200 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if out.String() != def+"a"+def+red+"b"+def+"\n" {
		t.Errorf("got %q", out.String())
	}
}
