package main

import (
	"math"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/msuozzo/nit/review"
)

// evFn is a function posted to the event loop to run on the main goroutine.
type evFn struct {
	t  time.Time
	fn func()
}

func (e *evFn) When() time.Time { return e.t }

// hot is a clickable span of one screen row.
type hot struct {
	y, x0, x1 int
	fn        func(x int)
}

// anim glides the top row from one position to another.
type anim struct {
	on       bool
	from, to int
	t0       time.Time
	dur      time.Duration
}

const (
	dragNone = iota
	dragSel
	dragGutter
	dragDivider
	dragScroll
)

const tabW = 4

// App holds all UI state. It is mutated only on the main goroutine, and
// background work hands results back through post.
type App struct {
	s    tcell.Screen
	repo *review.Repo
	th   *Theme

	d      *review.Snapshot
	files  []*File // in tree order
	byPath map[string]*File
	tree   *TreeNode
	trows  []TreeRow
	lay    *Layout
	err    string

	top, cur  int
	side      int // the side the cursor is on in split mode
	hx        int // horizontal scroll
	sel       int // visual anchor row, -1 when off
	anim      anim
	ticking   bool
	unified   bool
	layW      int  // pane width the layout was built for
	laySplit  bool // split mode the layout was built for
	prefetchQ map[*File]bool

	showTree  bool
	treeW     int
	treeTop   int
	treeCur   int
	focusTree bool

	compose *Card
	drafts  map[string]string // unsent compose text by draftKey
	pasting bool

	searching  bool
	query      string
	matches    []match
	matchAt    map[int][]match
	mi         int
	searchFrom [2]int // top and cursor when the search began

	help bool

	toast      string
	toastColor tcell.Color
	toastUntil time.Time
	stale      bool
	changed    []string // files that differ on disk, while stale
	pending    string   // first key of a two-key command
	count      int      // count prefix being typed

	W, H           int
	px, py, pw, ph int // diff pane
	tw             int // tree width, 0 when hidden
	hots           []hot

	hoverRow, hoverSide, hoverX, hoverY int
	drag, dragAnchor                    int
	btnDown                             bool
	lastClick                           time.Time
	lastClickXY                         [2]int
	wheelH, wheelV                      float64   // recent sideways and vertical wheel ticks
	wheelAt                             time.Time // when they were last counted

	sem     chan struct{}               // bounds concurrent repo reads
	polled  atomic.Pointer[review.Repo] // repo of the diff on screen
	focused atomic.Bool
	pollNow chan struct{}
	quit    bool
}

type match struct {
	row, side, a, b, idx int
}

func newApp(s tcell.Screen, repo *review.Repo, th *Theme) *App {
	return &App{
		s: s, repo: repo, th: th, sel: -1, side: 1, showTree: true, treeW: 34,
		drafts: map[string]string{}, byPath: map[string]*File{}, hoverRow: -1,
		sem: make(chan struct{}, 3), prefetchQ: map[*File]bool{}, pollNow: make(chan struct{}, 1),
	}
}

func (a *App) post(fn func()) { a.s.PostEventWait(&evFn{time.Now(), fn}) }

func (a *App) after(d time.Duration, fn func()) { time.AfterFunc(d, func() { a.post(fn) }) }

func (a *App) run() {
	a.s.EnableMouse(tcell.MouseMotionEvents)
	a.s.EnablePaste()
	a.s.EnableFocus()
	a.focused.Store(true)
	a.s.SetTitle("nit")
	a.geom()
	a.load()
	go a.poll()
	last := time.Time{}
	for !a.quit {
		ev := a.s.PollEvent()
		if ev == nil {
			return
		}
		a.handle(ev)
		if a.quit {
			return
		}
		if !a.s.HasPendingEvent() || time.Since(last) > 30*time.Millisecond {
			a.draw()
			a.prefetch()
			last = time.Now()
		}
	}
}

