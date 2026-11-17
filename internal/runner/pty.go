// SPDX-License-Identifier: GPL-2.0-or-later

package runner

import (
	"os"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// openPTY opens a pseudo-terminal for one of the command's output streams,
// sized like the terminal sizeFrom. The command only writes to it: the pty
// does not become the command's controlling terminal, so job control and
// Ctrl-C keep working through the real one.
func openPTY(sizeFrom *os.File) (master, slave *os.File, err error) {
	master, slave, err = pty.Open()
	if err != nil {
		return nil, nil, err
	}
	if err := disableOutputProcessing(slave); err != nil {
		master.Close()
		slave.Close()
		return nil, nil, err
	}
	resizePTY(master, sizeFrom)
	return master, slave, nil
}

// disableOutputProcessing turns off the terminal's output processing, so
// "\n" is not turned into "\r\n".
func disableOutputProcessing(tty *os.File) error {
	t, err := unix.IoctlGetTermios(int(tty.Fd()), ioctlGetTermios)
	if err != nil {
		return err
	}
	t.Oflag &^= unix.OPOST
	return unix.IoctlSetTermios(int(tty.Fd()), ioctlSetTermios, t)
}

// resizePTY copies the window size of the real terminal to the pty. Errors
// are ignored: a pty with the wrong size still works.
func resizePTY(master, term *os.File) {
	if ws, err := pty.GetsizeFull(term); err == nil {
		_ = pty.Setsize(master, ws)
	}
}
