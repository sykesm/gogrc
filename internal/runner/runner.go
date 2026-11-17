// SPDX-License-Identifier: GPL-2.0-or-later

// Package runner starts the wrapped command with its output routed through
// colorizers.
//
// Each colored stream reaches gogrc through a pipe or, when the stream it
// replaces is a terminal, through a pseudo-terminal, so the command keeps its
// terminal behavior. The command's stdin is gogrc's own, and the command
// stays in gogrc's process group, so typing, job control and Ctrl-C work
// through the real terminal.
package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/sykesm/gogrc/internal/colorize"
	"github.com/sykesm/gogrc/internal/stream"
)

// pagerEnv stops commands that now see a terminal from starting a pager,
// whose screen output cannot be colored line by line.
var pagerEnv = []string{
	"PAGER=cat", "GIT_PAGER=cat", "MANPAGER=cat", "SYSTEMD_PAGER=cat", "GH_PAGER=cat", "AWS_PAGER=",
}

// forwarded lists the signals Run handles while the command runs.
var forwarded = []os.Signal{
	unix.SIGINT, unix.SIGQUIT, unix.SIGHUP, unix.SIGTERM, unix.SIGUSR1, unix.SIGUSR2, unix.SIGWINCH,
}

// Stream describes how one of the command's output streams is handled.
type Stream struct {
	// File is where the stream's output goes: gogrc's own stdout or stderr.
	// When nil, os.Stdout or os.Stderr is used.
	File *os.File
	// Colorizer colors the stream. When nil, the command writes to File
	// directly.
	Colorizer *colorize.Colorizer
	// PTY gives the command a pseudo-terminal for this stream when File is a
	// terminal. Otherwise the command writes to a pipe.
	PTY bool
}

// Config describes the command to run.
type Config struct {
	// Args is the command and its arguments. Args[0] is looked up in PATH.
	Args []string
	// Stdin is the command's standard input. When nil, os.Stdin is used.
	Stdin *os.File
	// Stdout and Stderr describe the command's output streams.
	Stdout, Stderr Stream
	// Warnings receives problems reported while copying output. When nil,
	// os.Stderr is used.
	Warnings io.Writer
}

// Status is how the command finished.
type Status struct {
	// Code is the exit status, or 128 plus the signal number when the
	// command was killed by a signal, as shells report it.
	Code int
	// Signal is the signal that killed the command, or 0 if it exited.
	Signal syscall.Signal
}

// Exec replaces the current process with the command. It returns only on
// failure.
func Exec(args []string) error {
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, os.Environ())
}

// output is one redirected stream: the end the command writes to, and the
// end gogrc reads from.
type output struct {
	dst    *os.File // gogrc's own stdout or stderr
	reader *os.File // pipe read end or pty master
	writer *os.File // pipe write end or pty slave
	isPTY  bool
	c      *colorize.Colorizer
}

