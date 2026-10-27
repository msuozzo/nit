package main

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

func (a *App) set(x, y int, r rune, st tcell.Style) { a.s.SetContent(x, y, r, nil, st) }

func (a *App) fill(x0, x1, y int, st tcell.Style) {
	for x := x0; x < x1; x++ {
		a.s.SetContent(x, y, ' ', nil, st)
	}
}

// put draws s from x, clipped before maxX, and returns the column after it.
func (a *App) put(x, y, maxX int, s string, st tcell.Style) int {
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w == 0 {
			continue
		}
		if x+w > maxX {
			break
		}
		a.s.SetContent(x, y, r, nil, st)
		x += w
	}
	return x
}

func width(s string) int { return runewidth.StringWidth(s) }

// putR draws s to end at x1 and returns where it starts. It draws nothing
// when s would cross minX.
func (a *App) putR(x1, y, minX int, s string, st tcell.Style) int {
	x := x1 - width(s)
	if x < minX {
		return x1
	}
	a.put(x, y, x1, s, st)
	return x
}

func (a *App) hovered(y, x0, x1 int) bool {
	return a.hoverY == y && a.hoverX >= x0 && a.hoverX < x1 && a.drag == dragNone
}

// button draws a clickable label and returns the column after it.
func (a *App) button(x, y, maxX int, label string, st, hover tcell.Style, fn func(int)) int {
	w := width(label)
	if x+w > maxX {
		return x
	}
	if a.hovered(y, x, x+w) {
		st = hover
	}
	a.put(x, y, maxX, label, st)
	a.hots = append(a.hots, hot{y, x, x + w, fn})
	return x + w
}

// buttonR draws a button ending at x1 and returns where it starts.
func (a *App) buttonR(x1, y, minX int, label string, st, hover tcell.Style, fn func(int)) int {
	x := x1 - width(label)
	if x < minX {
		return x1
	}
	a.button(x, y, x1, label, st, hover, fn)
	return x
}

func ellipsis(s string, w int) string {
	if width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}

// ellipsisLeft keeps the end of s, for paths.
func ellipsisLeft(s string, w int) string {
	if width(s) <= w {
		return s
	}
	rs := []rune(s)
	for len(rs) > 0 && runewidth.StringWidth(string(rs))+1 > w {
		rs = rs[1:]
	}
	return "…" + string(rs)
}

func (a *App) statusColor(st byte) tcell.Color {
	switch st {
	case 'A':
		return a.th.Green
	case 'D':
		return a.th.Red
	case 'R', 'C':
		return a.th.Purple
	}
	return a.th.Amber
}

func (a *App) draw() {
	th := a.th
	a.hots = a.hots[:0]
	a.s.SetStyle(th.st(th.Fg, th.Bg))
	a.s.HideCursor()
	if a.d == nil || a.lay == nil {
		a.s.Fill(' ', th.st(th.Fg, th.Bg))
		msg, st := "loading diff…", th.st(th.Muted, th.Bg)
		if a.err != "" {
			msg, st = "✗ "+a.err, th.st(th.Red, th.Bg)
		}
		a.put(max((a.W-width(msg))/2, 0), a.H/2, a.W, msg, st)
		a.s.Show()
		return
	}
	a.treeFollow()
	a.drawTitle()
	if a.tw > 0 {
		a.drawTree()
	}
	a.drawPane()
	a.drawScrollbar()
	a.drawStatus()
	if a.stale {
		a.drawChanged()
	}
	if a.help {
		a.drawHelp()
	}
	a.s.Show()
}

