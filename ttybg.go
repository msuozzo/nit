package main

import (
	"os"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

var (
	oscRGB = regexp.MustCompile(`rgb:([0-9a-fA-F]+)/([0-9a-fA-F]+)/([0-9a-fA-F]+)`)
	da1    = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)
)

// terminalDark asks the terminal for its background color (OSC 11) and
// reports whether it is dark. ok is false when the terminal doesn't say.
// The trailing DA1 query makes every terminal answer something, so the
// read ends without waiting out the timeout.
func terminalDark() (dark, ok bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false, false
	}
	defer tty.Close()
	fd := int(tty.Fd())
	old, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return false, false
	}
	raw := *old
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN], raw.Cc[unix.VTIME] = 0, 2
	if unix.IoctlSetTermios(fd, ioctlSetTermios, &raw) != nil {
		return false, false
	}
	defer unix.IoctlSetTermios(fd, ioctlSetTermios, old)
	tty.WriteString("\x1b]11;?\x1b\\\x1b[c")
	var buf []byte
	tmp := make([]byte, 128)
	for deadline := time.Now().Add(500 * time.Millisecond); time.Now().Before(deadline) && !da1.Match(buf); {
		n, _ := tty.Read(tmp)
		if n == 0 {
			break
		}
		buf = append(buf, tmp[:n]...)
	}
	m := oscRGB.FindSubmatch(buf)
	if m == nil {
		return false, false
	}
	ch := func(b []byte) float64 {
		v, _ := strconv.ParseUint(string(b), 16, 32)
		return float64(v) / float64(uint64(1)<<(4*len(b))-1)
	}
	lum := 0.2126*ch(m[1]) + 0.7152*ch(m[2]) + 0.0722*ch(m[3])
	return lum < 0.5, true
}
