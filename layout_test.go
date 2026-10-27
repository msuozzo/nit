package main

import (
	"reflect"
	"strconv"
	"testing"
)

// rowStrs spells rows: file, gapN, note:T, card:Body:Sub, or the line text,
// as L|R in split mode with - for an empty half.
func rowStrs(lay *Layout) []string {
	var out []string
	for _, r := range lay.Rows {
		switch r.Kind {
		case RowFile:
			out = append(out, "file")
		case RowGap:
			out = append(out, "gap"+strconv.Itoa(r.Gap))
		case RowNote:
			out = append(out, "note:"+r.Note)
		case RowCard:
			out = append(out, "card:"+r.Card.Body+":"+strconv.Itoa(r.Sub))
		case RowLine:
			text := func(l *Line) string {
				if l == nil {
					return "-"
				}
				return l.Text
			}
			if lay.Split {
				out = append(out, text(r.L)+"|"+text(r.R))
			} else {
				out = append(out, r.L.Text)
			}
		}
	}
	return out
}

// changeFile has lines a(1:1) b(2:0) c(3:0) B(0:2) d(4:3).
func changeFile(t *testing.T) *File {
	return mkFile(t, slash, "@@ -1,4 +1,3 @@\n a\n-b\n-c\n+B\n d\n")
}

func TestBuildLayout(t *testing.T) {
	f := changeFile(t)
	card := func(side, line int, body string) *Card {
		return &Card{Kind: CardSent, File: f, Side: side, Line: line, Body: body}
	}
	for _, tc := range []struct {
		name   string
		split  bool
		sent   []*Card
		want   []string
		change int
	}{
		{"unified", false, nil, []string{"file", "a", "b", "c", "B", "d"}, 2},
		{"split", true, nil, []string{"file", "a|a", "b|B", "c|-", "d|d"}, 2},
		{"card on an added line", false, []*Card{card(1, 2, "hi")}, []string{"file", "a", "b", "c", "B", "card:hi:0", "card:hi:1", "d"}, 2},
		{"card on an added line, split", true, []*Card{card(1, 2, "hi")}, []string{"file", "a|a", "b|B", "card:hi:0", "card:hi:1", "c|-", "d|d"}, 2},
		{"card on a deleted line", true, []*Card{card(0, 3, "x")}, []string{"file", "a|a", "b|B", "c|-", "card:x:0", "card:x:1", "d|d"}, 2},
		{"card above the first line", false, []*Card{card(1, 0, "top")}, []string{"file", "card:top:0", "card:top:1", "a", "b", "c", "B", "d"}, 4},
		{"card on a hidden line", false, []*Card{card(1, 99, "end")}, []string{"file", "a", "b", "c", "B", "d", "card:end:0", "card:end:1"}, 2},
		{"two cards on one line keep order", false, []*Card{card(1, 1, "p"), card(1, 1, "q")},
			[]string{"file", "a", "card:p:0", "card:p:1", "card:q:0", "card:q:1", "b", "c", "B", "d"}, 6},
	} {
		f.Sent = tc.sent
		lay := buildLayout([]*File{f}, tc.split, 160, nil)
		if got := rowStrs(lay); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: rows %q, want %q", tc.name, got, tc.want)
		}
		if !reflect.DeepEqual(lay.changes, []int{tc.change}) || lay.fileAt[f] != 0 {
			t.Errorf("%s: changes %v fileAt %d, want change at %d", tc.name, lay.changes, lay.fileAt[f], tc.change)
		}
	}
	f.Sent = nil
	f.Collapsed = true
	if got := rowStrs(buildLayout([]*File{f}, false, 160, nil)); !reflect.DeepEqual(got, []string{"file"}) {
		t.Errorf("collapsed rows %q", got)
	}
	f.Collapsed = false
	compose := &Card{Kind: CardCompose, File: f, Side: 1, Line: 3, Ed: newEditor("")}
	if got := rowStrs(buildLayout([]*File{f}, false, 160, compose)); !reflect.DeepEqual(got, []string{"file", "a", "b", "c", "B", "d", "card::0", "card::1", "card::2", "card::3"}) {
		t.Errorf("compose rows %q", got)
	}
	compose.Err = "bad"
	if n := len(buildLayout([]*File{f}, false, 160, compose).Rows); n != 11 {
		t.Errorf("compose with error has %d rows, want 11", n)
	}
	other := mkFile(t, slash, "@@ -1 +1 @@\n-x\n+y\n")
	if n := len(buildLayout([]*File{other}, false, 160, compose).Rows); n != 3 {
		t.Errorf("compose for another file leaked: %d rows", n)
	}
}

func TestTextW(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{{"\tabc", 7}, {"日本語", 6}} {
		f := mkFile(t, slash, "@@ -1 +1 @@\n-x\n+"+tc.line+"\n")
		if w := buildLayout([]*File{f}, true, 160, nil).textW; w != tc.want {
			t.Errorf("textW with %q = %d, want %d", tc.line, w, tc.want)
		}
	}
}

