package main

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/msuozzo/nit/review"
)

var slash = review.Syntax{Prefix: "//"}

// mkFile parses one file's hunks into an initialized File.
func mkFile(t *testing.T, syn review.Syntax, hunks string) *File {
	t.Helper()
	fs := parsePatch("diff --git a/f.go b/f.go\n" + hunks)
	if len(fs) != 1 {
		t.Fatalf("parsed %d files", len(fs))
	}
	fs[0].init(&syn)
	return fs[0]
}

// gapFile is a file with hunks at old 2-3/new 2-4 and old 10-11/new 11-12,
// so three gaps: before, between and after.
func gapFile(t *testing.T) *File {
	return mkFile(t, slash, "@@ -2,2 +2,3 @@\n b\n-c\n+C\n+C2\n@@ -10,2 +11,2 @@\n j\n-k\n+K\n")
}

// numbered is n lines of prefix1, prefix2, ... as file contents.
func numbered(prefix string, n int) *string {
	var ls []string
	for i := 1; i <= n; i++ {
		ls = append(ls, prefix+strconv.Itoa(i))
	}
	return ptr(strings.Join(ls, "\n") + "\n")
}

func TestExtractComments(t *testing.T) {
	xml := review.Syntax{Prefix: "<!--", Suffix: "-->"}
	for _, tc := range []struct {
		name  string
		syn   review.Syntax
		hunks string
		want  []Comment
		adds  int
		kept  []string
	}{
		{"added block", slash, "@@ -1,2 +1,5 @@\n a\n+b\n+// <review>\n+// why\n+// </review>\n c\n",
			[]Comment{{Open: 3, Anchor: 2, Body: "why"}}, 1, []string{" 1:1 a", "+0:2 b", " 2:6 c"}},
		{"context block with blank line", slash, "@@ -1,7 +1,7 @@\n a\n // <review>\n // why\n //\n // here?\n // </review>\n c\n",
			[]Comment{{Open: 2, Anchor: 1, Body: "why\n\nhere?"}}, 0, []string{" 1:1 a", " 7:7 c"}},
		{"only deletions before the tag", slash, "@@ -3,1 +3,3 @@\n-x\n+// <review>\n+r\n+// </review>\n",
			[]Comment{{Open: 3, Anchor: 2, Body: "r"}}, 0, []string{"-3:0 x"}},
		{"two blocks", slash, "@@ -1,1 +1,7 @@\n+// <review>\n+one\n+// </review>\n a\n+// <review>\n+two\n+// </review>\n",
			[]Comment{{Open: 1, Anchor: 0, Body: "one"}, {Open: 5, Anchor: 4, Body: "two"}}, 0, []string{" 1:4 a"}},
		{"deleted block stays", slash, "@@ -1,3 +1,0 @@\n-// <review>\n-gone\n-// </review>\n",
			nil, 0, []string{"-1:0 // <review>", "-2:0 gone", "-3:0 // </review>"}},
		{"unclosed", slash, "@@ -1,2 +1,2 @@\n // <review>\n why\n", nil, 0, []string{" 1:1 // <review>", " 2:2 why"}},
		{"closed on the other side", slash, "@@ -1,2 +1,4 @@\n+// <review>\n+why\n // </review>\n z\n",
			nil, 2, []string{"+0:1 // <review>", "+0:2 why", " 1:3 // </review>", " 2:4 z"}},
		{"xml syntax", xml, "@@ -1,1 +1,4 @@\n a\n+<!-- <review> -->\n+<!-- hi -->\n+<!-- </review> -->\n",
			[]Comment{{Open: 2, Anchor: 1, Body: "hi"}}, 0, []string{" 1:1 a"}},
	} {
		f := mkFile(t, tc.syn, tc.hunks)
		var got []Comment
		for _, c := range f.Comments {
			got = append(got, *c)
		}
		if !reflect.DeepEqual(got, tc.want) || f.Adds != tc.adds {
			t.Errorf("%s: comments %+v adds %d, want %+v adds %d", tc.name, got, f.Adds, tc.want, tc.adds)
		}
		if kept := lineStrs(f.Hunks[0].Lines); !reflect.DeepEqual(kept, tc.kept) {
			t.Errorf("%s: kept %q, want %q", tc.name, kept, tc.kept)
		}
		if len(f.Reviews) != len(f.Comments) {
			t.Errorf("%s: %d review cards for %d comments", tc.name, len(f.Reviews), len(f.Comments))
		}
		for i, c := range f.Reviews {
			if c.Kind != CardReview || c.Side != 1 || c.Line != f.Comments[i].Anchor || c.Comment != f.Comments[i] {
				t.Errorf("%s: card %d = %+v", tc.name, i, c)
			}
		}
	}
}

