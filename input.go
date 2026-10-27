package main

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

var keyNames = map[tcell.Key]string{
	tcell.KeyEnter: "enter", tcell.KeyTab: "tab", tcell.KeyBacktab: "tab", tcell.KeyEsc: "esc",
	tcell.KeyBackspace: "bs", tcell.KeyBackspace2: "bs", tcell.KeyDelete: "del",
	tcell.KeyUp: "up", tcell.KeyDown: "down", tcell.KeyLeft: "left", tcell.KeyRight: "right",
	tcell.KeyHome: "home", tcell.KeyEnd: "end", tcell.KeyPgUp: "pgup", tcell.KeyPgDn: "pgdn",
	tcell.KeyInsert: "ins",
}

// keyName spells a key the way the handlers match it: "j", "C-d", "M-b",
// "S-enter", "C-M-enter", "pgdn". Control letters never carry S-.
func keyName(ev *tcell.EventKey) string {
	k, m := ev.Key(), ev.Modifiers()
	ctrl, alt, shift := "", "", ""
	if m&tcell.ModCtrl != 0 {
		ctrl = "C-"
	}
	if m&tcell.ModAlt != 0 {
		alt = "M-"
	}
	if m&tcell.ModShift != 0 || k == tcell.KeyBacktab {
		shift = "S-"
	}
	switch {
	case k == tcell.KeyRune:
		return ctrl + alt + string(ev.Rune())
	case keyNames[k] != "":
		return ctrl + alt + shift + keyNames[k]
	case k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ:
		return "C-" + alt + string(rune('a'+k-tcell.KeyCtrlA))
	}
	return ctrl + alt + shift + ev.Name()
}

func (a *App) key(ev *tcell.EventKey) {
	k := keyName(ev)
	switch {
	case a.help:
		a.help = false
	case a.compose != nil:
		a.composeKey(k, ev)
	case a.searching:
		a.searchKey(k, ev)
	case a.pasting:
	case k == "C-c" || (a.d == nil && k == "q"):
		a.quit = true
	case a.lay != nil:
		a.normalKey(k)
	}
}

// plainRune returns the key's rune when it was typed without ctrl or alt.
func plainRune(ev *tcell.EventKey) (rune, bool) {
	if ev.Key() != tcell.KeyRune || ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
		return 0, false
	}
	return ev.Rune(), true
}

// curPos is the cursor's file and line as path:line, favoring the active side.
func (a *App) curPos() string {
	r := &a.lay.Rows[a.cur]
	for _, side := range [2]int{a.side, 1 - a.side} {
		if l := r.lineOn(side, a.lay.Split); l != nil {
			return r.F.Path + ":" + strconv.Itoa(l.num(side))
		}
	}
	return r.F.Path
}

