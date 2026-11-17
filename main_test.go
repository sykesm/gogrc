// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
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
)

// The tests in this file run a built gogrc binary as the foreground job of a
// pseudo-terminal, to check what in-process tests cannot: the exec path,
// signals arriving from a terminal, and gogrc's own exit status.

// binary is the gogrc binary built for the tests.
var binary string

// coverDir, when set through GOGRC_COVERDIR, makes TestMain build binary
// with coverage instrumentation that writes its data there, so the
// subprocesses count toward coverage (see "make cover").
var coverDir = os.Getenv("GOGRC_COVERDIR")

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gogrc-test")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "gogrc")
	build := []string{"build", "-o", binary}
	if coverDir != "" {
		build = append(build, "-cover", "-coverpkg=./...")
	}
	if out, err := exec.Command("go", append(build, ".")...).CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// terminal runs gogrc with args as the foreground job of a fresh terminal
// and collects everything written to it.
type terminal struct {
	t      *testing.T
	cmd    *exec.Cmd
	master *os.File
	mu     sync.Mutex
	out    bytes.Buffer
	done   chan struct{}
}

func startTerminal(t *testing.T, args ...string) *terminal {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME=", "NO_COLOR=", "PAGER=less")
	if coverDir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverDir)
	}
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 132})
	if err != nil {
		t.Fatal(err)
	}
	term := &terminal{t: t, cmd: cmd, master: master, done: make(chan struct{})}
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
	return term
}

func (term *terminal) output() string {
	term.mu.Lock()
	defer term.mu.Unlock()
	return term.out.String()
}

// waitFor waits until the output contains s.
func (term *terminal) waitFor(s string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(term.output(), s) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// wait waits for gogrc to exit and returns its wait status.
func (term *terminal) wait() syscall.WaitStatus {
	term.t.Helper()
	err := term.cmd.Wait()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		term.t.Fatal(err)
	}
	select {
	case <-term.done:
	case <-time.After(time.Second):
	}
	term.master.Close()
	return term.cmd.ProcessState.Sys().(syscall.WaitStatus)
}

// testConf writes a color file that colors ERROR red.
func testConf(t *testing.T) string {
	t.Helper()
	conf := filepath.Join(t.TempDir(), "conf.t")
	if err := os.WriteFile(conf, []byte("regexp=ERROR\ncolours=red\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return conf
}

const probe = `[ -t 0 ] && i=tty || i=pipe; [ -t 1 ] && o=tty || o=pipe; [ -t 2 ] && e=tty || e=pipe; echo "stdin=$i stdout=$o stderr=$e cols=$(stty size <&2 | cut -d' ' -f2) pager=$PAGER"`

func TestCommandSeesTerminal(t *testing.T) {
	term := startTerminal(t, "-es", "-c", testConf(t), "sh", "-c", probe)
	term.wait()
	if out := term.output(); !strings.Contains(out, "stdin=tty stdout=tty stderr=tty cols=132 pager=cat") {
		t.Errorf("output = %q", out)
	}
}

func TestNoPTY(t *testing.T) {
	term := startTerminal(t, "--no-pty", "-c", testConf(t), "sh", "-c", probe)
	term.wait()
	if out := term.output(); !strings.Contains(out, "stdin=tty stdout=pipe stderr=tty") {
		t.Errorf("output = %q", out)
	}
}

func TestColorsAndExitCode(t *testing.T) {
	term := startTerminal(t, "-c", testConf(t), "sh", "-c", "echo an ERROR here; exit 7")
	ws := term.wait()
	if ws.ExitStatus() != 7 {
		t.Errorf("exit status = %d, want 7", ws.ExitStatus())
	}
	// The outer terminal adds \r; gogrc's own pty must not add another.
	want := "\033[0man \033[0m\033[31mERROR\033[0m here\033[0m\r\n"
	if out := term.output(); out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestPromptAppearsWithoutNewline(t *testing.T) {
	term := startTerminal(t, "-c", testConf(t), "sh", "-c", `printf "Continue? "; read answer; echo "got $answer"`)
	if !term.waitFor("Continue? ", 2*time.Second) {
		t.Fatalf("prompt not shown; output = %q", term.output())
	}
	io.WriteString(term.master, "yes\n")
	term.wait()
	if out := term.output(); !strings.Contains(out, "got yes") {
		t.Errorf("output = %q", out)
	}
}

func TestCtrlCReachesCommandOnce(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	// Count SIGINTs until the flag file appears, then die from SIGINT.
	script := `
import os, signal, sys, time
n = 0
def count(*_):
    global n
    n += 1
signal.signal(signal.SIGINT, count)
print("ready", flush=True)
while not os.path.exists(sys.argv[1]):
    time.sleep(0.02)
print("ints=%d" % n, flush=True)
signal.signal(signal.SIGINT, signal.SIG_DFL)
os.kill(os.getpid(), signal.SIGINT)
`
	flag := filepath.Join(t.TempDir(), "flag")
	term := startTerminal(t, "-c", testConf(t), python, "-c", script, flag)
	if !term.waitFor("ready", 5*time.Second) {
		t.Fatalf("output = %q", term.output())
	}
	io.WriteString(term.master, "\x03")
	time.Sleep(300 * time.Millisecond)
	os.WriteFile(flag, nil, 0o644)
	ws := term.wait()
	if out := term.output(); !strings.Contains(out, "ints=1") {
		t.Errorf("output = %q, want the command to see exactly one SIGINT", out)
	}
	if !ws.Signaled() || ws.Signal() != syscall.SIGINT {
		t.Errorf("gogrc status = %v, want killed by SIGINT like the command", ws)
	}
}

func TestPassthroughWithoutColorFile(t *testing.T) {
	term := startTerminal(t, "sh", "-c", probe)
	term.wait()
	// No grc.conf matches "sh", so gogrc execs the command directly.
	if out := term.output(); !strings.Contains(out, "stdin=tty stdout=tty stderr=tty") || !strings.Contains(out, "pager=less") {
		t.Errorf("output = %q", out)
	}
}