func TestTokenize(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want []string
	}{
		{"ab_1 (x)\t", []string{"ab_1", " ", "(", "x", ")", "\t"}},
		{"héllo  wörld", []string{"héllo", "  ", "wörld"}},
		{"a->b", []string{"a", "-", ">", "b"}},
		{"", nil},
	} {
		var got []string
		for _, sp := range tokenize(tc.s) {
			got = append(got, tc.s[sp.A:sp.B])
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("tokenize(%q) = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestWordDiff(t *testing.T) {
	for _, tc := range []struct {
		a, b         string
		wantA, wantB []Span
	}{
		{"foo(x, y)", "foo(x, z)", []Span{{7, 8}}, []Span{{7, 8}}},
		{"set x y done", "set p q done", []Span{{4, 7}}, []Span{{4, 7}}},
		{"a.b", "a.c", []Span{{2, 3}}, []Span{{2, 3}}},
		{"\tif x {", "\tif y {", []Span{{4, 5}}, []Span{{4, 5}}},
		{"return a", "return a, b", nil, []Span{{8, 11}}},
		{"abc", "abc", nil, nil},
		{"alpha beta", "gamma delta", nil, nil},
		{"", "x", nil, []Span{{0, 1}}},
	} {
		a, b := &Line{Text: tc.a}, &Line{Text: tc.b}
		wordDiff(a, b)
		if !reflect.DeepEqual(a.Words, tc.wantA) || !reflect.DeepEqual(b.Words, tc.wantB) {
			t.Errorf("wordDiff(%q, %q) = %v, %v, want %v, %v", tc.a, tc.b, a.Words, b.Words, tc.wantA, tc.wantB)
		}
	}
}

func TestChangedSpans(t *testing.T) {
	const s = "a b  c"
	for _, tc := range []struct {
		keep []bool
		want []Span
	}{
		{[]bool{false, true, false, true, false}, []Span{{0, 3}, {5, 6}}},
		{[]bool{false, false, false, false, false}, []Span{{0, 6}}},
		{[]bool{true, true, true, true, true}, nil},
		{[]bool{true, true, true, true, false}, []Span{{5, 6}}},
	} {
		if got := changedSpans(tokenize(s), tc.keep, s); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("changedSpans(%v) = %v, want %v", tc.keep, got, tc.want)
		}
	}
}

func TestComputeWords(t *testing.T) {
	f := mkFile(t, slash, "@@ -1,3 +1,4 @@\n-a = 1\n-b = 2\n+a = 9\n+b = 2\n ctx\n+x = 3\n")
	ls := f.Hunks[0].Lines
	want := [][]Span{{{4, 5}}, nil, {{4, 5}}, nil, nil, nil}
	for i, l := range ls {
		if !reflect.DeepEqual(l.Words, want[i]) {
			t.Errorf("line %d %q words %v, want %v", i, l.Text, l.Words, want[i])
		}
	}
}

func TestGaps(t *testing.T) {
	f := gapFile(t)
	if len(f.gaps) != 3 || f.numW != 3 {
		t.Fatalf("gaps %d numW %d", len(f.gaps), f.numW)
	}
	type info struct{ oldS, newS, count int }
	check := func(name string, want ...info) {
		t.Helper()
		for i, w := range want {
			o, n, c := f.gapInfo(i)
			if got := (info{o, n, c}); got != w {
				t.Errorf("%s: gapInfo(%d) = %+v, want %+v", name, i, got, w)
			}
		}
	}
	check("unloaded", info{1, 1, 1}, info{4, 5, 6}, info{12, 13, 0})
	f.setContents(numbered("o", 19), numbered("n", 20))
	check("loaded", info{1, 1, 1}, info{4, 5, 6}, info{12, 13, 8})
	if l := f.gapLine(4, 5, 0); lineStr(l) != " 4:5 n5" {
		t.Errorf("gapLine = %q", lineStr(l))
	}
	f.expandGap(1, 'd', 20)
	f.expandGap(2, 'u', 3)
	f.expandGap(2, 'd', 10)
	f.expandGap(2, 'a', 0)
	f.expandGap(0, 'a', 0)
	want := []Gap{{1, 0}, {6, 0}, {5, 3}}
	if !reflect.DeepEqual(f.gaps, want) {
		t.Errorf("gaps = %v, want %v", f.gaps, want)
	}
	f.fitNumbers()
	if f.numW != 3 {
		t.Errorf("numW = %d after expanding to line 18", f.numW)
	}
	f.setContents(numbered("o", 19), numbered("n", 1200))
	f.expandGap(2, 'a', 0)
	f.fitNumbers()
	if f.gaps[2].Top != 1188-3 || f.numW != 4 {
		t.Errorf("after expanding a 1200-line file: gap %v numW %d", f.gaps[2], f.numW)
	}
	added := mkFile(t, slash, "@@ -0,0 +1,2 @@\n+a\n+b\n")
	added.Status = 'A'
	added.setContents(nil, numbered("n", 5))
	if _, _, c := added.gapInfo(1); c != 0 {
		t.Errorf("added file trailing gap = %d, want 0", c)
	}
}

func TestSplitLines(t *testing.T) {
	for _, tc := range []struct {
		in   *string
		want []string
	}{
		{nil, []string{}},
		{ptr(""), []string{}},
		{ptr("\n"), []string{}},
		{ptr("a\r\nb\n"), []string{"a", "b"}},
		{ptr("a\n\nb"), []string{"a", "", "b"}},
		{ptr("a\n\n"), []string{"a", ""}},
	} {
		if got := splitLines(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitLines(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestExcerpt(t *testing.T) {
	f := gapFile(t)
	for _, tc := range []struct {
		name           string
		side, from, to int
		want           []review.CodeRun
	}{
		{"new side across hunks", 1, 4, 11, []review.CodeRun{{Lines: [2]int{2, 4}, Text: "b\nC\nC2"}, {Lines: [2]int{11, 12}, Text: "j\nK"}}},
		{"old side", 0, 3, 3, []review.CodeRun{{Lines: [2]int{2, 3}, Text: "b\nc"}}},
		{"nothing known", 1, 7, 7, nil},
	} {
		if got := f.excerpt(tc.side, tc.from, tc.to); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: excerpt = %+v, want %+v", tc.name, got, tc.want)
		}
	}
	f.setContents(numbered("o", 19), numbered("n", 20))
	for _, tc := range []struct {
		from, to int
		want     review.CodeRun
	}{
		{1, 2, review.CodeRun{Lines: [2]int{1, 4}, Text: "n1\nn2\nn3\nn4"}},
		{20, 20, review.CodeRun{Lines: [2]int{18, 20}, Text: "n18\nn19\nn20"}},
		{10, 10, review.CodeRun{Lines: [2]int{8, 12}, Text: "n8\nn9\nn10\nn11\nn12"}},
	} {
		if got := f.excerpt(1, tc.from, tc.to); len(got) != 1 || got[0] != tc.want {
			t.Errorf("loaded excerpt(%d, %d) = %+v, want %+v", tc.from, tc.to, got, tc.want)
		}
	}
	if text, ok := f.sideText(0, 19); !ok || text != "o19" {
		t.Errorf("sideText(0, 19) = %q, %v", text, ok)
	}
	if _, ok := f.sideText(0, 20); ok {
		t.Error("sideText past the end succeeded")
	}
}