func (a *App) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *evFn:
		ev.fn()
	case *tcell.EventResize:
		a.geom()
		a.s.Sync()
	case *tcell.EventKey:
		a.key(ev)
	case *tcell.EventMouse:
		a.mouse(ev)
	case *tcell.EventPaste:
		a.pasting = ev.Start()
	case *tcell.EventFocus:
		a.focused.Store(ev.Focused)
		if ev.Focused {
			select {
			case a.pollNow <- struct{}{}:
			default:
			}
		}
	}
}

func (a *App) load() {
	r := a.repo
	go func() {
		d, err := r.Load()
		a.post(func() {
			if a.repo == r {
				a.applyDiff(d, err)
			}
		})
	}()
}

// poll checks the diff every few seconds while the window has focus, and as
// soon as it regains focus, like the web page.
func (a *App) poll() {
	t := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-t.C:
		case <-a.pollNow:
		}
		if a.focused.Load() {
			a.check()
		}
	}
}

// check notes whether the diff on screen changed on disk, and in which files.
func (a *App) check() {
	r := a.polled.Load()
	if r == nil {
		return
	}
	if h, files, err := r.Peek(); err == nil {
		a.post(func() {
			if a.repo == r {
				a.stale, a.changed = h != a.d.PatchHash, changedFiles(a.d.FileHashes, files)
			}
		})
	}
}