func (a *App) normalKey(k string) {
	if len(k) == 1 && (k[0] >= '1' && k[0] <= '9' || k[0] == '0' && a.count > 0) {
		a.count = a.count*10 + int(k[0]-'0')
		return
	}
	n := max(a.count, 1)
	a.count = 0
	pend := a.pending
	a.pending = ""
	rows := a.lay.Rows
	if len(rows) == 0 {
		if k == "q" {
			a.quit = true
		}
		return
	}
	row := &rows[a.cur]
	onCard := row.Kind == RowCard && row.Card.Kind != CardCompose
	switch {
	case pend == "g" && k == "g":
		a.jump(0, 0)
		return
	case pend == "d" && k == "d":
		if onCard {
			a.resolve(row.Card)
		}
		return
	}
	if a.focusTree && a.treeKey(k) {
		return
	}
	switch k {
	case "q":
		a.quit = true
	case "esc":
		a.sel, a.query, a.matches, a.matchAt = -1, "", nil, nil
		a.focusTree = false
	case "j", "down":
		a.move(n)
	case "k", "up":
		a.move(-n)
	case "C-d":
		a.page(a.ph / 2)
	case "C-u":
		a.page(-a.ph / 2)
	case "C-f", "pgdn", " ":
		a.page(a.ph - 2)
	case "C-b", "pgup":
		a.page(-(a.ph - 2))
	case "C-e":
		a.scroll(n)
	case "C-y":
		a.scroll(-n)
	case "g":
		a.pending = "g"
	case "G", "end":
		a.jump(len(rows)-1, a.maxTop())
	case "home":
		a.jump(0, 0)
	case "n", "N":
		if k == "N" {
			n = -n
		}
		if len(a.matches) > 0 {
			a.nextMatch(n)
		} else {
			a.nextFile(n)
		}
	case "p":
		a.nextFile(-n)
	case "]", "J", "}":
		a.nextChange(1)
	case "[", "K", "{":
		a.nextChange(-1)
	case "h", "left":
		a.hx = max(a.hx-8*n, 0)
	case "l", "right":
		a.hx += 8 * n
	case "0":
		a.hx = 0
	case "H":
		a.side = 0
	case "L":
		a.side = 1
	case "v":
		if f := a.curFile(); f != nil {
			a.toggleViewed(f)
		}
	case "e":
		if f := a.curFile(); f != nil {
			a.toggleCollapse(f)
		}
	case "E":
		a.collapseAll()
	case "s":
		a.toggleSplit()
	case "t":
		a.toggleTheme()
	case "b":
		a.showTree, a.focusTree = !a.showTree, false
		a.geom()
	case "/":
		a.searching, a.query = true, ""
		a.searchFrom = [2]int{a.top, a.cur}
		a.search(false)
	case "c":
		a.openCompose(a.commentSide())
	case "enter", "o":
		switch row.Kind {
		case RowFile:
			a.toggleCollapse(row.F)
		case RowGap:
			a.expandAround(a.cur)
		case RowLine:
			a.openCompose(a.commentSide())
		}
	case "V":
		if a.sel >= 0 {
			a.sel = -1
		} else if row.Kind == RowLine {
			a.sel = a.cur
		}
	case "x":
		a.expandAround(a.cur)
	case "X":
		a.expandFile(row.F)
	case "d":
		if onCard {
			a.pending = "d"
		}
	case "y":
		a.yank(a.curPos())
	case "r":
		a.load()
		a.flash("refreshing…", a.th.Muted)
	case "tab", "S-tab":
		if a.tw > 0 {
			a.focusTree = !a.focusTree
			a.treeFollowForce()
		}
	case "?":
		a.help = true
	}
}

func (a *App) move(n int) {
	if a.anim.on {
		a.top, a.anim.on = a.anim.to, false
	}
	a.cur = clamp(a.cur+n, 0, len(a.lay.Rows)-1)
	a.follow()
}

// page scrolls by n rows and moves the cursor along with the view.
func (a *App) page(n int) {
	top := a.top
	if a.anim.on {
		top = a.anim.to
	}
	target := clamp(top+n, 0, a.maxTop())
	so := a.scrolloff()
	lo := target + 1 + so
	if target == 0 {
		lo = 0
	}
	a.cur = clamp(clamp(a.cur+n, lo, target+a.ph-1-so), 0, len(a.lay.Rows)-1)
	a.glide(target)
}

func (a *App) toggleSplit() {
	a.unified = !a.unified
	a.relayout()
	mode := "split"
	switch {
	case !a.unified && !a.lay.Split:
		mode = "split needs a wider pane (150 columns), zoom out"
	case !a.lay.Split:
		mode = "unified"
	}
	a.flash(mode, a.th.Muted)
}

func (a *App) toggleTheme() {
	if a.th.Dark {
		a.th = lightTheme()
	} else {
		a.th = darkTheme()
	}
}

// treeSelect highlights f in the tree and reports whether it is listed.
func (a *App) treeSelect(f *File) bool {
	i := slices.IndexFunc(a.trows, func(r TreeRow) bool { return r.N.File == f })
	if i >= 0 {
		a.treeCur = i
	}
	return i >= 0
}

