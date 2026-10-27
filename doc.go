package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/msuozzo/jj-highlight/syntax"

	"github.com/msuozzo/nit/review"
)

// Comment is a <review> block that skepsis wrote into the working copy.
type Comment struct {
	Open   int // new-side line of the open tag, what RemoveComment takes
	Anchor int // new-side line the card follows
	Body   string
}

type contentState uint8

const (
	contentNone contentState = iota
	contentLoading
	contentLoaded
	contentFailed
)

type File struct {
	Path, OldPath    string
	Status           byte // M A D R C
	Binary           bool
	Pseudo           bool // a commit message, not a working-copy file
	Note             string
	OldBlob, NewBlob string
	Hash             string
	Hunks            []*Hunk
	Adds, Dels       int
	Syntax           review.Syntax
	Comments         []*Comment
	Reviews          []*Card // one per comment
	Sent             []*Card

	Viewed, Collapsed bool

	content  contentState
	Old, New []string // full contents at each endpoint, nil until loaded
	pending  []func() // run once contents load
	hl       [2]map[int][]Seg
	hlFull   bool  // hl covers the full contents, not just the hunks
	gaps     []Gap // gaps[i] precedes Hunks[i], the last one follows the last hunk
	numW     int
	idx      int
}

// Gap is the unchanged stretch between two hunks. Top lines are revealed
// below the previous hunk and Bottom lines above the next one.
type Gap struct{ Top, Bottom int }

// Seg colors bytes up to End of a line.
type Seg struct {
	End   int
	Class syntax.Class
}

type CardKind uint8

const (
	CardReview CardKind = iota
	CardSent
	CardCompose
)

// Card is anything drawn between diff lines: a review comment from the file,
// a comment already sent, or the compose box.
type Card struct {
	Kind     CardKind
	File     *File
	Side     int // 0 deletions (old), 1 additions (new)
	Line     int // the card follows this line on Side
	From, To int // lines commented on
	Body     string
	Comment  *Comment
	Ed       *Editor
	Busy     string
	Err      string
	At       time.Time

	x0, x1 int      // pane columns, set by layout
	wrap   []string // body wrapped to the card's width
}

func (c *Card) draftKey() string {
	return c.File.Path + ":" + strconv.Itoa(c.Side) + ":" + strconv.Itoa(c.To)
}

func sideName(side int) string {
	if side == 0 {
		return "deletions"
	}
	return "additions"
}

func (f *File) name() string { return f.Path[strings.LastIndexByte(f.Path, '/')+1:] }

func (f *File) init(syn *review.Syntax) {
	if syn != nil {
		f.Syntax = *syn
	}
	f.extractComments()
	for _, c := range f.Comments {
		f.Reviews = append(f.Reviews, &Card{Kind: CardReview, File: f, Side: 1, Line: c.Anchor, Body: c.Body, Comment: c})
	}
	f.computeWords()
	f.gaps = make([]Gap, len(f.Hunks)+1)
	f.fitNumbers()
}

// fitNumbers sizes the line number column to the largest number shown.
func (f *File) fitNumbers() {
	n := 1
	for _, h := range f.Hunks {
		n = max(n, h.lastOld(), h.lastNew())
	}
	if g := f.gaps[len(f.Hunks)]; g.Top > 0 {
		_, newS, _ := f.gapInfo(len(f.Hunks))
		n = max(n, newS+g.Top)
	}
	f.numW = max(len(strconv.Itoa(n)), 3)
}

func (f *File) setContents(old, new *string) {
	f.content = contentLoaded
	f.Old, f.New = splitLines(old), splitLines(new)
	run := f.pending
	f.pending = nil
	for _, fn := range run {
		fn()
	}
}

// splitLines splits file contents into lines without their terminators. A
// missing file has no lines.
func splitLines(s *string) []string {
	if s == nil || strings.TrimSuffix(*s, "\n") == "" {
		return []string{}
	}
	ls := strings.Split(strings.TrimSuffix(*s, "\n"), "\n")
	for i, l := range ls {
		ls[i] = strings.TrimSuffix(l, "\r")
	}
	return ls
}