func (a *App) drawTitle() {
	th := a.th
	bg := th.Panel
	base, btn, btnHover := th.st(th.Muted, bg), th.st(th.Muted, bg), th.st(th.Bg, th.Accent)
	a.fill(0, a.W, 0, base)
	x := a.put(1, 0, a.W, " nit ", th.st(th.Bg, th.Accent).Bold(true))
	x = a.put(x+1, 0, a.W, a.repo.Name(), th.st(th.Fg, bg).Bold(true))
	adds, dels, viewed := 0, 0, 0
	for _, f := range a.files {
		adds, dels, viewed = adds+f.Adds, dels+f.Dels, viewed+b2i(f.Viewed)
	}
	rx := a.W - 1
	rx = a.buttonR(rx, 0, x, " ? ", btn, btnHover, func(int) { a.help = true }) - 1
	themeLabel := " ◑ dark "
	if !th.Dark {
		themeLabel = " ◐ light "
	}
	rx = a.buttonR(rx, 0, x, themeLabel, btn, btnHover, func(int) { a.toggleTheme() }) - 1
	mode := " ▥ split "
	if !a.lay.Split {
		mode = " ▤ unified "
	}
	rx = a.buttonR(rx, 0, x, mode, btn, btnHover, func(int) { a.toggleSplit() }) - 2
	if n := len(a.files); n > 0 {
		const barW = 10
		fillN := viewed * barW / n
		if rx-barW-14 > x {
			rx = a.putR(rx, 0, x, strings.Repeat("━", barW-fillN), th.st(th.Faint, bg))
			rx = a.putR(rx, 0, x, strings.Repeat("━", fillN), th.st(th.Green, bg)) - 1
			rx = a.putR(rx, 0, x, fmt.Sprintf("%d/%d viewed", viewed, n), base) - 2
		}
	}
	stats, add, del := fmt.Sprintf("%d files", len(a.files)), "+"+strconv.Itoa(adds), "−"+strconv.Itoa(dels)
	if revW := rx - x - width(stats) - width(add) - width(del) - 7; revW > 8 {
		x = a.put(x+1, 0, x+1+revW, ellipsis(a.repo.Command(), revW), th.st(th.Subtle, bg))
	}
	x = a.put(x+2, 0, rx, stats, base)
	x = a.put(x+1, 0, rx, add, th.st(th.Green, bg))
	a.put(x+1, 0, rx, del, th.st(th.Red, bg))
}

func (a *App) drawTree() {
	th := a.th
	bg := th.Panel
	x1 := a.tw
	a.fill(0, x1, a.py, th.st(th.Muted, bg))
	a.put(1, a.py, x1, "FILES", th.st(th.Subtle, bg).Bold(true))
	a.putR(x1-1, a.py, 7, strconv.Itoa(len(a.files)), th.st(th.Subtle, bg))
	vis := a.ph - 1
	a.treeTop = clamp(a.treeTop, 0, max(len(a.trows)-vis, 0))
	var cf *File
	if a.cur < len(a.lay.Rows) {
		cf = a.lay.Rows[a.cur].F
	}
	for k := range vis {
		y, i := a.py+1+k, a.treeTop+k
		if i >= len(a.trows) {
			a.fill(0, x1, y, th.st(th.Fg, bg))
			continue
		}
		n, depth := a.trows[i].N, a.trows[i].Depth
		rbg := bg
		isCur := n.File != nil && n.File == cf
		focused := a.focusTree && i == a.treeCur
		switch {
		case focused:
			rbg = mix(bg, th.Accent, 0.30)
		case isCur:
			rbg = mix(bg, th.Accent, 0.15)
		case a.hovered(y, 0, x1):
			rbg = mix(bg, th.Fg, 0.06)
		}
		a.fill(0, x1, y, th.st(th.Fg, rbg))
		if isCur || focused {
			a.set(0, y, '▎', th.st(th.Accent, rbg))
		}
		x := 1 + depth*2
		if n.Dir {
			ch := "▾ "
			if n.Closed {
				ch = "▸ "
			}
			x = a.put(x, y, x1, ch, th.st(th.Subtle, rbg))
			a.put(x, y, x1-1, ellipsis(n.Name+"/", x1-1-x), th.st(th.Muted, rbg))
			continue
		}
		f := n.File
		x = a.put(x, y, x1, string(f.Status), th.st(a.statusColor(f.Status), rbg).Bold(true)) + 1
		rx := x1 - 1
		if f.Viewed {
			rx = a.putR(rx, y, x+6, "✓", th.st(th.Green, rbg)) - 1
		}
		if c := len(f.Comments); c > 0 {
			rx = a.putR(rx, y, x+6, "✎"+strconv.Itoa(c), th.st(th.Amber, rbg)) - 1
		}
		if !f.Viewed && x1-x > 22 {
			if f.Dels > 0 {
				rx = a.putR(rx, y, x+8, "−"+strconv.Itoa(f.Dels), th.st(mix(th.Red, rbg, 0.25), rbg))
			}
			if f.Adds > 0 {
				rx = a.putR(rx-1, y, x+8, "+"+strconv.Itoa(f.Adds), th.st(mix(th.Green, rbg, 0.25), rbg))
			}
			rx--
		}
		st := th.st(th.Fg, rbg)
		if f.Viewed {
			st = th.st(th.Subtle, rbg)
		}
		if isCur {
			st = st.Bold(true)
		}
		a.put(x, y, rx, ellipsis(n.Name, rx-x), st)
	}
	dst := th.st(th.Border, th.Bg)
	if a.drag == dragDivider || (a.hoverX == a.tw && a.hoverY > 0 && a.hoverY < a.H-1) {
		dst = th.st(th.Accent, th.Bg)
	}
	for y := a.py; y < a.py+a.ph; y++ {
		a.set(a.tw, y, '│', dst)
	}
}