// treeFollowForce puts the tree's highlight on the cursor's file.
func (a *App) treeFollowForce() {
	a.treeSelect(a.lay.Rows[a.cur].F)
	a.revealTree()
}

// treeFollow keeps the tree's highlight on the cursor's file while the
// tree isn't focused.
func (a *App) treeFollow() {
	if a.focusTree || a.lay == nil || a.cur >= len(a.lay.Rows) {
		return
	}
	f := a.lay.Rows[a.cur].F
	if a.treeCur < len(a.trows) && a.trows[a.treeCur].N.File == f {
		return
	}
	if a.treeSelect(f) {
		a.revealTree()
	}
}

func (a *App) revealTree() {
	vis := a.ph - 1
	if a.treeCur < a.treeTop {
		a.treeTop = a.treeCur
	} else if a.treeCur >= a.treeTop+vis {
		a.treeTop = a.treeCur - vis + 1
	}
}

func (a *App) toggleDir(n *TreeNode) {
	n.Closed = !n.Closed
	a.trows = flatten(a.tree)
}

func (a *App) treeKey(k string) bool {
	if len(a.trows) == 0 {
		return false
	}
	a.treeCur = clamp(a.treeCur, 0, len(a.trows)-1)
	n := a.trows[a.treeCur]
	switch k {
	case "j", "down":
		a.treeCur = min(a.treeCur+1, len(a.trows)-1)
	case "k", "up":
		a.treeCur = max(a.treeCur-1, 0)
	case "g", "home":
		a.treeCur = 0
	case "G", "end":
		a.treeCur = len(a.trows) - 1
	case "enter", "l", "o", " ", "right":
		if n.N.Dir {
			a.toggleDir(n.N)
		} else {
			a.gotoFile(n.N.File)
			a.focusTree = k != "enter"
		}
	case "h", "left":
		if n.N.Dir && !n.N.Closed {
			a.toggleDir(n.N)
			break
		}
		for i := a.treeCur - 1; i >= 0; i-- {
			if a.trows[i].Depth < n.Depth {
				a.treeCur = i
				break
			}
		}
	case "esc":
		a.focusTree = false
	default:
		return false
	}
	a.revealTree()
	return true
}

func (a *App) composeKey(k string, ev *tcell.EventKey) {
	c := a.compose
	before := len(c.Ed.wrap(c.Ed.width))
	switch k {
	case "C-enter", "C-s":
		a.submit(a.repo.CanSend())
		return
	case "C-M-enter", "M-C-enter", "C-o":
		a.submit(false)
		return
	case "esc", "C-c":
		a.closeCompose(true)
		return
	case "enter":
		c.Ed.Newline()
	case "tab":
		c.Ed.InsertText("    ")
	default:
		if r, ok := plainRune(ev); ok {
			c.Ed.Insert(r)
		} else if !c.Ed.Key(k) {
			return
		}
	}
	c.Err = ""
	if len(c.Ed.wrap(c.Ed.width)) != before || len(c.wrap) > 0 {
		a.relayout()
		a.revealCompose()
	}
}

func (a *App) searchKey(k string, ev *tcell.EventKey) {
	switch k {
	case "enter":
		a.searching = false
		if len(a.matches) == 0 {
			a.query = ""
		}
		return
	case "esc", "C-c":
		a.searching, a.query = false, ""
		a.top, a.cur = a.searchFrom[0], a.searchFrom[1]
		a.search(false)
		return
	case "bs":
		if a.query == "" {
			a.searching = false
			return
		}
		a.query = trimLastRune(a.query)
	case "C-w", "C-u", "M-bs":
		a.query = ""
	default:
		r, ok := plainRune(ev)
		if !ok {
			return
		}
		a.query += string(r)
	}
	a.top, a.cur = a.searchFrom[0], a.searchFrom[1]
	a.search(true)
}

func trimLastRune(s string) string {
	_, sz := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-sz]
}