// Run starts the command and copies its colored output until it exits and
// all of its output has been written.
func Run(cfg Config) (Status, error) {
	if len(cfg.Args) == 0 {
		return Status{}, errors.New("no command")
	}
	cmd := exec.Command(cfg.Args[0], cfg.Args[1:]...)
	if cmd.Err != nil {
		return Status{}, cmd.Err
	}
	cmd.Stdin = orDefault(cfg.Stdin, os.Stdin)
	stdout := orDefault(cfg.Stdout.File, os.Stdout)
	stderr := orDefault(cfg.Stderr.File, os.Stderr)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	warnings := cfg.Warnings
	if warnings == nil {
		warnings = os.Stderr
	}

	var outs []*output
	defer func() {
		for _, o := range outs {
			o.reader.Close()
			o.writer.Close()
		}
	}()
	usedPTY := false
	for _, s := range []struct {
		Stream
		dst    *os.File
		target *io.Writer
	}{
		{cfg.Stdout, stdout, &cmd.Stdout},
		{cfg.Stderr, stderr, &cmd.Stderr},
	} {
		if s.Colorizer == nil {
			continue
		}
		o := &output{dst: s.dst, c: s.Colorizer}
		var err error
		if s.PTY && term.IsTerminal(int(o.dst.Fd())) {
			o.reader, o.writer, err = openPTY(o.dst)
			o.isPTY = true
			usedPTY = true
		} else {
			o.reader, o.writer, err = os.Pipe()
		}
		if err != nil {
			return Status{}, err
		}
		outs = append(outs, o)
		*s.target = o.writer
	}
	if usedPTY {
		cmd.Env = append(os.Environ(), pagerEnv...)
	}

	// Handle signals before starting the command so none are missed.
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, forwarded...)
	defer signal.Stop(sigs)

	if err := cmd.Start(); err != nil {
		return Status{}, err
	}
	// Only the command may hold the write ends, so readers see end of file
	// when it exits.
	for _, o := range outs {
		o.writer.Close()
	}

	done := make(chan struct{})
	forwarding := make(chan struct{})
	go func() {
		defer close(forwarding)
		forwardSignals(sigs, done, cmd.Process, outs)
	}()

	var wg sync.WaitGroup
	for _, o := range outs {
		opts := stream.Options{}
		if term.IsTerminal(int(o.dst.Fd())) {
			opts.PartialTimeout = stream.DefaultPartialTimeout
		}
		wg.Go(func() {
			if err := stream.Copy(o.dst, o.reader, o.c, opts); err != nil {
				fmt.Fprintf(warnings, "gogrc: %v\n", err)
				// Stop reading so the command is not blocked writing to us.
				o.reader.Close()
			}
		})
	}

	waitErr := cmd.Wait()
	wg.Wait()
	close(done)
	<-forwarding

	if exitErr := (*exec.ExitError)(nil); waitErr != nil && !errors.As(waitErr, &exitErr) {
		return Status{}, waitErr
	}
	return statusOf(cmd.ProcessState.Sys().(syscall.WaitStatus)), nil
}

// statusOf converts a wait status to a Status.
func statusOf(ws syscall.WaitStatus) Status {
	if ws.Signaled() {
		return Status{Code: 128 + int(ws.Signal()), Signal: ws.Signal()}
	}
	return Status{Code: ws.ExitStatus()}
}

func orDefault(f, def *os.File) *os.File {
	if f == nil {
		return def
	}
	return f
}

// forwardSignals relays signals to the command until done is closed.
// Signals from the terminal (Ctrl-C, Ctrl-\, hangup) already reach the
// command because it shares gogrc's process group, so they are only forwarded
// when gogrc is not in the terminal's foreground group, for example when sent
// with kill. Window size changes are copied to the command's pseudo-terminals.
func forwardSignals(sigs <-chan os.Signal, done <-chan struct{}, p *os.Process, outs []*output) {
	for {
		select {
		case <-done:
			return
		case sig := <-sigs:
			switch sig {
			case unix.SIGWINCH:
				for _, o := range outs {
					if o.isPTY {
						resizePTY(o.reader, o.dst)
					}
				}
			case unix.SIGINT, unix.SIGQUIT, unix.SIGHUP:
				if !inForeground() {
					p.Signal(sig)
				}
			default:
				p.Signal(sig)
			}
		}
	}
}

// inForeground reports whether gogrc belongs to the foreground process
// group of its controlling terminal.
func inForeground() bool {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	defer tty.Close()
	pgrp, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP)
	return err == nil && pgrp == unix.Getpgrp()
}

// Exit ends gogrc the way the command ended. A command killed by SIGINT,
// SIGTERM, SIGHUP or SIGPIPE is mirrored by gogrc killing itself with the
// same signal, so shells notice the interruption (and stop loops). Other
// statuses become gogrc's exit code.
func Exit(st Status) {
	switch st.Signal {
	case unix.SIGINT, unix.SIGTERM, unix.SIGHUP, unix.SIGPIPE:
		signal.Reset(st.Signal)
		// The signal is delivered asynchronously; give it time to arrive. If
		// it cannot be sent, the exit code below still reports it.
		if unix.Kill(os.Getpid(), st.Signal) == nil {
			time.Sleep(100 * time.Millisecond)
		}
	}
	os.Exit(st.Code)
}