func (a *App) drawPane() {
	rows := a.lay.Rows
	if len(rows) == 0 {
		for y := a.py; y < a.py+a.ph; y++ {
			a.fill(a.px, a.px+a.pw, y, a.th.st(a.th.Fg, a.th.Bg))
		}
		msg := "no changes in " + a.repo.Command()
		a.put(a.px+max((a.pw-width(msg))/2, 0), a.py+a.ph/2, a.px+a.pw, msg, a.th.st(a.th.Muted, a.th.Bg))
		return
	}
	for y := range a.ph {
		i, sy := a.top+y, a.py+y
		if i >= len(rows) {
			a.fill(a.px, a.px+a.pw, sy, a.th.st(a.th.Fg, a.th.Bg))
			continue
		}
		r := &rows[i]
		switch r.Kind {
		case RowFile:
			a.drawHeader(r.F, sy, i, "")
		case RowGap:
			a.drawGap(r, sy, i)
		case RowLine:
			a.drawLine(r, sy, i)
		case RowCard:
			a.drawCard(r, sy, i)
		case RowNote:
			a.fill(a.px, a.px+a.pw, sy, a.th.st(a.th.Fg, a.th.Bg))
			a.put(a.px+a.lay.codeX(r.F, 0), sy, a.px+a.pw, r.Note, a.th.st(a.th.Muted, a.th.Bg).Italic(true))
		}
	}
	if a.top < len(rows) && rows[a.top].Kind != RowFile {
		f, crumb := rows[a.top].F, ""
		if g := a.gapNear(f, a.top, -1); g >= 0 && g < len(f.Hunks) {
			crumb = f.Hunks[g].Heading
		}
		a.drawHeader(f, a.py, -1, crumb)
	}
}

// drawHeader draws a file's header row. The sticky copy pinned to the top
// of the pane has row -1 and names the hunk it is in.
func (a *App) drawHeader(f *File, y, row int, crumb string) {
	th := a.th
	bg := th.FileBg
	x0, x1 := a.px, a.px+a.pw
	cursor := row >= 0 && row == a.cur && !a.focusTree
	if cursor {
		bg = mix(bg, th.Accent, 0.20)
	} else if a.hovered(y, x0, x1) {
		bg = mix(bg, th.Fg, 0.04)
	}
	a.fill(x0, x1, y, th.st(th.Fg, bg))
	if cursor {
		a.set(x0, y, '▎', th.st(th.Accent, bg))
	}
	toggle := func(int) { a.toggleCollapse(f) }
	if row < 0 {
		toggle = func(int) { a.jumpFile(f) }
	}
	chev := "▾"
	if f.Collapsed {
		chev = "▸"
	}
	x := a.button(x0+1, y, x1, chev, th.st(th.Muted, bg), th.st(th.Accent, bg), toggle) + 1
	x = a.put(x, y, x1, string(f.Status), th.st(a.statusColor(f.Status), bg).Bold(true)) + 1
	rx := x1 - 1
	vl, vst := "☐ Viewed", th.st(th.Muted, bg)
	if f.Viewed {
		vl, vst = "☑ Viewed", th.st(th.Green, bg)
	}
	rx = a.buttonR(rx, y, x+12, vl, vst, th.st(th.Accent, bg), func(int) { a.toggleViewed(f) }) - 2
	if c := len(f.Comments); c > 0 {
		rx = a.putR(rx, y, x+12, "✎ "+strconv.Itoa(c), th.st(th.Amber, bg)) - 2
	}
	if c := len(f.Sent); c > 0 {
		rx = a.putR(rx, y, x+12, "➜ "+strconv.Itoa(c), th.st(th.Green, bg)) - 2
	}
	if f.content == contentLoading {
		rx = a.putR(rx, y, x+12, "⋯", th.st(th.Subtle, bg)) - 2
	}
	add, del := "+"+strconv.Itoa(f.Adds), "−"+strconv.Itoa(f.Dels)
	avail := rx - x - width(add) - width(del) - 8
	dir, base := path.Split(f.Path)
	full := dir + base
	if f.OldPath != f.Path && (f.Status == 'R' || f.Status == 'C') {
		full = f.OldPath + " → " + full
	}
	if width(full) > avail {
		full = ellipsisLeft(full, max(avail, 4))
	}
	bi := 0
	if strings.HasSuffix(full, base) {
		bi = len(full) - len(base)
	}
	start := x
	x = a.put(x, y, x+avail, full[:bi], th.st(th.Muted, bg))
	x = a.put(x, y, x+avail, full[bi:], th.st(th.Fg, bg).Bold(true))
	a.hots = append(a.hots, hot{y, start, x, toggle})
	x = a.put(x+2, y, rx, add, th.st(th.Green, bg))
	x = a.put(x+1, y, rx, del, th.st(th.Red, bg)) + 1
	if tot := f.Adds + f.Dels; tot > 0 {
		g := (f.Adds*5 + tot/2) / tot
		for k := range 5 {
			c := th.Faint
			if k < g {
				c = th.Green
			} else if f.Dels > 0 {
				c = th.Red
			}
			x = a.put(x, y, rx, "■", th.st(c, bg))
		}
	}
	if crumb != "" && rx-x > 12 {
		a.put(x+2, y, rx, "› "+ellipsis(crumb, rx-x-4), th.st(th.Subtle, bg).Italic(true))
	}
}

