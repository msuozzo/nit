package main

import (
	"slices"
	"strings"
)

type RowKind uint8

const (
	RowFile RowKind = iota
	RowGap
	RowLine
	RowCard
	RowNote
)

// Row is one screen row of the diff pane. In split mode a line row holds the
// old side in L and the new side in R. In unified mode it holds one line in L.
type Row struct {
	Kind RowKind
	F    *File
	L, R *Line
	Gap  int // gap index, for RowGap
	Card *Card
	Sub  int // row within the card, for RowCard
	Note string
}

type Layout struct {
	Rows    []Row
	Split   bool
	W       int
	fileAt  map[*File]int
	changes []int // first row of each run of changed lines
}

// Column geometry. Unified rows are
//
//	mark | old no | sp | new no | sp | sign | sp | code
//
// and each split half is mark | no | sp | sign | sp | code.
func (lay *Layout) half() int { return (lay.W - 1) / 2 }

func (lay *Layout) codeX(f *File, side int) int {
	switch {
	case !lay.Split:
		return 2*f.numW + 5
	case side == 0:
		return f.numW + 4
	}
	return lay.half() + 1 + f.numW + 4
}

func (lay *Layout) region(side int) (int, int) {
	switch {
	case !lay.Split:
		return 0, lay.W
	case side == 0:
		return 0, lay.half()
	}
	return lay.half() + 1, lay.W
}

func buildLayout(files []*File, split bool, w int, compose *Card) *Layout {
	lay := &Layout{Split: split, W: w, fileAt: map[*File]int{}}
	for _, f := range files {
		f.fitNumbers()
		lay.fileAt[f] = len(lay.Rows)
		lay.add(Row{Kind: RowFile, F: f})
		if f.Collapsed {
			continue
		}
		cards := slices.Concat(f.Reviews, f.Sent)
		if compose != nil && compose.File == f {
			cards = append(cards, compose)
		}
		at := map[[2]int][]*Card{}
		for _, c := range cards {
			at[[2]int{c.Side, c.Line}] = append(at[[2]int{c.Side, c.Line}], c)
		}
		done := map[*Card]bool{}
		emit := func(side, line int) {
			for _, c := range at[[2]int{side, line}] {
				if !done[c] {
					done[c] = true
					lay.addCard(c)
				}
			}
		}
		emit(0, 0)
		emit(1, 0)
		switch {
		case f.Binary:
			lay.add(Row{Kind: RowNote, F: f, Note: "Binary file not shown"})
		case len(f.Hunks) == 0 && f.Note == "" && f.Status == 'R':
			lay.add(Row{Kind: RowNote, F: f, Note: "Renamed without changes"})
		case len(f.Hunks) == 0 && f.Note == "":
			lay.add(Row{Kind: RowNote, F: f, Note: "No content changes"})
		}
		if f.Note != "" {
			lay.add(Row{Kind: RowNote, F: f, Note: f.Note})
		}
		row := func(L, R *Line) {
			r := Row{Kind: RowLine, F: f, L: L, R: R}
			lay.add(r)
			for side := range 2 {
				if l := r.lineOn(side, split); l != nil {
					emit(side, l.num(side))
				}
			}
		}
		ctx := func(l *Line) {
			if split {
				row(l, l)
			} else {
				row(l, nil)
			}
		}
		for i := 0; i <= len(f.Hunks); i++ {
			oldS, newS, count := f.gapInfo(i)
			g := f.gaps[i]
			for k := range g.Top {
				ctx(f.gapLine(oldS, newS, k))
			}
			if count-g.Top-g.Bottom > 0 {
				lay.add(Row{Kind: RowGap, F: f, Gap: i})
			}
			for k := count - g.Bottom; k < count; k++ {
				ctx(f.gapLine(oldS, newS, k))
			}
			if i == len(f.Hunks) {
				break
			}
			ls := f.Hunks[i].Lines
			for a := 0; a < len(ls); {
				if ls[a].Kind == Ctx {
					ctx(ls[a])
					a++
					continue
				}
				lay.changes = append(lay.changes, len(lay.Rows))
				b, c := block(ls, a)
				switch {
				case !split:
					for _, l := range ls[a:c] {
						row(l, nil)
					}
				default:
					for p := range max(b-a, c-b) {
						var L, R *Line
						if p < b-a {
							L = ls[a+p]
						}
						if p < c-b {
							R = ls[b+p]
						}
						row(L, R)
					}
				}
				a = c
			}
		}
		for _, c := range cards {
			if !done[c] {
				lay.addCard(c)
			}
		}
	}
	return lay
}

