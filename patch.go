package main

import (
	"regexp"
	"strconv"
	"strings"
)

type Kind uint8

const (
	Ctx Kind = iota
	Add
	Del
)

// Line is one line of a diff side. Old and New are 1-based line numbers, 0
// where the line has none on that side.
type Line struct {
	Kind     Kind
	Old, New int
	Text     string
	Words    []Span // byte ranges that differ from the paired line
}

// Span is the byte range [A, B) of a string.
type Span struct{ A, B int }

// num returns the line's number on a side, 0 where it has none.
func (l *Line) num(side int) int {
	if side == 0 {
		return l.Old
	}
	return l.New
}

type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Heading            string
	Lines              []*Line
}

// firstOld is the first old line the hunk covers. A side with no lines
// reports the line before the insertion point.
func (h *Hunk) firstOld() int { return h.OldStart + b2i(h.OldCount == 0) }
func (h *Hunk) firstNew() int { return h.NewStart + b2i(h.NewCount == 0) }
func (h *Hunk) lastOld() int  { return h.firstOld() + h.OldCount - 1 }
func (h *Hunk) lastNew() int  { return h.firstNew() + h.NewCount - 1 }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

var (
	diffGitRe = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)
	hunkRe    = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)
)

// parsePatch reads a git-format patch. Files are keyed the way skepsis keys
// fileHashes and viewed state: by the b/ path of the diff --git line.
func parsePatch(patch string) []*File {
	var files []*File
	var f *File
	var h *Hunk
	var oldN, newN, remOld, remNew int
	var oldMode string
	for _, ln := range strings.Split(patch, "\n") {
		if h != nil && remOld+remNew > 0 {
			c := byte(' ')
			if ln != "" {
				c = ln[0]
			}
			if c == '\\' {
				continue
			}
			if c == ' ' || c == '-' || c == '+' {
				l := &Line{Text: strings.TrimSuffix(ln[min(1, len(ln)):], "\r")}
				if c != '+' {
					l.Old, oldN, remOld = oldN, oldN+1, remOld-1
				}
				if c != '-' {
					l.New, newN, remNew = newN, newN+1, remNew-1
				}
				switch c {
				case '-':
					l.Kind, f.Dels = Del, f.Dels+1
				case '+':
					l.Kind, f.Adds = Add, f.Adds+1
				}
				h.Lines = append(h.Lines, l)
				continue
			}
			h = nil
		}
		if m := diffGitRe.FindStringSubmatch(ln); m != nil {
			f = &File{Path: m[2], OldPath: m[1], Status: 'M'}
			files = append(files, f)
			h, oldMode = nil, ""
			continue
		}
		if f == nil {
			continue
		}
		switch {
		case strings.HasPrefix(ln, "new file mode"):
			f.Status = 'A'
		case strings.HasPrefix(ln, "deleted file mode"):
			f.Status = 'D'
		case strings.HasPrefix(ln, "rename from "):
			f.Status, f.OldPath = 'R', unquote(ln[len("rename from "):])
		case strings.HasPrefix(ln, "copy from "):
			f.Status, f.OldPath = 'C', unquote(ln[len("copy from "):])
		case strings.HasPrefix(ln, "old mode "):
			oldMode = ln[len("old mode "):]
		case strings.HasPrefix(ln, "new mode "):
			f.Note = "mode " + oldMode + " → " + ln[len("new mode "):]
		case strings.HasPrefix(ln, "index "):
			ids, _, _ := strings.Cut(ln[len("index "):], " ")
			if a, b, ok := strings.Cut(ids, ".."); ok {
				f.OldBlob, f.NewBlob = a, b
			}
		case strings.HasPrefix(ln, "Binary files"), strings.HasPrefix(ln, "GIT binary patch"):
			f.Binary = true
		default:
			if m := hunkRe.FindStringSubmatch(ln); m != nil {
				h = &Hunk{OldStart: atoi(m[1]), OldCount: atoiOr(m[2], 1), NewStart: atoi(m[3]), NewCount: atoiOr(m[4], 1), Heading: m[5]}
				f.Hunks = append(f.Hunks, h)
				oldN, newN, remOld, remNew = h.OldStart, h.NewStart, h.OldCount, h.NewCount
			}
		}
	}
	return files
}

func unquote(s string) string {
	if u, err := strconv.Unquote(s); err == nil {
		return u
	}
	return s
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	return atoi(s)
}
