package main

import (
	"slices"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// Editor is the compose box's text: a grid of runes with a cursor, soft
// wrapped to the box's width when drawn.
type Editor struct {
	lines    [][]rune
	row, col int
	width    int // wrap width from the last layout
}

func newEditor(text string) *Editor {
	e := &Editor{lines: [][]rune{{}}}
	e.InsertText(text)
	return e
}

func (e *Editor) Text() string {
	parts := make([]string, len(e.lines))
	for i, l := range e.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

// InsertText types s at the cursor, dropping control characters other than
// tabs and newlines.
func (e *Editor) InsertText(s string) {
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
	for _, r := range s {
		if r == '\n' {
			e.Newline()
		} else if r == '\t' || !unicode.IsControl(r) {
			e.Insert(r)
		}
	}
}

func (e *Editor) Insert(r rune) {
	e.lines[e.row] = slices.Insert(e.lines[e.row], e.col, r)
	e.col++
}

func (e *Editor) Newline() {
	l := e.lines[e.row]
	rest := slices.Clone(l[e.col:])
	e.lines[e.row] = l[:e.col]
	e.lines = slices.Insert(e.lines, e.row+1, rest)
	e.row, e.col = e.row+1, 0
}

func (e *Editor) Backspace() {
	if e.col > 0 {
		e.lines[e.row] = slices.Delete(e.lines[e.row], e.col-1, e.col)
		e.col--
	} else if e.row > 0 {
		e.col = len(e.lines[e.row-1])
		e.joinNext(e.row - 1)
		e.row--
	}
}

func (e *Editor) Delete() {
	if l := e.lines[e.row]; e.col < len(l) {
		e.lines[e.row] = slices.Delete(l, e.col, e.col+1)
	} else if e.row+1 < len(e.lines) {
		e.joinNext(e.row)
	}
}

// joinNext appends line r+1 to line r.
func (e *Editor) joinNext(r int) {
	e.lines[r] = append(e.lines[r], e.lines[r+1]...)
	e.lines = slices.Delete(e.lines, r+1, r+2)
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func (e *Editor) wordLeft() int {
	l, c := e.lines[e.row], e.col
	for c > 0 && !isWord(l[c-1]) {
		c--
	}
	for c > 0 && isWord(l[c-1]) {
		c--
	}
	return c
}

func (e *Editor) wordRight() int {
	l, c := e.lines[e.row], e.col
	for c < len(l) && !isWord(l[c]) {
		c++
	}
	for c < len(l) && isWord(l[c]) {
		c++
	}
	return c
}

// Key applies an editing key and reports whether it was one.
func (e *Editor) Key(k string) bool {
	l := e.lines[e.row]
	switch k {
	case "left", "C-b":
		if e.col > 0 {
			e.col--
		} else if e.row > 0 {
			e.row--
			e.col = len(e.lines[e.row])
		}
	case "right", "C-f":
		if e.col < len(l) {
			e.col++
		} else if e.row+1 < len(e.lines) {
			e.row, e.col = e.row+1, 0
		}
	case "up", "C-p":
		e.vmove(-1)
	case "down", "C-n":
		e.vmove(1)
	case "home", "C-a":
		e.col = 0
	case "end", "C-e":
		e.col = len(l)
	case "M-b", "M-left", "C-left":
		e.col = e.wordLeft()
	case "M-f", "M-right", "C-right":
		e.col = e.wordRight()
	case "bs", "S-bs":
		e.Backspace()
	case "del", "C-d":
		e.Delete()
	case "C-w", "M-bs", "C-bs":
		c := e.wordLeft()
		e.lines[e.row] = slices.Delete(l, c, e.col)
		e.col = c
	case "M-d":
		e.lines[e.row] = slices.Delete(l, e.col, e.wordRight())
	case "C-u":
		e.lines[e.row] = l[e.col:]
		e.col = 0
	case "C-k":
		if e.col == len(l) {
			e.Delete()
		} else {
			e.lines[e.row] = l[:e.col]
		}
	case "S-enter", "M-enter", "C-j":
		e.Newline()
	default:
		return false
	}
	return true
}

// vline is a visual line: runes [a, b) of a logical row.
type vline struct{ row, a, b int }

// wrap soft-wraps the text at word boundaries to w columns.
func (e *Editor) wrap(w int) []vline {
	w = max(w, 4)
	e.width = w
	var out []vline
	for r, l := range e.lines {
		a := 0
		for {
			col, b, lastSpace := 0, a, -1
			for b < len(l) {
				cw := runewidth.RuneWidth(l[b])
				if col+cw > w {
					break
				}
				if l[b] == ' ' {
					lastSpace = b
				}
				col += cw
				b++
			}
			if b >= len(l) {
				out = append(out, vline{r, a, len(l)})
				break
			}
			if lastSpace > a {
				b = lastSpace + 1
			}
			out = append(out, vline{r, a, b})
			a = b
		}
	}
	return out
}

// cursorVis returns the visual line holding the cursor and its column.
func (e *Editor) cursorVis(vl []vline) (int, int) {
	for i, v := range vl {
		last := i+1 == len(vl) || vl[i+1].row != v.row
		if v.row == e.row && e.col >= v.a && (e.col < v.b || last) {
			return i, runewidth.StringWidth(string(e.lines[v.row][v.a:e.col]))
		}
	}
	return 0, 0
}

// setVis puts the cursor on visual line v at column x, or its end.
func (e *Editor) setVis(v vline, x int) {
	col, c := 0, v.a
	for ; c < v.b; c++ {
		cw := runewidth.RuneWidth(e.lines[v.row][c])
		if col+cw > x {
			break
		}
		col += cw
	}
	e.row, e.col = v.row, c
}

func (e *Editor) vmove(d int) {
	vl := e.wrap(e.width)
	i, x := e.cursorVis(vl)
	if j := i + d; j >= 0 && j < len(vl) {
		e.setVis(vl[j], x)
	} else if d < 0 {
		e.col = 0
	} else {
		e.col = len(e.lines[e.row])
	}
}