func (a *App) drawGap(r *Row, y, i int) {
	th := a.th
	f, n := r.F, r.Gap
	bg := th.HunkBg
	cursor := i == a.cur && !a.focusTree
	if cursor {
		bg = mix(bg, th.Accent, 0.18)
	}
	x0, x1 := a.px, a.px+a.pw
	a.fill(x0, x1, y, th.st(th.HunkFg, bg))
	if cursor {
		a.set(x0, y, '▎', th.st(th.Accent, bg))
	}
	_, _, count := f.gapInfo(n)
	hidden := count - f.gaps[n].Top - f.gaps[n].Bottom
	a.put(x0+2, y, x1, "⋯", th.st(th.Subtle, bg))
	x := x0 + a.lay.codeX(f, 0) - 2
	btn, hov := th.st(th.Accent, bg), th.st(th.Bg, th.Accent)
	switch {
	case !a.repo.Expandable():
		x = a.put(x, y, x1, fmt.Sprintf("%d lines hidden", hidden), th.st(th.Muted, bg)) + 2
	case f.content == contentLoading && len(f.pending) > 0:
		x = a.put(x, y, x1, "loading…", th.st(th.Muted, bg)) + 2
	default:
		exp := func(dir byte, k int) func(int) {
			return func(int) { a.ensureContents(f, func() { f.expandGap(n, dir, k) }) }
		}
		if hidden > 20 && n > 0 {
			x = a.button(x, y, x1, " ↓ 20 ", btn, hov, exp('d', 20)) + 1
		}
		if hidden > 20 && n < len(f.Hunks) {
			x = a.button(x, y, x1, " ↑ 20 ", btn, hov, exp('u', 20)) + 1
		}
		label := fmt.Sprintf(" ↕ %d lines ", hidden)
		if hidden == 1 {
			label = " ↕ 1 line "
		}
		x = a.button(x, y, x1, label, btn, hov, exp('a', 0)) + 2
	}
	if n < len(f.Hunks) {
		h := f.Hunks[n]
		x = a.put(x, y, x1, fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount), th.st(th.Subtle, bg)) + 1
		if h.Heading != "" {
			a.put(x, y, x1, h.Heading, th.st(th.Muted, bg).Italic(true))
		}
	}
}

// lineColors returns the code, number column, and changed-word backgrounds
// for a line.
func (a *App) lineColors(l *Line) (code, num, word tcell.Color) {
	th := a.th
	switch l.Kind {
	case Add:
		return th.AddBg, th.AddNum, th.AddWord
	case Del:
		return th.DelBg, th.DelNum, th.DelWord
	}
	return th.Bg, th.Bg, th.Bg
}

// tint shades a row's background for selection, the cursor, or hover.
func (a *App) tint(row int, c tcell.Color) tcell.Color {
	switch {
	case a.sel >= 0 && row >= min(a.sel, a.cur) && row <= max(a.sel, a.cur):
		return mix(c, a.th.Accent, 0.24)
	case row == a.cur && !a.focusTree:
		return mix(c, a.th.Accent, 0.09)
	case row == a.hoverRow && a.drag == dragNone:
		return mix(c, a.th.Fg, 0.03)
	}
	return c
}

func (a *App) drawLine(r *Row, y, i int) {
	if !a.lay.Split {
		a.drawSide(a.px, a.px+a.pw, y, i, r.L, b2i(r.L.Kind != Del), r.F)
		return
	}
	lw := a.lay.half()
	a.drawSide(a.px, a.px+lw, y, i, r.L, 0, r.F)
	a.set(a.px+lw, y, '│', a.th.st(a.th.Faint, a.th.Bg))
	a.drawSide(a.px+lw+1, a.px+a.pw, y, i, r.R, 1, r.F)
}

// numCell right-aligns a line number in a w-wide cell, blank for 0.
func (a *App) numCell(x, y, w, n int, st tcell.Style) {
	s := ""
	if n > 0 {
		s = strconv.Itoa(n)
	}
	a.fill(x, x+w-width(s), y, st)
	a.put(x+w-width(s), y, x+w, s, st)
}

