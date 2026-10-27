package main

import "golang.org/x/sys/unix"

// Linux names the termios ioctls TCGETS and TCSETS.
const ioctlGetTermios, ioctlSetTermios = unix.TCGETS, unix.TCSETS
