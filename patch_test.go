package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// lineStr spells a line as "<kind><old>:<new> <text>".
func lineStr(l *Line) string {
	return fmt.Sprintf("%c%d:%d %s", " +-"[l.Kind], l.Old, l.New, l.Text)
}

func lineStrs(ls []*Line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, lineStr(l))
	}
	return out
}

func TestParsePatch(t *testing.T) {
	const patch = "diff --git a/a.go b/a.go\n" +
		"index 1111..2222 100644\n--- a/a.go\n+++ b/a.go\n" +
		"@@ -1,3 +1,4 @@ func f\n ctx\n-old\n+new\n+more\n end\n\\ No newline at end of file\n" +
		"diff --git a/old.txt b/new.txt\nsimilarity index 90%\nrename from old.txt\nrename to new.txt\n" +
		"diff --git a/q b/q\nrename from \"q\\tx\"\nrename to q\n" +
		"diff --git a/new.go b/new.go\nnew file mode 100644\nindex 0000000..abc1234\n--- /dev/null\n+++ b/new.go\n@@ -0,0 +1 @@\n+hi\n" +
		"diff --git a/gone.go b/gone.go\ndeleted file mode 100644\nindex abc1234..0000000\n--- a/gone.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-bye\n" +
		"diff --git a/run.sh b/run.sh\nold mode 100644\nnew mode 100755\n" +
		"diff --git a/img.png b/img.png\nindex 1..2 100644\nBinary files a/img.png and b/img.png differ\n" +
		"diff --git a/c.txt b/c.txt\nnew file mode 100644\nindex 0..3\n--- /dev/null\n+++ b/c.txt\n@@ -0,0 +1 @@\n+x\n" +
		"diff --git a/crlf.txt b/crlf.txt\nindex 3..4 100644\n--- a/crlf.txt\n+++ b/crlf.txt\n@@ -1,2 +1,2 @@\n-a\r\n+b\r\n\n" +
		"@@ -9,2 +9,2 @@ second\n-c\n+d\n e\n"
	type want struct {
		path, oldPath string
		status        byte
		binary        bool
		note          string
		blobs         [2]string
		adds, dels    int
		hunks         []string
		lines         []string
	}
	wants := []want{
		{path: "a.go", oldPath: "a.go", status: 'M', blobs: [2]string{"1111", "2222"}, adds: 2, dels: 1,
			hunks: []string{"@@ -1,3 +1,4 @@ func f"}, lines: []string{" 1:1 ctx", "-2:0 old", "+0:2 new", "+0:3 more", " 3:4 end"}},
		{path: "new.txt", oldPath: "old.txt", status: 'R'},
		{path: "q", oldPath: "q\tx", status: 'R'},
		{path: "new.go", oldPath: "new.go", status: 'A', blobs: [2]string{"0000000", "abc1234"}, adds: 1,
			hunks: []string{"@@ -0,0 +1,1 @@ "}, lines: []string{"+0:1 hi"}},
		{path: "gone.go", oldPath: "gone.go", status: 'D', blobs: [2]string{"abc1234", "0000000"}, dels: 1,
			hunks: []string{"@@ -1,1 +0,0 @@ "}, lines: []string{"-1:0 bye"}},
		{path: "run.sh", oldPath: "run.sh", status: 'M', note: "mode 100644 → 100755"},
		{path: "img.png", oldPath: "img.png", status: 'M', binary: true, blobs: [2]string{"1", "2"}},
		{path: "c.txt", oldPath: "c.txt", status: 'A', blobs: [2]string{"0", "3"}, adds: 1, hunks: []string{"@@ -0,0 +1,1 @@ "}, lines: []string{"+0:1 x"}},
		{path: "crlf.txt", oldPath: "crlf.txt", status: 'M', blobs: [2]string{"3", "4"}, adds: 2, dels: 2,
			hunks: []string{"@@ -1,2 +1,2 @@ ", "@@ -9,2 +9,2 @@ second"},
			lines: []string{"-1:0 a", "+0:1 b", " 2:2 ", "-9:0 c", "+0:9 d", " 10:10 e"}},
	}
	files := parsePatch(patch)
	if len(files) != len(wants) {
		t.Fatalf("got %d files, want %d", len(files), len(wants))
	}
	for i, f := range files {
		var hunks, lines []string
		for _, h := range f.Hunks {
			hunks = append(hunks, fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", h.OldStart, h.OldCount, h.NewStart, h.NewCount, h.Heading))
			lines = append(lines, lineStrs(h.Lines)...)
		}
		got := want{f.Path, f.OldPath, f.Status, f.Binary, f.Note, [2]string{f.OldBlob, f.NewBlob}, f.Adds, f.Dels, hunks, lines}
		if !reflect.DeepEqual(got, wants[i]) {
			t.Errorf("file %d:\n got %+v\nwant %+v", i, got, wants[i])
		}
	}
}

func TestHunkBounds(t *testing.T) {
	for _, tc := range []struct {
		h                    Hunk
		firstOld, lastOld    int
		firstNew, lastNew    int
		wantOldN, wantNewLen int
	}{
		{h: Hunk{OldStart: 5, OldCount: 3, NewStart: 7, NewCount: 2}, firstOld: 5, lastOld: 7, firstNew: 7, lastNew: 8},
		{h: Hunk{OldStart: 0, OldCount: 0, NewStart: 1, NewCount: 4}, firstOld: 1, lastOld: 0, firstNew: 1, lastNew: 4},
		{h: Hunk{OldStart: 3, OldCount: 0, NewStart: 4, NewCount: 1}, firstOld: 4, lastOld: 3, firstNew: 4, lastNew: 4},
		{h: Hunk{OldStart: 4, OldCount: 1, NewStart: 3, NewCount: 0}, firstOld: 4, lastOld: 4, firstNew: 4, lastNew: 3},
	} {
		got := [4]int{tc.h.firstOld(), tc.h.lastOld(), tc.h.firstNew(), tc.h.lastNew()}
		if want := [4]int{tc.firstOld, tc.lastOld, tc.firstNew, tc.lastNew}; got != want {
			t.Errorf("%+v bounds = %v, want %v", tc.h, got, want)
		}
	}
}

func TestParsePatchEmptyAndGarbage(t *testing.T) {
	if fs := parsePatch(""); len(fs) != 0 {
		t.Errorf("empty patch parsed to %d files", len(fs))
	}
	fs := parsePatch("junk\n@@ -1 +1 @@\n-x\n+y\n" + strings.Repeat("diff --git a/a b/a\n", 2))
	if len(fs) != 2 || len(fs[0].Hunks) != 0 {
		t.Errorf("hunks before any file header were kept: %+v", fs)
	}
}