// drawSide draws one side of a line row: cursor mark, number column, sign
// and code. A nil line is the blank half opposite a lone change.
func (a *App) drawSide(x0, x1, y, row int, l *Line, side int, f *File) {
	th := a.th
	if l == nil {
		a.fill(x0, x1, y, th.st(th.Fg, a.tint(row, mix(th.Bg, th.Panel, 0.55))))
		return
	}
	code, num, word := a.lineColors(l)
	code, num, word = a.tint(row, code), a.tint(row, num), a.tint(row, word)
	mark := row == a.cur && !a.focusTree && (!a.lay.Split || a.side == side)
	numSt := th.st(th.Subtle, num)
	if mark {
		numSt = th.st(th.Fg, num)
		a.set(x0, y, '▎', th.st(th.Accent, num))
	} else {
		a.set(x0, y, ' ', numSt)
	}
	x := x0 + 1
	if a.lay.Split {
		a.numCell(x, y, f.numW, l.num(side), numSt)
		x += f.numW
	} else {
		a.numCell(x, y, f.numW, l.Old, numSt)
		a.set(x+f.numW, y, ' ', numSt)
		a.numCell(x+f.numW+1, y, f.numW, l.New, numSt)
		x += 2*f.numW + 1
	}
	a.set(x, y, ' ', numSt)
	x++
	sign, sc := ' ', th.Subtle
	switch l.Kind {
	case Add:
		sign, sc = '+', th.Green
	case Del:
		sign, sc = '-', th.Red
	}
	if row == a.hoverRow && side == a.hoverSide && a.drag == dragNone && a.compose == nil {
		a.set(x, y, '+', th.st(th.Bg, th.Accent).Bold(true))
	} else {
		a.set(x, y, sign, th.st(sc, code))
	}
	a.set(x+1, y, ' ', th.st(th.Fg, code))
	a.drawCode(x+2, x1, y, row, l, side, f, code, word)
}

// drawCode draws a line's text with syntax colors, changed-word and search
// highlights, tabs expanded, and control characters shown as dots.
func (a *App) drawCode(x0, x1, y, row int, l *Line, side int, f *File, bg, wordBg tcell.Color) {
	th := a.th
	a.fill(x0, x1, y, th.st(th.Fg, bg))
	hs := side
	switch l.Kind {
	case Del:
		hs = 0
	case Add:
		hs = 1
	}
	segs := f.hl[hs][l.num(hs)]
	if segs == nil && l.Kind == Ctx {
		segs = f.hl[1-hs][l.num(1-hs)]
	}
	var ms []match
	for _, m := range a.matchAt[row] {
		if m.side == side {
			ms = append(ms, m)
		}
	}
	cur := -1
	if a.mi >= 0 && a.mi < len(a.matches) {
		cur = a.mi
	}
	base := tcell.StyleDefault.Foreground(th.Fg)
	col, si, wi, mi := 0, 0, 0, 0
	trunc := false
	for bi, r := range l.Text {
		for si < len(segs) && bi >= segs[si].End {
			si++
		}
		st := base
		if si < len(segs) {
			st = th.tok(segs[si].Class)
		}
		for wi < len(l.Words) && bi >= l.Words[wi].B {
			wi++
		}
		cbg := bg
		if wi < len(l.Words) && bi >= l.Words[wi].A {
			cbg = wordBg
		}
		for mi < len(ms) && bi >= ms[mi].b {
			mi++
		}
		if mi < len(ms) && bi >= ms[mi].a {
			cbg, st = th.Match, st.Foreground(hex("#000000"))
			if ms[mi].idx == cur {
				cbg, st = th.MatchCur, st.Bold(true)
			}
		}
		w := 1
		switch {
		case r == '\t':
			w, r = tabW-col%tabW, ' '
		case r < ' ' || r == 0x7f:
			r = '·'
		default:
			w = runewidth.RuneWidth(r)
			if w == 0 {
				continue
			}
		}
		sx := x0 + col - a.hx
		col += w
		if sx+w > x1 {
			trunc = true
			break
		}
		if sx < x0 {
			continue
		}
		st = st.Background(cbg)
		a.s.SetContent(sx, y, r, nil, st)
		if r == ' ' {
			for k := 1; k < w; k++ {
				a.s.SetContent(sx+k, y, ' ', nil, st)
			}
		}
	}
	if trunc {
		a.set(x1-1, y, '›', th.st(th.Subtle, bg))
	}
	if a.hx > 0 && l.Text != "" {
		a.set(x0, y, '‹', th.st(th.Subtle, bg))
	}
}

func lineLabel(c *Card) string {
	if c.From == c.To {
		return "line " + strconv.Itoa(c.To)
	}
	return "lines " + strconv.Itoa(c.From) + "–" + strconv.Itoa(c.To)
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	}
	return strconv.Itoa(int(d.Hours())) + "h ago"
}

