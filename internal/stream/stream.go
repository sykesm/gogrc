// SPDX-License-Identifier: GPL-2.0-or-later

// Package stream copies a command's output through a Colorizer, line by
// line, flushing promptly so interactive output appears as it is produced.
package stream

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/sykesm/gogrc/internal/colorize"
)

// DefaultPartialTimeout is the PartialTimeout gogrc uses when writing to a
// terminal.
const DefaultPartialTimeout = 50 * time.Millisecond

// Options configures Copy.
type Options struct {
	// PartialTimeout, when positive, is how long an incomplete line may wait
	// for its newline before it is colored and written anyway, so prompts
	// and progress output appear. Use it only for interactive output: it can
	// split a line that arrives slowly.
	PartialTimeout time.Duration
}

// Copy reads lines from src, colors them with c and writes them to dst until
// src reaches end of file. It returns the first read or write error. EIO,
// which a pty master returns once the command exits, counts as end of file.
//
// Lines end at "\n". A "\r" before the newline is kept out of the text being
// colored and written back after it. If c.RunCommand is nil, Copy sets it to
// run rule commands through /bin/sh with their output going to dst, after
// any colored output already pending.
func Copy(dst io.Writer, src io.Reader, c *colorize.Colorizer, opts Options) error {
	w := bufio.NewWriterSize(dst, 64*1024)
	if c.RunCommand == nil {
		c.RunCommand = commandRunner(w, dst)
	}

	chunks, readErr, stop := readChunks(src)
	defer stop()

	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	var pending []byte
	var timeout <-chan time.Time
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				if len(pending) > 0 {
					if err := emit(w, c, pending, "\n"); err != nil {
						return err
					}
				}
				if err := w.Flush(); err != nil {
					return err
				}
				return <-readErr
			}
			pending = append(pending, chunk...)
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				if err := emit(w, c, pending[:i], "\n"); err != nil {
					return err
				}
				pending = pending[i+1:]
			}
			if len(pending) == 0 {
				pending = nil // release the buffer
			}
			// Flush once the reader has nothing more queued, so bulk output
			// is written in large pieces and interactive output immediately.
			if len(chunks) == 0 {
				if err := w.Flush(); err != nil {
					return err
				}
			}
			timeout = nil
			if opts.PartialTimeout > 0 && len(pending) > 0 {
				if timer == nil {
					timer = time.NewTimer(opts.PartialTimeout)
				} else {
					timer.Reset(opts.PartialTimeout)
				}
				timeout = timer.C
			}
		case <-timeout:
			timeout = nil
			if err := emit(w, c, pending, ""); err != nil {
				return err
			}
			pending = nil
			if err := w.Flush(); err != nil {
				return err
			}
		}
	}
}

// readChunks reads src in a goroutine. The chunks channel is closed at end
// of file or on error, after which readErr yields the error (nil at end of
// file). Calling stop makes the goroutine exit at its next chunk.
func readChunks(src io.Reader) (chunks <-chan []byte, readErr <-chan error, stop func()) {
	ch := make(chan []byte, 16)
	errc := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(ch)
		for {
			buf := make([]byte, 32*1024)
			n, err := src.Read(buf)
			if n > 0 {
				select {
				case ch <- buf[:n]:
				case <-done:
					return
				}
			}
			if err != nil {
				if isEOF(err) {
					err = nil
				}
				errc <- err
				return
			}
		}
	}()
	return ch, errc, func() { close(done) }
}

// emit colors one line and writes it with its terminator.
func emit(w *bufio.Writer, c *colorize.Colorizer, line []byte, term string) error {
	s := string(line)
	if term == "\n" && len(s) > 0 && s[len(s)-1] == '\r' {
		// Keep CRLF lines matching patterns anchored with $.
		s, term = s[:len(s)-1], "\r\n"
	}
	out, ok := c.Line(s)
	if !ok {
		return nil
	}
	if _, err := w.WriteString(out); err != nil {
		return err
	}
	_, err := w.WriteString(term)
	return err
}

// isEOF reports whether err means the source has no more data.
func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed)
}