// asciiLower folds only ASCII, so byte offsets into the result still index
// the original text.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// search finds the query in the shown lines, smart-case. With jump it moves
// to the first match at or after where the search started.
func (a *App) search(jump bool) {
	a.matches, a.matchAt = nil, nil
	if a.query == "" || a.lay == nil {
		return
	}
	q := a.query
	fold := asciiLower(q) == q
	a.matchAt = map[int][]match{}
	find := func(i int, l *Line, side int) []match {
		t := l.Text
		if fold {
			t = asciiLower(t)
		}
		var ms []match
		for off := 0; off <= len(t); {
			j := strings.Index(t[off:], q)
			if j < 0 {
				break
			}
			ms = append(ms, match{row: i, side: side, a: off + j, b: off + j + len(q), idx: len(a.matches) + len(ms)})
			off += j + len(q)
		}
		a.matches = append(a.matches, ms...)
		return ms
	}
	for i := range a.lay.Rows {
		r := &a.lay.Rows[i]
		if r.Kind != RowLine {
			continue
		}
		var ms []match
		switch {
		case !a.lay.Split:
			ms = find(i, r.L, b2i(r.L.Kind != Del))
		case r.L == r.R:
			// A context line is one match, highlighted on both sides.
			ms = find(i, r.L, 0)
			for _, m := range ms {
				m.side = 1
				ms = append(ms, m)
			}
		default:
			if r.L != nil {
				ms = find(i, r.L, 0)
			}
			if r.R != nil {
				ms = append(ms, find(i, r.R, 1)...)
			}
		}
		if len(ms) > 0 {
			a.matchAt[i] = ms
		}
	}
	if !jump {
		a.mi = clamp(a.mi, 0, max(len(a.matches)-1, 0))
		return
	}
	a.mi = 0
	for k, m := range a.matches {
		if m.row >= a.searchFrom[1] {
			a.mi = k
			break
		}
	}
	a.showMatch()
}

func (a *App) nextMatch(d int) {
	if n := len(a.matches); n > 0 {
		a.mi = ((a.mi+d)%n + n) % n
		a.showMatch()
	}
}

// showMatch moves to the current match and scrolls sideways to show it.
func (a *App) showMatch() {
	if len(a.matches) == 0 {
		return
	}
	m := a.matches[a.mi]
	if a.lay.Split {
		a.side = m.side
	}
	a.jump(m.row, -1)
	r := &a.lay.Rows[m.row]
	l := r.lineOn(m.side, a.lay.Split)
	if l == nil {
		return
	}
	col := 0
	for _, ch := range l.Text[:m.a] {
		if ch == '\t' {
			col += tabW - col%tabW
		} else {
			col++
		}
	}
	x0, x1 := a.lay.region(m.side)
	room := x1 - a.lay.codeX(r.F, m.side) - 4
	if room < 0 {
		room = x1 - x0
	}
	if col < a.hx || col+m.b-m.a > a.hx+room {
		a.hx = max(col-room/3, 0)
	}
}

func (a *App) mouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	b := ev.Buttons()
	if b&(tcell.WheelUp|tcell.WheelDown|tcell.WheelLeft|tcell.WheelRight) != 0 {
		a.wheel(x, y, b, ev.Modifiers())
		return
	}
	a.hoverX, a.hoverY = x, y
	a.hoverRow = -1
	if a.lay != nil && x >= a.px && x < a.px+a.pw && y > a.py && y < a.py+a.ph && !a.help {
		if i := a.top + y - a.py; i < len(a.lay.Rows) && a.lay.Rows[i].Kind == RowLine {
			a.hoverRow, a.hoverSide = i, a.sideAt(x, &a.lay.Rows[i])
		}
	}
	down := b&tcell.Button1 != 0
	switch {
	case down && !a.btnDown:
		a.btnDown = true
		a.press(x, y)
	case down:
		a.dragTo(x, y)
	case a.btnDown:
		a.btnDown = false
		a.release()
	}
}

// sideAt is the side of a line row under column x.
func (a *App) sideAt(x int, r *Row) int {
	if a.lay.Split {
		return b2i(x >= a.px+a.lay.half())
	}
	return b2i(r.L == nil || r.L.Kind != Del)
}