func (a *App) drawCard(r *Row, y, i int) {
	th := a.th
	c := r.Card
	cursor := i == a.cur && !a.focusTree
	a.fill(a.px, a.px+a.pw, y, th.st(th.Fg, th.Bg))
	if a.lay.Split {
		o0, o1 := a.lay.region(1 - c.Side)
		a.fill(a.px+o0, a.px+o1, y, th.st(th.Fg, mix(th.Bg, th.Panel, 0.55)))
		a.set(a.px+a.lay.half(), y, '│', th.st(th.Faint, th.Bg))
	}
	x0, x1 := a.px+c.x0, a.px+c.x1
	bar := [...]tcell.Color{CardReview: th.Amber, CardSent: th.Green, CardCompose: th.Accent}[c.Kind]
	bg := th.CardBg
	if cursor {
		bg = mix(bg, th.Accent, 0.10)
		a.set(a.px, y, '▎', th.st(th.Accent, th.Bg))
	}
	a.fill(x0, x1, y, th.st(th.Fg, bg))
	a.set(x0, y, '▎', th.st(bar, bg))
	tx, tx1 := x0+2, x1-1
	if c.Kind == CardCompose {
		a.drawCompose(c, r.Sub, y, tx, tx1, bg)
		return
	}
	head, info, action, body := "➜ sent", lineLabel(c)+" · "+ago(c.At), "dismiss", th.Muted
	if c.Kind == CardReview {
		head, info, action, body = "✎ review", "line "+strconv.Itoa(c.Comment.Open), "resolve", th.Fg
		if a.pending == "d" && cursor {
			action = "d again to resolve"
		}
	}
	if r.Sub > 0 {
		a.put(tx, y, tx1, c.wrap[r.Sub-1], th.st(body, bg))
		return
	}
	x := a.put(tx, y, tx1, head, th.st(bar, bg).Bold(true))
	x = a.put(x, y, tx1, " · "+info, th.st(th.Subtle, bg))
	a.buttonR(tx1, y, x+2, action, th.st(th.Muted, bg), th.st(th.Accent, bg), func(int) { a.resolve(c) })
}

// drawCompose draws row sub of the compose card: heading, editor lines,
// error lines, then the button row.
func (a *App) drawCompose(c *Card, sub, y, tx, tx1 int, bg tcell.Color) {
	th := a.th
	vl := c.Ed.wrap(tx1 - tx)
	nEd := max(len(vl), 2)
	nErr := 0
	if c.Err != "" {
		nErr = len(c.wrap)
	}
	switch {
	case sub == 0:
		side := "new"
		if c.Side == 0 {
			side = "old"
		}
		x := a.put(tx, y, tx1, "✎ comment on "+lineLabel(c), th.st(th.Accent, bg).Bold(true))
		x = a.put(x, y, tx1, " · "+side+" side", th.st(th.Subtle, bg))
		if c.Busy != "" {
			a.putR(tx1, y, x+2, c.Busy, th.st(th.Amber, bg))
		}
	case sub <= nEd:
		k := sub - 1
		ebg := th.Bg
		a.fill(tx-1, tx1, y, th.st(th.Fg, ebg))
		if k < len(vl) {
			v := vl[k]
			a.put(tx, y, tx1, string(c.Ed.lines[v.row][v.a:v.b]), th.st(th.Fg, ebg))
		}
		if k == 0 && c.Ed.Text() == "" {
			a.put(tx, y, tx1, "Leave a comment…", th.st(th.Subtle, ebg).Italic(true))
		}
		if cr, cc := c.Ed.cursorVis(vl); cr == k && a.compose == c && !a.help {
			a.s.ShowCursor(tx+cc, y)
			a.s.SetCursorStyle(tcell.CursorStyleSteadyBar)
		}
		a.hots = append(a.hots, hot{y, tx - 1, tx1, func(x int) {
			if k < len(vl) {
				c.Ed.setVis(vl[k], x-tx)
			} else if len(vl) > 0 {
				c.Ed.setVis(vl[len(vl)-1], 1<<20)
			}
		}})
	case sub <= nEd+nErr:
		a.put(tx, y, tx1, c.wrap[sub-nEd-1], th.st(th.Red, bg))
	default:
		x := tx
		if a.repo.CanSend() {
			x = a.button(x, y, tx1, " ⌃⏎ Send ", th.st(th.Bg, th.Accent).Bold(true), th.st(th.Bg, th.Fg).Bold(true), func(int) { a.submit(true) }) + 1
		}
		inline := th.st(th.Fg, th.Raised)
		if !a.repo.CommentsEnabled() || c.Side == 0 || c.File.Pseudo {
			inline = th.st(th.Subtle, th.Raised)
		}
		x = a.button(x, y, tx1, " ⌃⌥⏎ Comment in file ", inline, th.st(th.Bg, th.Fg), func(int) { a.submit(false) }) + 1
		x = a.button(x, y, tx1, " esc Cancel ", th.st(th.Muted, bg), th.st(th.Fg, th.Raised), func(int) { a.closeCompose(true) })
		a.putR(tx1, y, x+2, "⇧⏎ newline", th.st(th.Subtle, bg))
	}
}

