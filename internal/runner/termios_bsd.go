// SPDX-License-Identifier: GPL-2.0-or-later

//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package runner

import "golang.org/x/sys/unix"

// The ioctl requests for reading and setting terminal attributes on BSD
// systems, including macOS.
const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)
