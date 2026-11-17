// SPDX-License-Identifier: GPL-2.0-or-later

package runner

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/sykesm/gogrc/internal/colorfile"
	"github.com/sykesm/gogrc/internal/colorize"
)

const (
	def = colorfile.Default
	red = "\033[31m"
)

// colorizer returns a Colorizer that colors ERROR red.
func colorizer(t *testing.T) *colorize.Colorizer {
	t.Helper()
	f, err := colorfile.Parse(strings.NewReader("regexp=ERROR\ncolours=red\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	return colorize.New(f, colorfile.AnsiNative)
}

// tempFile returns an empty file that is removed after the test.
func tempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func contents(t *testing.T, f *os.File) string {
	t.Helper()
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRunColorsThroughPipes(t *testing.T) {
	stdout, stderr := tempFile(t), tempFile(t)
	st, err := Run(Config{
		Args:   []string{"sh", "-c", "echo an ERROR; echo ERROR on stderr >&2; exit 3"},
		Stdout: Stream{File: stdout, Colorizer: colorizer(t), PTY: true},
		Stderr: Stream{File: stderr},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st != (Status{Code: 3}) {
		t.Errorf("status = %+v, want code 3", st)
	}
	// stdout is a file, not a terminal, so a pipe is used despite PTY.
	if got, want := contents(t, stdout), def+"an "+def+red+"ERROR"+def+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	// stderr has no colorizer, so the command writes to it directly.
	if got := contents(t, stderr); got != "ERROR on stderr\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestRunColorsStderr(t *testing.T) {
	stdout, stderr := tempFile(t), tempFile(t)
	_, err := Run(Config{
		Args:   []string{"sh", "-c", "echo plain; echo ERROR >&2"},
		Stdout: Stream{File: stdout},
		Stderr: Stream{File: stderr, Colorizer: colorizer(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := contents(t, stdout); got != "plain\n" {
		t.Errorf("stdout = %q", got)
	}
	if got, want := contents(t, stderr), def+red+"ERROR"+def+"\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestRunReportsSignal(t *testing.T) {
	st, err := Run(Config{
		Args:   []string{"sh", "-c", "kill -TERM $$"},
		Stdout: Stream{File: tempFile(t), Colorizer: colorizer(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Status{Code: 128 + int(syscall.SIGTERM), Signal: syscall.SIGTERM}); st != want {
		t.Errorf("status = %+v, want %+v", st, want)
	}
}

func TestRunErrors(t *testing.T) {
	if _, err := Run(Config{}); err == nil {
		t.Error("Run with no command succeeded")
	}
	_, err := Run(Config{Args: []string{"gogrc-no-such-command"}})
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("err = %v, want exec.ErrNotFound", err)
	}
	dir := t.TempDir() // a directory found by path cannot be started
	if _, err := Run(Config{Args: []string{dir}, Stdout: Stream{File: tempFile(t)}}); err == nil {
		t.Error("running a directory succeeded")
	}
}

func TestRunReportsWriteErrors(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close() // writes to w now fail with EPIPE
	defer w.Close()
	var warnings bytes.Buffer
	st, err := Run(Config{
		Args:     []string{"sh", "-c", "echo ERROR"},
		Stdout:   Stream{File: w, Colorizer: colorizer(t)},
		Warnings: &warnings,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st // the command may succeed or die from SIGPIPE
	if !strings.Contains(warnings.String(), "broken pipe") {
		t.Errorf("warnings = %q, want a broken pipe error", warnings.String())
	}
}

// terminal is a pseudo-terminal standing in for the user's terminal.
type terminal struct {
	master, tty *os.File
	mu          sync.Mutex
	out         bytes.Buffer
	done        chan struct{}
}

func newTerminal(t *testing.T, rows, cols uint16) *terminal {
	t.Helper()
	master, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	if err := pty.Setsize(master, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		t.Fatal(err)
	}
	if err := disableOutputProcessing(tty); err != nil {
		t.Fatal(err)
	}
	term := &terminal{master: master, tty: tty, done: make(chan struct{})}
	go func() {
		defer close(term.done)
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			term.mu.Lock()
			term.out.Write(buf[:n])
			term.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		tty.Close()
		master.Close()
		<-term.done
	})
	return term
}

func (term *terminal) output() string {
	term.mu.Lock()
	defer term.mu.Unlock()
	return term.out.String()
}

func (term *terminal) waitFor(t *testing.T, s string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if strings.Contains(term.output(), s) {
			return
		}
	}
	t.Fatalf("timed out waiting for %q; output = %q", s, term.output())
}

func TestRunGivesCommandAPTY(t *testing.T) {
	term := newTerminal(t, 30, 100)
	flag := filepath.Join(t.TempDir(), "flag")
	script := `[ -t 1 ] && echo "stdout=tty pager=$PAGER"; echo "size=$(stty size <&2)"; echo ready
while [ ! -f "$0" ]; do sleep 0.02; done
echo "resized=$(stty size <&2)"`

	var st Status
	var runErr error
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		st, runErr = Run(Config{
			Args:   []string{"sh", "-c", script, flag},
			Stdout: Stream{File: term.tty, Colorizer: colorizer(t), PTY: true},
			Stderr: Stream{File: term.tty, Colorizer: colorizer(t), PTY: true},
		})
	}()

	term.waitFor(t, "ready")
	// Resize the "real" terminal and tell gogrc, as the terminal would.
	if err := pty.Setsize(term.master, &pty.Winsize{Rows: 50, Cols: 160}); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	<-finished

	if runErr != nil || st.Code != 0 {
		t.Fatalf("Run = %+v, %v", st, runErr)
	}
	// Run has written everything to the terminal, but the terminal's reader
	// may not have read it yet.
	term.waitFor(t, "resized=")
	out := term.output()
	for _, want := range []string{"stdout=tty pager=cat", "size=30 100", "resized=50 160"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
	if strings.Contains(out, "\r\n") {
		t.Errorf("output %q contains \\r\\n; the command's pty must not translate newlines", out)
	}
}

func TestRunForwardsSignals(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGUSR1, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			// SIGINT, SIGQUIT and SIGHUP reach the command directly from a
			// terminal it shares with gogrc, so they are only forwarded when
			// gogrc is not in the terminal's foreground group.
			if sig == syscall.SIGHUP && inForeground() {
				t.Skip("running in the terminal's foreground process group")
			}
			testForwardsSignal(t, sig)
		})
	}
}

func testForwardsSignal(t *testing.T, sig syscall.Signal) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ready := make(chan struct{})
	go func() {
		// Read until "ready", then drain so the command never blocks.
		br := bufio.NewReader(r)
		for {
			line, err := br.ReadString('\n')
			if strings.Contains(line, "ready") {
				close(ready)
				break
			}
			if err != nil {
				return
			}
		}
		io.Copy(io.Discard, br)
	}()

	var st Status
	var runErr error
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer w.Close()
		st, runErr = Run(Config{
			Args:   []string{"sh", "-c", "echo ready; exec sleep 10"},
			Stdout: Stream{File: w, Colorizer: colorizer(t)},
		})
	}()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("command did not start")
	}
	// Run has registered for the signal, so this reaches gogrc's handler,
	// not the default action, and is forwarded to the command.
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("signal was not forwarded")
	}
	if runErr != nil || st.Signal != sig {
		t.Errorf("Run = %+v, %v; want killed by %v", st, runErr, sig)
	}
}

func TestInForeground(t *testing.T) {
	// The test may or may not have a controlling terminal; either way the
	// answer must match the process group of /dev/tty.
	got := inForeground()
	tty, err := os.Open("/dev/tty")
	if err != nil {
		if got {
			t.Error("inForeground() = true without a terminal")
		}
		return
	}
	tty.Close()
	t.Logf("inForeground() = %v", got)
}

func TestStatusOf(t *testing.T) {
	tests := []struct {
		ws   syscall.WaitStatus
		want Status
	}{
		{0, Status{}},
		{3 << 8, Status{Code: 3}},
		{syscall.WaitStatus(syscall.SIGKILL), Status{Code: 137, Signal: syscall.SIGKILL}},
	}
	for _, tt := range tests {
		if got := statusOf(tt.ws); got != tt.want {
			t.Errorf("statusOf(%#x) = %+v, want %+v", uint32(tt.ws), got, tt.want)
		}
	}
}