// gapInfo returns where gap i starts on each side and how many lines it
// hides. The gap after the last hunk is empty until the contents load.
func (f *File) gapInfo(i int) (oldStart, newStart, count int) {
	oldStart, newStart = 1, 1
	if i > 0 {
		p := f.Hunks[i-1]
		oldStart, newStart = p.lastOld()+1, p.lastNew()+1
	}
	switch {
	case i < len(f.Hunks):
		count = f.Hunks[i].firstNew() - newStart
	case f.Status != 'A' && f.Status != 'D' && len(f.Hunks) > 0 && f.content == contentLoaded:
		count = max(len(f.New)-newStart+1, 0)
	}
	return oldStart, newStart, count
}

func (f *File) gapLine(oldStart, newStart, k int) *Line {
	l := &Line{Kind: Ctx, Old: oldStart + k, New: newStart + k}
	if l.New-1 < len(f.New) {
		l.Text = f.New[l.New-1]
	} else if l.Old-1 < len(f.Old) {
		l.Text = f.Old[l.Old-1]
	}
	l.Width = textWidth(l.Text)
	return l
}

// expandGap reveals n more lines of gap i downward ('d') or upward ('u'),
// or the whole gap for any other dir.
func (f *File) expandGap(i int, dir byte, n int) {
	_, _, count := f.gapInfo(i)
	g := &f.gaps[i]
	hidden := count - g.Top - g.Bottom
	switch dir {
	case 'd':
		g.Top += min(n, hidden)
	case 'u':
		g.Bottom += min(n, hidden)
	default:
		g.Top += hidden
	}
}

// sideText returns line n of a side from the full contents, or from the
// hunks when the contents aren't loaded.
func (f *File) sideText(side, n int) (string, bool) {
	if n < 1 {
		return "", false
	}
	if f.content == contentLoaded {
		lines := f.Old
		if side == 1 {
			lines = f.New
		}
		if n <= len(lines) {
			return lines[n-1], true
		}
		return "", false
	}
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.num(side) == n {
				return l.Text, true
			}
		}
	}
	return "", false
}

// excerpt quotes the lines around from..to on a side, in runs of the
// consecutive lines that are known.
func (f *File) excerpt(side, from, to int) []review.CodeRun {
	var runs []review.CodeRun
	for n := from - 2; n <= to+2; n++ {
		t, ok := f.sideText(side, n)
		if !ok {
			continue
		}
		if k := len(runs); k > 0 && runs[k-1].Lines[1] == n-1 {
			runs[k-1].Lines[1], runs[k-1].Text = n, runs[k-1].Text+"\n"+t
		} else {
			runs = append(runs, review.CodeRun{Lines: [2]int{n, n}, Text: t})
		}
	}
	return runs
}

// extractComments lifts <review> blocks out of the hunks into cards, the
// way the web UI shows them. A block's lines must all be on the same side.
func (f *File) extractComments() {
	open, close := f.Syntax.Tag("<review>"), f.Syntax.Tag("</review>")
	closeAt := func(ls []*Line, i int) int {
		if ls[i].Kind == Del || !open.MatchString(ls[i].Text) {
			return -1
		}
		for j := i + 1; j < len(ls) && ls[j].Kind == ls[i].Kind; j++ {
			if close.MatchString(ls[j].Text) {
				return j
			}
		}
		return -1
	}
	for _, h := range f.Hunks {
		kept := make([]*Line, 0, len(h.Lines))
		for i := 0; i < len(h.Lines); i++ {
			l, j := h.Lines[i], closeAt(h.Lines, i)
			if j < 0 {
				kept = append(kept, l)
				continue
			}
			anchor := l.New - 1
			for k := len(kept) - 1; k >= 0; k-- {
				if kept[k].Kind != Del {
					anchor = kept[k].New
					break
				}
			}
			var body []string
			for _, b := range h.Lines[i+1 : j] {
				body = append(body, f.Syntax.Strip(b.Text))
			}
			f.Comments = append(f.Comments, &Comment{Open: l.New, Anchor: anchor, Body: strings.Join(body, "\n")})
			if l.Kind == Add {
				f.Adds -= j - i + 1
			}
			i = j
		}
		h.Lines = kept
	}
}

