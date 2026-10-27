package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// CodeRun is a run of consecutive lines quoted with a comment.
type CodeRun struct {
	Lines [2]int `json:"lines"`
	Text  string `json:"text"`
}

// Comment is a review comment for the --send command.
type Comment struct {
	File  string
	Side  string    // additions or deletions
	Lines [2]int    // first and last line commented on, in Side's numbering
	Code  []CodeRun // excerpt around Lines
	Blob  string    // id of Side's version of File, "" if none
	Text  string
}

// payload is skepsis's --send JSON, version 1.
type payload struct {
	Version int    `json:"version"`
	Root    string `json:"root"`
	Diff    struct {
		Command       string `json:"command"`
		ToWorkingCopy bool   `json:"toWorkingCopy"`
	} `json:"diff"`
	File   string    `json:"file"`
	Side   string    `json:"side"`
	Lines  [2]int    `json:"lines"`
	Code   []CodeRun `json:"code"`
	Blob   *string   `json:"blob"`
	Text   string    `json:"text"`
	Prompt string    `json:"prompt"`
}

const sendTimeout = 10 * time.Second

// Send pipes c to the --send command as one line of JSON. Sends run one at
// a time. Exit 0 means delivered, anything else is an error carrying stderr.
func (r *Repo) Send(c Comment) error {
	if r.send == "" {
		return errors.New("no --send command")
	}
	p := payload{Version: 1, Root: r.Root, File: c.File, Side: c.Side, Lines: c.Lines, Code: c.Code, Text: c.Text}
	p.Diff.Command, p.Diff.ToWorkingCopy = r.Command(), r.inline
	if c.Blob != "" {
		p.Blob = &c.Blob
	}
	p.Prompt = prompt(p)
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	r.sendMu.Lock()
	defer r.sendMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", r.send)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "SKEPSIS_ROOT="+r.Root)
	cmd.Stdin = bytes.NewReader(append(data, '\n'))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	switch err := cmd.Run(); {
	case ctx.Err() != nil:
		return fmt.Errorf("--send command timed out after %v", sendTimeout)
	case err == nil:
		return nil
	case strings.TrimSpace(stderr.String()) != "":
		return errors.New(strings.TrimSpace(stderr.String()))
	default:
		return fmt.Errorf("--send command: %w", err)
	}
}

// prompt is the comment as one plain-text message:
//
//	Review comment on a.go, lines 42–45 (new side of `jj diff -r @-`), in lines 40–47:
//
//	```
//	…
//	```
//
//	why two?
//
// A file comment has lines 0–0 and reads "Review comment on a.go, whole
// file (`jj diff -r @-`):" with no excerpt.
func prompt(p payload) string {
	span := func(l [2]int) string {
		if l[0] == l[1] {
			return fmt.Sprintf("line %d", l[0])
		}
		return fmt.Sprintf("lines %d–%d", l[0], l[1])
	}
	side := "new side"
	if p.Side == "deletions" {
		side = "old side"
	}
	head := fmt.Sprintf("Review comment on %s, %s (%s of `%s`)", p.File, span(p.Lines), side, p.Diff.Command)
	if p.Lines[1] == 0 {
		head = fmt.Sprintf("Review comment on %s, whole file (`%s`)", p.File, p.Diff.Command)
	}
	if len(p.Code) == 0 {
		return head + ":\n\n" + p.Text
	}
	var spans, excerpts []string
	for _, c := range p.Code {
		spans = append(spans, span(c.Lines))
		fence := strings.Repeat("`", max(3, longestRun(c.Text, '`')+1))
		excerpts = append(excerpts, fence+"\n"+c.Text+"\n"+fence)
	}
	return head + ", in " + strings.Join(spans, " and ") + ":\n\n" + strings.Join(excerpts, "\n\n") + "\n\n" + p.Text
}

func longestRun(s string, c byte) int {
	best, n := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			n++
			best = max(best, n)
		} else {
			n = 0
		}
	}
	return best
}