func (lay *Layout) add(r Row) { lay.Rows = append(lay.Rows, r) }

// addCard sizes a card to its side's code column and adds its rows.
func (lay *Layout) addCard(c *Card) {
	x0, x1 := lay.region(c.Side)
	c.x0, c.x1 = lay.codeX(c.File, c.Side)-2, x1
	if c.x1-c.x0 < 24 {
		c.x0 = max(x0, c.x1-24)
	}
	w := max(c.x1-c.x0-3, 8)
	n := 1
	if c.Kind == CardCompose {
		n += max(len(c.Ed.wrap(w)), 2) + 1
		c.wrap = nil
		if c.Err != "" {
			c.wrap = wrapText(c.Err, w)
		}
	} else {
		c.wrap = wrapText(c.Body, w)
	}
	n += len(c.wrap)
	for i := range n {
		lay.add(Row{Kind: RowCard, F: c.File, Card: c, Sub: i})
	}
}

// lineOn returns the line a row shows on a side, or nil.
func (r *Row) lineOn(side int, split bool) *Line {
	switch {
	case r.Kind != RowLine:
		return nil
	case split && side == 0:
		return r.L
	case split:
		return r.R
	case side == 0 && r.L.Kind != Add, side == 1 && r.L.Kind != Del:
		return r.L
	}
	return nil
}

// rowKey names a row by its content, so it can be found after a rebuild.
type rowKey struct {
	path       string
	kind       RowKind
	side, line int // line number for RowLine and RowCard, gap index for RowGap
	card       *Card
	sub        int
}

func (lay *Layout) key(i int) rowKey {
	if i < 0 || i >= len(lay.Rows) {
		return rowKey{}
	}
	r := &lay.Rows[i]
	k := rowKey{path: r.F.Path, kind: r.Kind, card: r.Card, sub: r.Sub, line: r.Gap}
	switch r.Kind {
	case RowLine:
		if l := r.lineOn(1, lay.Split); l != nil {
			k.side, k.line = 1, l.New
		} else {
			k.side, k.line = 0, r.L.Old
		}
	case RowCard:
		k.side, k.line = r.Card.Side, r.Card.Line
	}
	return k
}

// find locates the row a key named, or the nearest line row before it in
// the same file, or the file's header.
func (lay *Layout) find(k rowKey, files map[string]*File) int {
	f := files[k.path]
	start, ok := lay.fileAt[f]
	if !ok {
		return 0
	}
	best := start
	for i := start; i < len(lay.Rows) && lay.Rows[i].F == f; i++ {
		r := &lay.Rows[i]
		switch {
		case r.Kind != k.kind:
		case r.Kind == RowFile, r.Kind == RowCard && r.Card == k.card && r.Sub == k.sub, r.Kind == RowGap && r.Gap == k.line:
			return i
		}
		if r.Kind == RowLine && (k.kind == RowLine || k.kind == RowCard) {
			if l := r.lineOn(k.side, lay.Split); l != nil {
				if n := l.num(k.side); n == k.line {
					return i
				} else if n < k.line {
					best = i
				}
			}
		}
	}
	return best
}

// wrapText soft-wraps each paragraph of s to w columns.
func wrapText(s string, w int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		e := newEditor(para)
		for _, v := range e.wrap(w) {
			out = append(out, string(e.lines[v.row][v.a:v.b]))
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}