func (a *App) wheel(x, y int, b tcell.ButtonMask, mod tcell.ModMask) {
	dy, dx := 0, 0
	switch {
	case b&tcell.WheelUp != 0:
		dy = -1
	case b&tcell.WheelDown != 0:
		dy = 1
	case b&tcell.WheelLeft != 0:
		dx = -4
	case b&tcell.WheelRight != 0:
		dx = 4
	}
	if mod&tcell.ModShift != 0 {
		dx, dy = dy*4, 0
	}
	switch {
	case a.help:
	case a.tw > 0 && x < a.tw:
		a.treeTop = max(a.treeTop+dy, 0)
	case dx != 0:
		a.hx = max(a.hx+dx, 0)
	default:
		a.scroll(dy)
	}
}

func (a *App) press(x, y int) {
	now := time.Now()
	double := now.Sub(a.lastClick) < 400*time.Millisecond && a.lastClickXY == [2]int{x, y}
	a.lastClick, a.lastClickXY = now, [2]int{x, y}
	if a.help {
		a.help = false
		return
	}
	for _, h := range a.hots {
		if h.y == y && x >= h.x0 && x < h.x1 {
			h.fn(x)
			return
		}
	}
	if a.lay == nil || y < a.py || y >= a.py+a.ph {
		return
	}
	switch {
	case a.tw > 0 && x == a.tw:
		a.drag = dragDivider
		return
	case x == a.W-1:
		a.drag = dragScroll
		a.scrollTo(y)
		return
	case a.tw > 0 && x < a.tw:
		a.treeClick(y)
		return
	}
	rows := a.lay.Rows
	if y == a.py && a.top < len(rows) && rows[a.top].Kind != RowFile {
		a.jumpFile(rows[a.top].F)
		return
	}
	i := a.top + y - a.py
	if i >= len(rows) {
		return
	}
	a.focusTree = false
	a.anim.on = false
	a.cur = i
	r := &rows[i]
	switch r.Kind {
	case RowFile:
		a.toggleCollapse(r.F)
	case RowGap:
		if double {
			a.expandAround(i)
		}
	case RowLine:
		a.side = a.sideAt(x, r)
		a.dragAnchor = i
		if x < a.px+a.lay.codeX(r.F, a.side) {
			a.drag, a.sel = dragGutter, i
			return
		}
		a.sel = -1
		if double {
			a.openCompose(a.commentSide())
			return
		}
		a.drag = dragSel
	}
}

// scrollTo jumps the view to the scrollbar position y.
func (a *App) scrollTo(y int) {
	n := len(a.lay.Rows)
	a.top = (y-a.py)*n/max(a.ph, 1) - a.ph/2
	a.anim.on = false
	a.clampTop()
	a.scroll(0)
}

func (a *App) treeClick(y int) {
	i := a.treeTop + y - a.py - 1
	if y == a.py || i < 0 || i >= len(a.trows) {
		return
	}
	a.treeCur = i
	if n := a.trows[i].N; n.Dir {
		a.toggleDir(n)
	} else {
		a.gotoFile(n.File)
	}
}

func (a *App) dragTo(x, y int) {
	switch a.drag {
	case dragDivider:
		a.treeW = x
		a.geom()
	case dragScroll:
		a.scrollTo(y)
	case dragSel, dragGutter:
		if y <= a.py {
			a.scroll(-1)
		} else if y >= a.py+a.ph-1 {
			a.scroll(1)
		}
		i := clamp(a.top+clamp(y, a.py+1, a.py+a.ph-1)-a.py, 0, len(a.lay.Rows)-1)
		if a.lay.Rows[i].F != a.lay.Rows[a.dragAnchor].F {
			return
		}
		a.cur, a.sel = i, a.dragAnchor
		if a.drag == dragSel && i == a.dragAnchor {
			a.sel = -1
		}
	}
}

func (a *App) release() {
	if a.drag == dragGutter {
		a.openCompose(a.side)
	}
	a.drag = dragNone
}
