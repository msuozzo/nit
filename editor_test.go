package main

import (
	"reflect"
	"strings"
	"testing"
)

// cursorEditor builds an editor from text with | marking the cursor.
func cursorEditor(s string) *Editor {
	i := strings.IndexByte(s, '|')
	e := newEditor(s[:i] + s[i+1:])
	before := s[:i]
	e.row = strings.Count(before, "\n")
	e.col = len([]rune(before[strings.LastIndexByte(before, '\n')+1:]))
	return e
}

func withCursor(e *Editor) string {
	var b strings.Builder
	for r, l := range e.lines {
		if r > 0 {
			b.WriteByte('\n')
		}
		if r == e.row {
			b.WriteString(string(l[:e.col]) + "|" + string(l[e.col:]))
		} else {
			b.WriteString(string(l))
		}
	}
	return b.String()
}

func TestEditorKeys(t *testing.T) {
	for _, tc := range []struct {
		in   string
		keys []string
		want string
	}{
		{"hello| world", []string{"C-w"}, "| world"},
		{"foo bar|", []string{"M-bs"}, "foo |"},
		{"foo.|bar", []string{"C-w"}, "|bar"},
		{"hello |world", []string{"M-d"}, "hello |"},
		{"ab|cd", []string{"C-u"}, "|cd"},
		{"ab|cd", []string{"C-k"}, "ab|"},
		{"ab|\ncd", []string{"C-k"}, "ab|cd"},
		{"ab\n|cd", []string{"bs"}, "ab|cd"},
		{"|ab", []string{"bs"}, "|ab"},
		{"ab|cd", []string{"del"}, "ab|d"},
		{"ab|", []string{"del"}, "ab|"},
		{"ab|\ncd", []string{"del"}, "ab|cd"},
		{"a|b", []string{"S-enter"}, "a\n|b"},
		{"a|b", []string{"C-j", "left"}, "a|\nb"},
		{"ab\n|cd", []string{"left"}, "ab|\ncd"},
		{"ab|\ncd", []string{"right"}, "ab\n|cd"},
		{"|ab", []string{"left"}, "|ab"},
		{"ab|", []string{"right"}, "ab|"},
		{"foo bar|", []string{"M-b"}, "foo |bar"},
		{"foo bar|", []string{"C-left", "C-left"}, "|foo bar"},
		{"|foo bar", []string{"M-f"}, "foo| bar"},
		{"|foo  bar!", []string{"M-f", "M-f"}, "foo  bar|!"},
		{"ab|", []string{"home"}, "|ab"},
		{"|ab", []string{"C-e"}, "ab|"},
		{"aaaa bb|bb cccc", []string{"down"}, "aaaa bbbb cc|cc"},
		{"aaaa bbbb cc|cc", []string{"up", "up"}, "aa|aa bbbb cccc"},
		{"aa|aa bbbb cccc", []string{"up"}, "|aaaa bbbb cccc"},
		{"aaaa bbbb cc|cc", []string{"down"}, "aaaa bbbb cccc|"},
		{"ab|\ncd", []string{"down"}, "ab\ncd|"},
		{"日本|語 x", []string{"up"}, "|日本語 x"},
	} {
		e := cursorEditor(tc.in)
		e.wrap(5)
		for _, k := range tc.keys {
			if !e.Key(k) {
				t.Errorf("%q: key %q not handled", tc.in, k)
			}
		}
		if got := withCursor(e); got != tc.want {
			t.Errorf("%q %v = %q, want %q", tc.in, tc.keys, got, tc.want)
		}
	}
	if e := newEditor(""); e.Key("x") || e.Key("C-z") {
		t.Error("unknown keys reported as handled")
	}
}

func TestEditorInsert(t *testing.T) {
	e := newEditor("a\r\nb\x01c\td")
	if got := e.Text(); got != "a\nbc\td" {
		t.Errorf("Text = %q", got)
	}
	if e.row != 1 || e.col != 4 {
		t.Errorf("cursor at %d:%d, want end", e.row, e.col)
	}
	e = cursorEditor("a|c")
	e.Insert('b')
	e.InsertText("\nx")
	if got := withCursor(e); got != "ab\nx|c" {
		t.Errorf("after inserts %q", got)
	}
}

func TestEditorWrap(t *testing.T) {
	for _, tc := range []struct {
		text string
		w    int
		want []string
	}{
		{"hello world foo", 6, []string{"hello ", "world ", "foo"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"日本語", 4, []string{"日本", "語"}},
		{"a\n\nb", 10, []string{"a", "", "b"}},
		{"", 10, []string{""}},
		{"ab cd", 2, []string{"ab ", "cd"}},
		{"aaaa bbbb cccc", 5, []string{"aaaa ", "bbbb ", "cccc"}},
		{"a  b", 4, []string{"a  b"}},
	} {
		e := newEditor(tc.text)
		var got []string
		for _, v := range e.wrap(tc.w) {
			got = append(got, string(e.lines[v.row][v.a:v.b]))
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("wrap(%q, %d) = %q, want %q", tc.text, tc.w, got, tc.want)
		}
	}
}

func TestEditorCursorVis(t *testing.T) {
	e := cursorEditor("aaaa bbbb|")
	vl := e.wrap(5)
	if i, x := e.cursorVis(vl); i != 1 || x != 4 {
		t.Errorf("cursor at end of a wrapped word: visual %d col %d, want 1, 4", i, x)
	}
	e.setVis(vl[0], 99)
	if e.col != 5 {
		t.Errorf("setVis past the end: col %d, want 5", e.col)
	}
	e.setVis(vl[1], 2)
	if e.col != 7 {
		t.Errorf("setVis in the second visual line: col %d, want 7", e.col)
	}
}