// drawScrollbar draws the thumb and a minimap of changes, cards, and matches.
func (a *App) drawScrollbar() {
	th := a.th
	x := a.W - 1
	rows := a.lay.Rows
	n := len(rows)
	hot := a.drag == dragScroll || (a.hoverX == x && a.hoverY >= a.py && a.hoverY < a.py+a.ph)
	for y := range a.ph {
		r0, r1 := 0, 0
		if n > 0 {
			r0 = y * n / a.ph
			r1 = max((y+1)*n/a.ph, r0+1)
		}
		bg := th.Track
		if n > 0 && r0 < a.top+a.ph && r1 > a.top {
			bg = th.Thumb
			if hot {
				bg = mix(th.Thumb, th.Fg, 0.25)
			}
		}
		adds, dels, cards, found := 0, 0, false, false
		for i := r0; i < min(r1, n); i++ {
			row := &rows[i]
			switch row.Kind {
			case RowLine:
				for _, l := range [2]*Line{row.L, row.R} {
					if l != nil && l.Kind == Add {
						adds++
					} else if l != nil && l.Kind == Del {
						dels++
					}
				}
			case RowCard:
				cards = true
			}
			found = found || a.matchAt[i] != nil
		}
		ch, fg := ' ', th.Fg
		switch {
		case found:
			ch, fg = '▐', th.MatchCur
		case cards:
			ch, fg = '▐', th.Amber
		case adds > 0 && adds >= dels:
			ch, fg = '▐', th.Green
		case dels > 0:
			ch, fg = '▐', th.Red
		}
		a.set(x, a.py+y, ch, th.st(fg, bg))
	}
}

func (a *App) drawStatus() {
	th := a.th
	y, bg := a.H-1, th.Panel
	a.fill(0, a.W, y, th.st(th.Muted, bg))
	mode, mc := "NORMAL", th.Accent
	switch {
	case a.compose != nil:
		mode, mc = "COMPOSE", th.Green
	case a.searching:
		mode, mc = "SEARCH", th.Amber
	case a.sel >= 0:
		mode, mc = "VISUAL", th.Purple
	case a.focusTree:
		mode, mc = "FILES", th.Muted
	}
	x := a.put(0, y, a.W, " "+mode+" ", th.st(th.Bg, mc).Bold(true)) + 1
	if a.searching {
		x = a.put(x, y, a.W, "/", th.st(th.Amber, bg))
		x = a.put(x, y, a.W, a.query, th.st(th.Fg, bg))
		a.s.ShowCursor(x, y)
		a.s.SetCursorStyle(tcell.CursorStyleSteadyBar)
		x = a.put(x+2, y, a.W, a.matchInfo(), th.st(th.Subtle, bg))
	} else if a.lay != nil && a.cur < len(a.lay.Rows) {
		x = a.put(x, y, a.W/2, ellipsisLeft(a.curPos(), a.W/2-x), th.st(th.Muted, bg))
		if a.query != "" {
			x = a.put(x+2, y, a.W, "/"+a.query+" "+a.matchInfo(), th.st(th.Amber, bg))
		}
		if a.count > 0 || a.pending != "" {
			count := ""
			if a.count > 0 {
				count = strconv.Itoa(a.count)
			}
			x = a.put(x+2, y, a.W, count+a.pending, th.st(th.Fg, bg))
		}
	}
	rx := a.W - 1
	rx = a.buttonR(rx, y, x+2, "? help", th.st(th.Subtle, bg), th.st(th.Accent, bg), func(int) { a.help = true }) - 3
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		a.putR(rx, y, x+2, ellipsis(a.toast, max(rx-x-2, 0)), th.st(a.toastColor, bg))
	}
}

func (a *App) matchInfo() string {
	switch {
	case a.query == "":
		return ""
	case len(a.matches) == 0:
		return "no matches"
	}
	return fmt.Sprintf("%d/%d", a.mi+1, len(a.matches))
}

