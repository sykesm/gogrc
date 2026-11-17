// SPDX-License-Identifier: GPL-2.0-or-later

package runner

import "golang.org/x/sys/unix"

// The ioctl requests for reading and setting terminal attributes on Linux.
const (
	ioctlGetTermios = unix.TCGETS
	ioctlSetTermios = unix.TCSETS
)
