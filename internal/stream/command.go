// SPDX-License-Identifier: GPL-2.0-or-later

package stream

import (
	"bufio"
	"io"
	"os"
	"os/exec"
)

// commandRunner returns a function that runs a color file command through
// the shell, like grcat's os.system. The command's output goes to dst after
// w is flushed, so it appears after the lines colored so far. Its stdin is
// empty, so it cannot consume the stream being colored.
func commandRunner(w *bufio.Writer, dst io.Writer) func(string) {
	return func(command string) {
		_ = w.Flush() // a write error is reported by Copy's next write
		cmd := exec.Command("/bin/sh", "-c", command)
		cmd.Stdout = dst
		cmd.Stderr = os.Stderr
		_ = cmd.Run() // like os.system, the command's status is ignored
	}
}
