package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/msuozzo/nit/review"
)

// command is one request on the --listen socket, a line of JSON such as
// {"op": "open", "dir": "/src/ws", "from": "main"}. The reply is a line
// reading "ok" or the error.
type command struct {
	Op             string // open, refresh or quit
	Dir            string // open: where the diff runs
	review.Options        // open: the diff, without send, which nit keeps
}

// listen opens a unix socket at path, replacing one nobody listens on. It
// won't replace anything but a socket.
func listen(path string) (net.Listener, error) {
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		return nil, fmt.Errorf("another nit is listening on %s", path)
	}
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("%s exists and isn't a socket", path)
	}
	os.Remove(path)
	return net.Listen("unix", path)
}

// serve runs commands from l until it closes.
func (a *App) serve(l net.Listener, send string) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			reply := "ok"
			if err := a.command(c, send); err != nil {
				reply = err.Error()
			}
			fmt.Fprintln(c, reply)
		}()
	}
}

func (a *App) command(c net.Conn, send string) error {
	var cmd command
	if err := json.NewDecoder(c).Decode(&cmd); err != nil {
		return err
	}
	switch cmd.Op {
	case "open":
		// A peer can't choose what Send runs.
		cmd.Send = send
		r, err := review.Open(cmd.Dir, cmd.Options)
		if err != nil {
			return err
		}
		// Open doesn't check revisions, so load before replying.
		d, err := r.Load()
		if err != nil {
			return err
		}
		a.post(func() { a.openDiff(r, d) })
	case "refresh":
		a.post(a.refresh)
	case "quit":
		a.post(func() { a.quit = true })
	default:
		return fmt.Errorf("unknown op %q", cmd.Op)
	}
	return nil
}