func changedFiles(old, new map[string]string) []string {
	var out []string
	for f, h := range new {
		if old[f] != h {
			out = append(out, f)
		}
	}
	for f := range old {
		if _, ok := new[f]; !ok {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out
}

// applyDiff installs a fresh snapshot, carrying over loaded contents, sent
// cards, collapse state, and the view position from the files it replaces.
func (a *App) applyDiff(d *review.Snapshot, err error) {
	if err != nil {
		if a.d == nil {
			a.err = err.Error()
		}
		a.flash("✗ "+err.Error(), a.th.Red)
		return
	}
	var topK, curK rowKey
	if a.lay != nil {
		topK, curK = a.lay.key(a.top), a.lay.key(a.cur)
	}
	files := parsePatch(d.Patch)
	byPath := map[string]*File{}
	for _, f := range files {
		f.Hash = d.FileHashes[f.Path]
		f.init(d.Syntaxes[f.Path])
		f.Pseudo = d.Pseudo[f.Path]
		_, f.Viewed = d.Viewed[f.Path]
		f.Collapsed = f.Viewed
		if o := a.byPath[f.Path]; o != nil {
			f.Collapsed = o.Collapsed && f.Viewed == o.Viewed || f.Viewed && !o.Viewed
			f.Sent = o.Sent
			for _, c := range f.Sent {
				c.File = f
			}
			if o.content == contentLoaded && o.Hash == f.Hash && o.OldBlob == f.OldBlob && len(o.gaps) == len(f.gaps) {
				f.content, f.Old, f.New, f.hl, f.hlFull = o.content, o.Old, o.New, o.hl, o.hlFull
				copy(f.gaps, o.gaps)
			}
		}
		byPath[f.Path] = f
	}
	a.tree = buildTree(files)
	a.files = a.tree.files(nil)
	for i, f := range a.files {
		f.idx = i
	}
	a.byPath, a.d, a.err, a.stale, a.changed = byPath, d, "", false, nil
	if a.toast == "refreshing…" {
		a.toast = ""
	}
	a.polled.Store(a.repo)
	a.prefetchQ = map[*File]bool{}
	a.trows = flatten(a.tree)
	if a.compose != nil {
		a.compose.File = byPath[a.compose.File.Path]
		if a.compose.File == nil {
			a.compose = nil
		}
	}
	a.layW = -1
	a.relayout()
	if topK.path != "" {
		a.top, a.cur = a.lay.find(topK, byPath), a.lay.find(curK, byPath)
		a.clampTop()
	}
	for _, f := range a.files {
		if !f.hlFull {
			a.highlight(f)
		}
	}
}

// openDiff shows d, loaded from r, in place of the current diff.
func (a *App) openDiff(r *review.Repo, d *review.Snapshot) {
	a.closeCompose(true)
	a.repo, a.d, a.lay, a.err, a.stale, a.changed, a.anim.on = r, nil, nil, "", false, nil, false
	a.files, a.byPath, a.tree, a.trows = nil, map[string]*File{}, nil, nil
	a.top, a.cur, a.hx, a.sel, a.treeTop, a.treeCur = 0, 0, 0, -1, 0, 0
	a.searching, a.matches, a.matchAt = false, nil, nil
	a.polled.Store(nil)
	clear(a.drafts) // drafts belong to the diff they were typed on
	a.applyDiff(d, nil)
}

func (a *App) refresh() {
	a.load()
	a.flash("refreshing…", a.th.Muted)
}

// highlight colors the hunk lines of a file until its full contents load.
func (a *App) highlight(f *File) {
	go func() {
		a.sem <- struct{}{}
		hl := fragmentHL(f)
		<-a.sem
		a.post(func() {
			if !f.hlFull {
				f.hl = hl
			}
		})
	}()
}

// ensureContents loads a file's full contents, then runs then and rebuilds
// the layout. A nil then prefetches quietly.
func (a *App) ensureContents(f *File, then func()) {
	if a.d == nil || !a.repo.Expandable() {
		if then != nil {
			a.flash("this diff can't be expanded", a.th.Amber)
		}
		return
	}
	switch f.content {
	case contentLoaded:
		if then != nil {
			then()
			a.relayout()
		}
		return
	case contentFailed:
		return
	}
	if then != nil {
		f.pending = append(f.pending, then)
	}
	if f.content == contentLoading {
		return
	}
	f.content = contentLoading
	r := a.repo
	go func() {
		a.sem <- struct{}{}
		old, new, err := r.Contents(f.Path, f.OldPath)
		var hl [2]map[int][]Seg
		if err == nil {
			hl = fullHL(f, splitLines(old), splitLines(new))
		}
		<-a.sem
		a.post(func() {
			if err != nil {
				f.content, f.pending = contentFailed, nil
				a.flash("✗ "+f.Path+": "+err.Error(), a.th.Red)
				return
			}
			f.setContents(old, new)
			f.hl, f.hlFull = hl, true
			a.relayout()
		})
	}()
}

// prefetch loads contents for the files on and near the screen, so
// expansion is instant and highlighting sees whole files.
func (a *App) prefetch() {
	if a.lay == nil || a.d == nil || !a.repo.Expandable() {
		return
	}
	for i := max(a.top-a.ph, 0); i < min(a.top+2*a.ph, len(a.lay.Rows)); i++ {
		f := a.lay.Rows[i].F
		if f.content == contentNone && !f.Collapsed && !f.Binary && !a.prefetchQ[f] {
			a.prefetchQ[f] = true
			a.ensureContents(f, nil)
		}
	}
}

func (a *App) split() bool { return !a.unified && a.pw >= 150 }

// relayout rebuilds the rows, keeping the top row and the cursor on the
// same content.
func (a *App) relayout() {
	if a.d == nil {
		return
	}
	var topK, curK, toK rowKey
	if a.lay != nil {
		topK, curK, toK = a.lay.key(a.top), a.lay.key(a.cur), a.lay.key(a.anim.to)
	}
	a.layW, a.laySplit = a.pw, a.split()
	a.lay = buildLayout(a.files, a.laySplit, a.pw, a.compose)
	if topK.path != "" {
		a.top, a.cur = a.lay.find(topK, a.byPath), a.lay.find(curK, a.byPath)
		if a.anim.on {
			a.anim.from, a.anim.to = a.top, clamp(a.lay.find(toK, a.byPath), 0, a.maxTop())
		}
	}
	a.clampTop()
	a.cur = clamp(a.cur, 0, len(a.lay.Rows)-1)
	a.search(false)
}

// geom lays out the screen: title row, optional tree, diff pane with a
// scrollbar column, status row.
func (a *App) geom() {
	a.W, a.H = a.s.Size()
	a.treeW = clamp(a.treeW, 16, max(16, a.W-40))
	a.tw = 0
	if a.showTree && a.W-a.treeW >= 90 {
		a.tw = a.treeW
	}
	a.px, a.py = 0, 1
	if a.tw > 0 {
		a.px = a.tw + 1
	}
	a.pw, a.ph = max(a.W-a.px-1, 10), max(a.H-2, 1)
	if a.lay != nil && (a.pw != a.layW || a.split() != a.laySplit) {
		a.relayout()
	}
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}

func (a *App) maxTop() int {
	if a.lay == nil {
		return 0
	}
	return max(len(a.lay.Rows)-a.ph+a.ph/3, 0)
}

func (a *App) clampTop() { a.top = clamp(a.top, 0, a.maxTop()) }

func (a *App) clampCur() { a.cur = clamp(a.cur, 0, len(a.lay.Rows)-1) }

// maxHX keeps the end of the widest line on screen.
func (a *App) maxHX() int {
	room := a.pw - 12
	if a.lay.Split {
		room = a.pw/2 - 12
	}
	return max(a.lay.textW-room, 0)
}

func (a *App) scrolloff() int { return min(3, a.ph/5) }

// inView reports whether row i shows below the sticky file header.
func (a *App) inView(i int) bool {
	if i == a.top && a.lay.Rows[i].Kind == RowFile {
		return true
	}
	so := a.scrolloff()
	return i >= a.top+1+so && i <= a.top+a.ph-1-so
}

// follow scrolls just enough to bring the cursor into view.
func (a *App) follow() {
	if a.lay == nil || a.inView(a.cur) {
		return
	}
	so := a.scrolloff()
	switch {
	case a.cur < a.top+1+so && a.lay.Rows[a.cur].Kind == RowFile:
		a.top = a.cur
	case a.cur < a.top+1+so:
		a.top = a.cur - 1 - so
	default:
		a.top = a.cur - a.ph + 1 + so
	}
	a.anim.on = false
	a.clampTop()
}

// jump moves the cursor to row i and glides the view there, a third of
// the way down, unless it's already comfortably in view. topAt pins the
// top row instead when it is non-negative.
func (a *App) jump(i int, topAt int) {
	if a.lay == nil || len(a.lay.Rows) == 0 {
		return
	}
	a.cur = clamp(i, 0, len(a.lay.Rows)-1)
	if topAt < 0 {
		if a.inView(a.cur) {
			return
		}
		topAt = a.cur - a.ph/3
	}
	a.glide(topAt)
}

// jumpFile puts a file's header at the top of the pane.
func (a *App) jumpFile(f *File) { a.jump(a.lay.fileAt[f], a.lay.fileAt[f]) }

// glide animates the top row to a target. Long distances skip ahead so the
// glide covers at most two pages.
func (a *App) glide(to int) {
	to = clamp(to, 0, a.maxTop())
	d := to - a.top
	if d == 0 {
		a.anim.on = false
		return
	}
	if d > 2*a.ph {
		a.top = to - 2*a.ph
	} else if d < -2*a.ph {
		a.top = to + 2*a.ph
	}
	dist := math.Abs(float64(to - a.top))
	dur := time.Duration(70+28*math.Log2(1+dist/4)) * time.Millisecond
	a.anim = anim{on: true, from: a.top, to: to, t0: time.Now(), dur: min(dur, 220*time.Millisecond)}
	a.tick()
}

func (a *App) tick() {
	if a.ticking {
		return
	}
	a.ticking = true
	a.after(7*time.Millisecond, func() {
		a.ticking = false
		a.step()
	})
}

func (a *App) step() {
	if !a.anim.on {
		return
	}
	t := float64(time.Since(a.anim.t0)) / float64(a.anim.dur)
	if t >= 1 {
		a.top, a.anim.on = a.anim.to, false
		return
	}
	e := 1 - math.Pow(1-t, 3)
	a.top = a.anim.from + int(math.Round(float64(a.anim.to-a.anim.from)*e))
	a.tick()
}

// scroll moves the view by n rows and drags the cursor along when it
// would leave the screen.
func (a *App) scroll(n int) {
	if a.lay == nil {
		return
	}
	a.anim.on = false
	a.top += n
	a.clampTop()
	so := a.scrolloff()
	if a.cur < a.top+1+so {
		a.cur = min(a.top+1+so, len(a.lay.Rows)-1)
	} else if a.cur > a.top+a.ph-1-so {
		a.cur = max(a.top+a.ph-1-so, 0)
	}
	a.clampCur()
}

// flash shows a toast for four seconds, then wakes the loop to clear it.
func (a *App) flash(msg string, c tcell.Color) {
	a.toast, a.toastColor, a.toastUntil = msg, c, time.Now().Add(4*time.Second)
	a.after(4100*time.Millisecond, func() {})
}

func (a *App) curFile() *File {
	if a.lay == nil || len(a.lay.Rows) == 0 {
		return nil
	}
	if a.focusTree && a.treeCur < len(a.trows) && a.trows[a.treeCur].N.File != nil {
		return a.trows[a.treeCur].N.File
	}
	return a.lay.Rows[clamp(a.cur, 0, len(a.lay.Rows)-1)].F
}

// toggleViewed flips a file's viewed state, collapsing it when viewed and
// moving on to the next file if the cursor was inside it.
func (a *App) toggleViewed(f *File) {
	f.Viewed = !f.Viewed
	f.Collapsed = f.Viewed
	viewed := f.Viewed
	inFile := a.lay.Rows[a.cur].F == f
	a.relayout()
	if viewed && inFile {
		a.jumpFile(a.files[min(f.idx+1, len(a.files)-1)])
	}
	r := a.repo
	go func() {
		if err := r.SetViewed(f.Path, f.Hash, viewed); err != nil {
			a.post(func() { a.flash("✗ viewed: "+err.Error(), a.th.Red) })
		}
	}()
}

func (a *App) toggleCollapse(f *File) {
	f.Collapsed = !f.Collapsed
	in := a.lay.Rows[a.cur].F == f
	a.relayout()
	if in && f.Collapsed {
		a.cur = a.lay.fileAt[f]
		a.follow()
	}
}

// collapseAll collapses every file, or expands every file when all are
// collapsed, keeping the cursor's file at the top.
func (a *App) collapseAll() {
	expanded := slices.ContainsFunc(a.files, func(f *File) bool { return !f.Collapsed })
	f := a.curFile()
	for _, g := range a.files {
		g.Collapsed = expanded
	}
	a.relayout()
	if f != nil {
		a.jumpFile(f)
	}
}

func (a *App) gotoFile(f *File) {
	a.focusTree = false
	if f.Collapsed && !f.Viewed {
		f.Collapsed = false
		a.relayout()
	}
	a.jumpFile(f)
}

// nextFile moves d files on. Going back from inside a file goes to its
// header first.
func (a *App) nextFile(d int) {
	f := a.curFile()
	if f == nil {
		return
	}
	i := f.idx + d
	if d < 0 && a.cur != a.lay.fileAt[f] {
		i = f.idx
	}
	if i >= 0 && i < len(a.files) {
		a.jumpFile(a.files[i])
	}
}

func (a *App) nextChange(d int) {
	ch := a.lay.changes
	if d < 0 {
		for k := len(ch) - 1; k >= 0; k-- {
			if ch[k] < a.cur {
				a.jump(ch[k], -1)
				return
			}
		}
		return
	}
	for _, r := range ch {
		if r > a.cur {
			a.jump(r, -1)
			return
		}
	}
}

// gapNear returns the gap index of the first gap row at or after i in
// direction d within f, or -1.
func (a *App) gapNear(f *File, i, d int) int {
	for ; i >= 0 && i < len(a.lay.Rows) && a.lay.Rows[i].F == f; i += d {
		if a.lay.Rows[i].Kind == RowGap {
			return a.lay.Rows[i].Gap
		}
	}
	return -1
}

// expandAround expands the gap at row i, or 20 lines into the gaps on
// either side of the hunk at row i.
func (a *App) expandAround(i int) {
	f := a.lay.Rows[i].F
	if a.lay.Rows[i].Kind == RowGap {
		g := a.lay.Rows[i].Gap
		a.ensureContents(f, func() { f.expandGap(g, 'a', 0) })
		return
	}
	ga, gb := a.gapNear(f, i, -1), a.gapNear(f, i+1, 1)
	if ga < 0 && gb < 0 && f.content == contentLoaded {
		return
	}
	a.ensureContents(f, func() {
		if ga >= 0 {
			f.expandGap(ga, 'u', 20)
		}
		if gb >= 0 {
			f.expandGap(gb, 'd', 20)
		} else if f.content == contentLoaded {
			f.expandGap(len(f.Hunks), 'd', 20)
		}
	})
}

func (a *App) expandFile(f *File) {
	a.ensureContents(f, func() {
		for i := range f.gaps {
			f.expandGap(i, 'a', 0)
		}
	})
}

// selRange returns the lines selected on a side: the visual selection, or
// the cursor line.
func (a *App) selRange(side int) (f *File, from, to int, ok bool) {
	lo, hi := a.cur, a.cur
	if a.sel >= 0 {
		lo, hi = min(a.sel, a.cur), max(a.sel, a.cur)
	}
	f = a.lay.Rows[a.cur].F
	for i := lo; i <= hi; i++ {
		r := &a.lay.Rows[i]
		if r.F != f {
			continue
		}
		if l := r.lineOn(side, a.lay.Split); l != nil {
			n := l.num(side)
			if !ok {
				from, to, ok = n, n, true
			}
			from, to = min(from, n), max(to, n)
		}
	}
	return f, from, to, ok
}

// commentSide picks the side a comment on the cursor row goes on.
func (a *App) commentSide() int {
	r := &a.lay.Rows[a.cur]
	switch {
	case r.Kind == RowFile:
		return b2i(r.F.Status != 'D')
	case a.lay.Split && r.lineOn(a.side, true) != nil:
		return a.side
	case a.lay.Split:
		return 1 - a.side
	case a.sel >= 0:
		_, _, _, ok := a.selRange(1)
		return b2i(ok)
	case r.Kind == RowLine && r.L.Kind == Del:
		return 0
	}
	return 1
}

// openCompose opens a compose box on the selected lines, or on the whole
// file from its header. A file comment has line 0 and can only be sent.
func (a *App) openCompose(side int) {
	if a.lay == nil {
		return
	}
	var c *Card
	switch r := &a.lay.Rows[a.cur]; {
	case r.Kind == RowFile && !a.repo.CanSend():
		a.flash("file comments need a --send command", a.th.Amber)
		return
	case r.Kind == RowFile:
		c = &Card{Kind: CardCompose, File: r.F, Side: side}
	case r.Kind != RowLine:
		return
	case !a.repo.CanSend() && !a.repo.CommentsEnabled():
		a.flash("comments are off: the diff doesn't end at the working copy and nit has no --send", a.th.Amber)
		return
	default:
		f, from, to, ok := a.selRange(side)
		if !ok {
			return
		}
		c = &Card{Kind: CardCompose, File: f, Side: side, Line: to, From: from, To: to}
	}
	a.stashDraft()
	c.Ed = newEditor(a.drafts[c.draftKey()])
	c.Ed.row = len(c.Ed.lines) - 1
	c.Ed.col = len(c.Ed.lines[c.Ed.row])
	a.compose, a.sel, a.searching = c, -1, false
	a.relayout()
	a.revealCompose()
}

// revealCompose scrolls so the whole compose box shows.
func (a *App) revealCompose() {
	first, last := -1, -1
	for i, r := range a.lay.Rows {
		if r.Kind == RowCard && r.Card == a.compose {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return
	}
	if last > a.top+a.ph-2 {
		a.glide(last - a.ph + 3)
	} else if first < a.top+1 {
		a.glide(first - 2)
	}
}

func (a *App) stashDraft() {
	if c := a.compose; c != nil {
		if t := c.Ed.Text(); strings.TrimSpace(t) != "" {
			a.drafts[c.draftKey()] = t
		} else {
			delete(a.drafts, c.draftKey())
		}
	}
}

func (a *App) closeCompose(keep bool) {
	if a.compose == nil {
		return
	}
	if keep {
		a.stashDraft()
	} else {
		delete(a.drafts, a.compose.draftKey())
	}
	a.compose = nil
	a.relayout()
}

// submit sends the compose box's text to the --send command, or writes it
// into the file as a <review> block.
func (a *App) submit(send bool) {
	c := a.compose
	if c == nil || c.Busy != "" {
		return
	}
	text := strings.TrimSpace(c.Ed.Text())
	if text == "" {
		return
	}
	f, r := c.File, a.repo
	var do func() error
	var then func()
	var ok string
	switch {
	case send && !a.repo.CanSend():
		c.Err = "no --send command (pass --send or set NIT_SEND)"
	case send:
		req := review.Comment{File: f.Path, Side: sideName(c.Side), Lines: [2]int{c.From, c.To}, Text: text}
		if c.To > 0 {
			req.Code = f.excerpt(c.Side, c.From, c.To)
		}
		if req.Blob = [2]string{f.OldBlob, f.NewBlob}[c.Side]; strings.Trim(req.Blob, "0") == "" {
			req.Blob = ""
		}
		c.Busy, ok = "sending…", "✓ sent to the session"
		do = func() error { return r.Send(req) }
		then = func() {
			f.Sent = append(f.Sent, &Card{Kind: CardSent, File: f, Side: c.Side, Line: c.Line, From: c.From, To: c.To, Body: text, At: time.Now()})
		}
	case c.To == 0:
		c.Err = "A comment on the whole file can only be sent"
	case f.Pseudo:
		c.Err = "The commit message isn't a file to comment in, use Send"
	case !a.repo.CommentsEnabled():
		c.Err = "Inline comments need the diff to end at the working copy"
	case c.Side == 0:
		c.Err = "Inline comments go on the new side of the diff"
	default:
		lineText, _ := f.sideText(1, c.To)
		c.Busy, ok = "writing…", "✓ comment written to "+f.name()
		do = func() error { return r.AddComment(f.Path, c.To, lineText, text) }
		then = a.load
	}
	if do == nil {
		a.relayout()
		return
	}
	c.Err = ""
	a.relayout()
	go func() {
		err := do()
		a.post(func() {
			c.Busy = ""
			if err != nil {
				c.Err = err.Error()
				a.relayout()
				return
			}
			then()
			if a.compose == c {
				a.closeCompose(false)
			}
			a.flash(ok, a.th.Green)
		})
	}()
}

// resolve removes a card: a sent one is dismissed, a review one is deleted
// from the file.
func (a *App) resolve(c *Card) {
	switch c.Kind {
	case CardSent:
		f := c.File
		if i := slices.Index(f.Sent, c); i >= 0 {
			f.Sent = slices.Delete(f.Sent, i, i+1)
		}
		a.relayout()
	case CardReview:
		f, line, r := c.File, c.Comment.Open, a.repo
		c.Body = "resolving…"
		go func() {
			err := r.RemoveComment(f.Path, line)
			a.post(func() {
				if err != nil {
					a.flash("✗ "+err.Error(), a.th.Red)
					return
				}
				a.flash("✓ resolved", a.th.Green)
				a.load()
			})
		}()
	}
}

func (a *App) yank(s string) {
	a.s.SetClipboard([]byte(s))
	a.flash("copied "+s, a.th.Muted)
}