// block returns where the deletions and then the additions of the change
// block starting at ls[i] end. ls[i] must not be a context line.
func block(ls []*Line, i int) (dels, end int) {
	dels = i
	for dels < len(ls) && ls[dels].Kind == Del {
		dels++
	}
	end = dels
	for end < len(ls) && ls[end].Kind == Add {
		end++
	}
	return dels, end
}

// computeWords pairs each change block's deletions with its additions and
// marks the words that differ.
func (f *File) computeWords() {
	for _, h := range f.Hunks {
		ls := h.Lines
		for i := 0; i < len(ls); {
			if ls[i].Kind == Ctx {
				i++
				continue
			}
			j, k := block(ls, i)
			for p := range min(j-i, k-j) {
				wordDiff(ls[i+p], ls[j+p])
			}
			i = k
		}
	}
}

// tokenize splits s into runs of word characters, runs of blanks, and
// single other runes.
func tokenize(s string) []Span {
	class := func(r rune) int {
		switch {
		case isWord(r):
			return 1
		case r == ' ' || r == '\t':
			return 2
		}
		return 0
	}
	var ts []Span
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		j := i + n
		if c := class(r); c != 0 {
			for j < len(s) {
				r2, n2 := utf8.DecodeRuneInString(s[j:])
				if class(r2) != c {
					break
				}
				j += n2
			}
		}
		ts = append(ts, Span{i, j})
		i = j
	}
	return ts
}

// wordDiff marks the tokens of a and b outside their longest common
// subsequence, unless the lines share too little to read as one edit.
func wordDiff(a, b *Line) {
	ta, tb := tokenize(a.Text), tokenize(b.Text)
	eq := func(i, j int) bool { return a.Text[ta[i].A:ta[i].B] == b.Text[tb[j].A:tb[j].B] }
	pre := 0
	for pre < len(ta) && pre < len(tb) && eq(pre, pre) {
		pre++
	}
	suf := 0
	for suf < len(ta)-pre && suf < len(tb)-pre && eq(len(ta)-1-suf, len(tb)-1-suf) {
		suf++
	}
	n, m := len(ta)-pre-suf, len(tb)-pre-suf
	if n*m > 120000 {
		return
	}
	keepA, keepB := make([]bool, len(ta)), make([]bool, len(tb))
	for i := range pre {
		keepA[i], keepB[i] = true, true
	}
	for i := range suf {
		keepA[len(ta)-1-i], keepB[len(tb)-1-i] = true, true
	}
	if n > 0 && m > 0 {
		w := m + 1
		dp := make([]uint16, (n+1)*w)
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				if eq(pre+i, pre+j) {
					dp[i*w+j] = dp[(i+1)*w+j+1] + 1
				} else {
					dp[i*w+j] = max(dp[(i+1)*w+j], dp[i*w+j+1])
				}
			}
		}
		for i, j := 0, 0; i < n && j < m; {
			switch {
			case eq(pre+i, pre+j):
				keepA[pre+i], keepB[pre+j] = true, true
				i, j = i+1, j+1
			case dp[(i+1)*w+j] >= dp[i*w+j+1]:
				i++
			default:
				j++
			}
		}
	}
	// Lines with under 40% of their word bytes in common read better unmarked.
	common := func(ts []Span, keep []bool, s string) (same, total int) {
		for i, t := range ts {
			if r, _ := utf8.DecodeRuneInString(s[t.A:]); !isWord(r) {
				continue
			}
			total += t.B - t.A
			if keep[i] {
				same += t.B - t.A
			}
		}
		return
	}
	sa, na := common(ta, keepA, a.Text)
	sb, nb := common(tb, keepB, b.Text)
	if na > 0 && nb > 0 && (sa*10 < na*4 || sb*10 < nb*4) {
		return
	}
	a.Words = changedSpans(ta, keepA, a.Text)
	b.Words = changedSpans(tb, keepB, b.Text)
}

// changedSpans merges the unkept tokens into spans, bridging a single blank
// between two changes.
func changedSpans(ts []Span, keep []bool, s string) []Span {
	var out []Span
	for i, t := range ts {
		if keep[i] {
			continue
		}
		if n := len(out); n > 0 {
			if gap := s[out[n-1].B:t.A]; len(gap) <= 1 && strings.TrimSpace(gap) == "" {
				out[n-1].B = t.B
				continue
			}
		}
		out = append(out, t)
	}
	return out
}
