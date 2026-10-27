// nit reviews a jj or git diff in the terminal. It follows skepsis
// (github.com/oxidecomputer/skepsis) as an independent implementation: it
// takes skepsis's flags, keys, and comment model, and keeps its on-disk
// state, so the two share viewed files and <review> comments:
//
//	nit                  # jj: fork_point(trunk() | @) to @, git: merge base to the working tree
//	nit -r @             # one revision
//	nit -f main -t @     # between two revisions
//	nit --send CMD       # pipe comments to CMD (default $SKEPSIS_SEND)
//	nit --listen SOCK    # take open, refresh and quit commands on SOCK
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/msuozzo/nit/review"
)

func main() {
	var o review.Options
	for _, name := range []string{"r", "revisions"} {
		flag.StringVar(&o.Revisions, name, "", "show changes in these revisions")
	}
	for _, name := range []string{"f", "from"} {
		flag.StringVar(&o.From, name, "", "show changes from this revision")
	}
	for _, name := range []string{"t", "to"} {
		flag.StringVar(&o.To, name, "", "show changes to this revision")
	}
	flag.BoolVar(&o.Git, "git", false, "use git even in a jj repo")
	flag.StringVar(&o.Send, "send", os.Getenv("SKEPSIS_SEND"), "shell command comments are sent to, as JSON on stdin")
	sock := flag.String("listen", "", "unix socket to take open, refresh and quit commands on")
	shot := flag.String("shot", "", "render to this PNG instead of the terminal (for development)")
	size := flag.String("size", "200x56", "screen size for -shot")
	script := flag.String("script", "", "keys and mouse events to replay for -shot")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: nit [-r REVSETS | -f FROM -t TO] [--git] [--send CMD] [--listen SOCK] [paths...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	o.Paths = flag.Args()
	dir, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	repo, err := review.Open(dir, o)
	if err != nil {
		fatal(err)
	}
	theme := review.Theme()
	dark := theme != "light"
	if theme == "system" && *shot == "" {
		if d, ok := terminalDark(); ok {
			dark = d
		}
	}
	th := lightTheme()
	if dark {
		th = darkTheme()
	}
	if *shot != "" {
		if err := runShot(repo, th, *shot, *size, *script); err != nil {
			fatal(err)
		}
		return
	}
	// Listen before the screen starts, so an error shows in the terminal.
	var l net.Listener
	if *sock != "" {
		if l, err = listen(*sock); err != nil {
			fatal(err)
		}
		defer l.Close()
	}
	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fatal(err)
	}
	defer s.Fini()
	a := newApp(s, repo, th)
	if l != nil {
		go a.serve(l, o.Send)
	}
	a.run()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "nit:", strings.TrimSpace(err.Error()))
	os.Exit(1)
}
