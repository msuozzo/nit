package main

import (
	"reflect"
	"testing"

	"github.com/msuozzo/jj-highlight/syntax"
)

func TestHighlightLines(t *testing.T) {
	got := highlightLines("x.go", []string{"/* a", "b */", "func f() int { return 1 }"})
	want := [][]Seg{
		{{4, syntax.Comment}},
		{{4, syntax.Comment}},
		{{4, syntax.Keyword}, {9, syntax.None}, {12, syntax.Type}, {15, syntax.None}, {21, syntax.Keyword}, {22, syntax.None}, {23, syntax.Number}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("highlightLines =\n%v\nwant\n%v", got, want)
	}
	if got := highlightLines("notes.txt", []string{"func"}); len(got) != 1 || got[0] != nil {
		t.Errorf("unsupported file highlighted: %v", got)
	}
	if got := highlightLines("x.go", nil); len(got) != 0 {
		t.Errorf("no lines highlighted to %v", got)
	}
}

func TestSideHighlights(t *testing.T) {
	f := mkFile(t, slash, "@@ -1,2 +1,2 @@\n package a\n-// x\n+// y\n")
	hl := fragmentHL(f)
	pkg := []Seg{{7, syntax.Keyword}, {8, syntax.None}, {9, syntax.Type}}
	comment := []Seg{{4, syntax.Comment}}
	for side, want := range []map[int][]Seg{{1: pkg, 2: comment}, {1: pkg, 2: comment}} {
		if !reflect.DeepEqual(hl[side], want) {
			t.Errorf("fragmentHL side %d = %v, want %v", side, hl[side], want)
		}
	}
	full := fullHL(f, []string{"package a", "// x"}, []string{"package a", "", "// y"})
	if !reflect.DeepEqual(full[0], map[int][]Seg{1: pkg, 2: comment}) || !reflect.DeepEqual(full[1], map[int][]Seg{1: pkg, 2: nil, 3: comment}) {
		t.Errorf("fullHL = %v", full)
	}
}