func TestBuildLayoutNotes(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    *File
		want []string
	}{
		{"binary", &File{Path: "i.png", Binary: true}, []string{"file", "note:Binary file not shown"}},
		{"rename", &File{Path: "b", OldPath: "a", Status: 'R'}, []string{"file", "note:Renamed without changes"}},
		{"mode", &File{Path: "r", Status: 'M', Note: "mode 100644 → 100755"}, []string{"file", "note:mode 100644 → 100755"}},
		{"empty", &File{Path: "e", Status: 'M'}, []string{"file", "note:No content changes"}},
	} {
		tc.f.init(nil)
		if got := rowStrs(buildLayout([]*File{tc.f}, true, 160, nil)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: rows %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestBuildLayoutGaps(t *testing.T) {
	f := gapFile(t)
	lay := buildLayout([]*File{f}, false, 160, nil)
	want := []string{"file", "gap0", "b", "c", "C", "C2", "gap1", "j", "k", "K"}
	if got := rowStrs(lay); !reflect.DeepEqual(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if !reflect.DeepEqual(lay.changes, []int{3, 8}) {
		t.Errorf("changes %v", lay.changes)
	}
	f.setContents(numbered("o", 19), numbered("n", 20))
	f.expandGap(0, 'a', 0)
	f.expandGap(1, 'd', 1)
	f.expandGap(1, 'u', 1)
	f.expandGap(2, 'd', 1)
	want = []string{"file", "n1", "b", "c", "C", "C2", "n5", "gap1", "n10", "j", "k", "K", "n13", "gap2"}
	if got := rowStrs(buildLayout([]*File{f}, false, 160, nil)); !reflect.DeepEqual(got, want) {
		t.Errorf("expanded rows %q, want %q", got, want)
	}
	if got := rowStrs(buildLayout([]*File{f}, true, 160, nil))[1]; got != "n1|n1" {
		t.Errorf("split gap line %q", got)
	}
}

func TestCardGeometry(t *testing.T) {
	f := changeFile(t)
	for _, tc := range []struct {
		split  bool
		w      int
		side   int
		x0, x1 int
	}{
		{false, 160, 1, 9, 160},
		{true, 160, 0, 5, 79},
		{true, 160, 1, 85, 160},
		{false, 20, 1, 0, 20},
		{true, 40, 1, 20, 40},
	} {
		c := &Card{Kind: CardSent, File: f, Side: tc.side, Line: 1, Body: "x"}
		f.Sent = []*Card{c}
		buildLayout([]*File{f}, tc.split, tc.w, nil)
		if c.x0 != tc.x0 || c.x1 != tc.x1 || len(c.wrap) != 1 {
			t.Errorf("split=%v w=%d side=%d: card at %d..%d wrap %q, want %d..%d", tc.split, tc.w, tc.side, c.x0, c.x1, c.wrap, tc.x0, tc.x1)
		}
	}
}

func TestLayoutRemap(t *testing.T) {
	f := changeFile(t)
	f.Sent = []*Card{{Kind: CardSent, File: f, Side: 1, Line: 2, Body: "hi"}}
	files := map[string]*File{f.Path: f}
	uni := buildLayout([]*File{f}, false, 160, nil)
	split := buildLayout([]*File{f}, true, 160, nil)
	for _, tc := range []struct {
		name     string
		from, to *Layout
		row      int
		want     string
	}{
		{"added line", uni, split, 4, "b|B"},
		{"deleted line", uni, split, 3, "c|-"},
		{"context line", uni, split, 7, "d|d"},
		{"paired row keeps the new side", split, uni, 2, "B"},
		{"lone deletion", split, uni, 5, "c"},
		{"card lands on its anchor line", uni, split, 6, "b|B"},
		{"card above the first line", uni, split, 0, "file"},
	} {
		k := tc.from.key(tc.row)
		if got := rowStrs(tc.to)[tc.to.find(k, files)]; got != tc.want {
			t.Errorf("%s: %+v found %q, want %q", tc.name, k, got, tc.want)
		}
	}
	f.Sent = nil
	plain := buildLayout([]*File{f}, false, 160, nil)
	for _, tc := range []struct {
		name string
		k    rowKey
		lay  *Layout
		want int
	}{
		{"card gone lands on its line", uni.key(5), plain, 4},
		{"line after the last known", rowKey{path: "f.go", kind: RowLine, side: 1, line: 99}, plain, 5},
		{"line before the first known", rowKey{path: "f.go", kind: RowLine, side: 0, line: 0}, plain, 0},
		{"unknown file", rowKey{path: "zzz", kind: RowLine, side: 1, line: 1}, plain, 0},
		{"out of range row", uni.key(99), plain, 0},
	} {
		if got := tc.lay.find(tc.k, files); got != tc.want {
			t.Errorf("%s: find = %d, want %d", tc.name, got, tc.want)
		}
	}
	f.Collapsed = true
	if got := buildLayout([]*File{f}, false, 160, nil).find(uni.key(4), files); got != 0 {
		t.Errorf("collapsed file: find = %d, want the header", got)
	}
	g := gapFile(t)
	gu, gs := buildLayout([]*File{g}, false, 160, nil), buildLayout([]*File{g}, true, 160, nil)
	if k := gu.key(6); k.kind != RowGap || k.line != 1 || rowStrs(gs)[gs.find(k, map[string]*File{g.Path: g})] != "gap1" {
		t.Errorf("gap key %+v did not find gap1", k)
	}
}

func TestWrapText(t *testing.T) {
	for _, tc := range []struct {
		s    string
		w    int
		want []string
	}{
		{"", 10, []string{""}},
		{"a b\n\nc", 10, []string{"a b", "", "c"}},
		{"hello world foo", 6, []string{"hello ", "world ", "foo"}},
		{"abcdefgh", 4, []string{"abcd", "efgh"}},
	} {
		if got := wrapText(tc.s, tc.w); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("wrapText(%q, %d) = %q, want %q", tc.s, tc.w, got, tc.want)
		}
	}
}