// drawChanged floats the files that changed on disk over the bottom right of
// the diff pane. Clicking it refreshes.
func (a *App) drawChanged() {
	th := a.th
	const most = 8
	head, files := "● diff changed · r to refresh", a.changed
	if len(files) > most {
		files = append(slices.Clip(files[:most-1]), fmt.Sprintf("and %d more", len(files)-most+1))
	}
	w := width(head)
	for _, f := range files {
		w = max(w, width(f)+2)
	}
	w, h := min(w+4, a.pw), len(files)+3
	x0, y0 := a.px+a.pw-w, a.py+a.ph-h
	ix, iy, ix1, _ := a.box(x0, y0, w, h, "")
	bg := th.Raised
	a.put(ix, iy, ix1, head, th.st(th.Amber, bg).Bold(true))
	for i, f := range files {
		a.put(ix+2, iy+1+i, ix1, ellipsisLeft(f, ix1-ix-2), th.st(th.Fg, bg))
	}
	for y := y0; y < y0+h; y++ {
		a.hots = append(a.hots, hot{y, x0, x0 + w, func(int) { a.refresh() }})
	}
}

// box draws a bordered panel and returns its inner rectangle.
func (a *App) box(x0, y0, w, h int, title string) (int, int, int, int) {
	th := a.th
	bg := th.Raised
	bst := th.st(th.Border, bg)
	for y := y0; y < y0+h; y++ {
		a.fill(x0, x0+w, y, th.st(th.Fg, bg))
		a.set(x0, y, '│', bst)
		a.set(x0+w-1, y, '│', bst)
	}
	for x := x0; x < x0+w; x++ {
		a.set(x, y0, '─', bst)
		a.set(x, y0+h-1, '─', bst)
	}
	a.set(x0, y0, '╭', bst)
	a.set(x0+w-1, y0, '╮', bst)
	a.set(x0, y0+h-1, '╰', bst)
	a.set(x0+w-1, y0+h-1, '╯', bst)
	if title != "" {
		a.put(x0+2, y0, x0+w-2, " "+title+" ", th.st(th.Fg, bg).Bold(true))
	}
	return x0 + 2, y0 + 1, x0 + w - 2, y0 + h - 1
}

type helpSection struct {
	title string
	keys  [][2]string
}

var helpSections = []helpSection{
	{"Move", [][2]string{
		{"j k  ↑ ↓", "line"},
		{"^d ^u", "half page"},
		{"^f ^b  space", "page"},
		{"gg  G", "top, bottom"},
		{"n p", "next, previous file"},
		{"] [", "next, previous change"},
		{"h l", "scroll sideways"},
		{"H L", "old, new side (split)"},
		{"/  n N", "search, next, previous"},
		{"tab", "file tree focus"},
	}},
	{"Review", [][2]string{
		{"c  ⏎", "comment on the line"},
		{"V", "select lines"},
		{"^⏎  ^s", "send to the session"},
		{"^⌥⏎  ^o", "comment in the file"},
		{"dd", "resolve, dismiss card"},
		{"v", "viewed"},
		{"x X", "expand context, file"},
		{"y", "copy path:line"},
	}},
	{"View", [][2]string{
		{"e E", "collapse file, all"},
		{"s", "split, unified"},
		{"t", "light, dark"},
		{"b", "file tree"},
		{"r", "refresh"},
		{"q", "quit"},
	}},
}

// helpHeight is the rows the sections take: a title, the keys, and a blank.
func helpHeight(ss []helpSection) int {
	n := 0
	for _, s := range ss {
		n += len(s.keys) + 2
	}
	return n
}

// drawHelp draws the key reference in two columns, or one on a narrow
// screen.
func (a *App) drawHelp() {
	th := a.th
	const colW = 40
	left, right := helpSections[:1], helpSections[1:]
	if a.W < 2*colW+6 {
		left, right = helpSections, nil
	}
	cols := 1 + b2i(right != nil)
	h := max(helpHeight(left), helpHeight(right)) + 3
	w := cols*colW + 4
	x0, y0 := max((a.W-w)/2, 0), max((a.H-h)/2, 0)
	ix, iy, _, _ := a.box(x0, y0, min(w, a.W), min(h, a.H), "Keys")
	bg := th.Raised
	draw := func(ss []helpSection, x int) {
		y := iy + 1
		for _, s := range ss {
			a.put(x, y, x+colW, s.title, th.st(th.Accent, bg).Bold(true))
			y++
			for _, k := range s.keys {
				a.put(x, y, x+colW, k[0], th.st(th.Fg, bg).Bold(true))
				a.put(x+15, y, x+colW, k[1], th.st(th.Muted, bg))
				y++
			}
			y++
		}
	}
	draw(left, ix)
	draw(right, ix+colW)
	a.put(ix, y0+h-2, x0+w-2, "mouse: click, drag the gutter to comment, wheel, ⇧wheel", th.st(th.Subtle, bg))
}
