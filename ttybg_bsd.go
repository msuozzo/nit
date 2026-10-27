//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package main

import "golang.org/x/sys/unix"

// macOS and the BSDs name the termios ioctls TIOCGETA and TIOCSETA.
const ioctlGetTermios, ioctlSetTermios = unix.TIOCGETA, unix.TIOCSETA
