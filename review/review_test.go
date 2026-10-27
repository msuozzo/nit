package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestPrompt pins the prompt text, which the review skill parses. The first
// case is the example from skepsis's README.
func TestPrompt(t *testing.T) {
	for _, tc := range []struct {
		side  string
		lines [2]int
		code  []CodeRun
		want  string
	}{
		{"additions", [2]int{42, 45}, []CodeRun{{[2]int{41, 46}, "const z = 0\nconst a = 1\n…"}},
			"Review comment on src/a.ts, lines 42–45 (new side of `jj diff -r @-`), in lines 41–46:\n\n" +
				"```\nconst z = 0\nconst a = 1\n…\n```\n\nwhy two?"},
		{"deletions", [2]int{7, 7}, []CodeRun{{[2]int{5, 9}, "a ```b```"}, {[2]int{12, 12}, "c"}},
			"Review comment on src/a.ts, line 7 (old side of `jj diff -r @-`), in lines 5–9 and line 12:\n\n" +
				"````\na ```b```\n````\n\n```\nc\n```\n\nwhy two?"},
		{"additions", [2]int{1, 2}, nil, "Review comment on src/a.ts, lines 1–2 (new side of `jj diff -r @-`):\n\nwhy two?"},
	} {
		p := payload{File: "src/a.ts", Side: tc.side, Lines: tc.lines, Code: tc.code, Text: "why two?"}
		p.Diff.Command = "jj diff -r @-"
		if got := prompt(p); got != tc.want {
			t.Errorf("prompt(%s %v) =\n%s\nwant\n%s", tc.side, tc.lines, got, tc.want)
		}
	}
}

func TestSyntaxFor(t *testing.T) {
	for _, tc := range []struct {
		path, first string
		want        Syntax
		ok          bool
	}{
		{"a/main.go", "", slashC, true},
		{"x.py", "", hashC, true},
		{"README.md", "", xmlC, true},
		{"s.css", "", blockC, true},
		{"q.sql", "", dashC, true},
		{"t.tex", "", percentC, true},
		{"e.el", "", semiC, true},
		{".vimrc", "", quoteC, true},
		{"Makefile", "", hashC, true},
		{"Dockerfile.builder", "", hashC, true},
		{"bin/run", "#!/usr/bin/env -S python3 -u", hashC, true},
		{"bin/tool", "#!/usr/bin/node", slashC, true},
		{"bin/l", "#!/usr/bin/lua5.4", dashC, true},
		{"notes.txt", "", Syntax{}, false},
		{"bin/blob", "\x7fELF", Syntax{}, false},
		{"bin/x", "#!/usr/bin/env", Syntax{}, false},
	} {
		got, ok := SyntaxFor(tc.path, tc.first)
		if got != tc.want || ok != tc.ok {
			t.Errorf("SyntaxFor(%q, %q) = %+v, %v, want %+v, %v", tc.path, tc.first, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTagStrip(t *testing.T) {
	for _, tc := range []struct {
		syn        Syntax
		line       string
		open, body string
	}{
		{slashC, "// <review>", "open", ""},
		{slashC, "\t  //<review>  ", "open", ""},
		{slashC, "// <review> here", "", "<review> here"},
		{slashC, "<review>", "", "<review>"},
		{slashC, "  // why", "", "why"},
		{slashC, "//", "", ""},
		{slashC, "why", "", "why"},
		{xmlC, "<!-- <review> -->", "open", ""},
		{xmlC, "<!-- <review>", "open", ""},
		{xmlC, "  <!-- hi there -->", "", "hi there"},
		{Syntax{}, "<review>", "open", ""},
		{Syntax{}, "  text  ", "", "text  "},
	} {
		if got := tc.syn.Tag("<review>").MatchString(tc.line); got != (tc.open == "open") {
			t.Errorf("%+v Tag(<review>) matches %q = %v", tc.syn, tc.line, got)
		}
		if tc.open == "" {
			if got := tc.syn.Strip(tc.line); got != tc.body {
				t.Errorf("%+v Strip(%q) = %q, want %q", tc.syn, tc.line, got, tc.body)
			}
		}
	}
}

func TestCommentRoundTrip(t *testing.T) {
	r := &Repo{Root: t.TempDir()}
	for _, tc := range []struct {
		file, src string
		after     int
		want      string
	}{
		{"a.go", "package a\n\nfunc f() {\n\treturn\n}\n", 4,
			"package a\n\nfunc f() {\n\treturn\n\t// <review>\n\t// why\n\t//\n\t// here?\n\t// </review>\n}\n"},
		{"s.css", "a {\n  b: c;\n}\n", 2,
			"a {\n  b: c;\n  /* <review> */\n  /* why */\n  /* */\n  /* here? */\n  /* </review> */\n}\n"},
		{"w.txt", "one\r\ntwo\r\n", 1, "one\r\n<review>\r\nwhy\r\n\r\nhere?\r\n</review>\r\ntwo\r\n"},
		{"run", "#!/bin/sh\necho hi\n", 2, "#!/bin/sh\necho hi\n# <review>\n# why\n#\n# here?\n# </review>\n"},
		{"noeol.go", "package a", 1, "package a\n// <review>\n// why\n//\n// here?\n// </review>"},
	} {
		path := filepath.Join(r.Root, tc.file)
		os.WriteFile(path, []byte(tc.src), 0o600)
		line := strings.TrimSuffix(strings.Split(tc.src, "\n")[tc.after-1], "\r")
		if err := r.AddComment(tc.file, tc.after, line, "why\n\nhere?"); err != nil {
			t.Fatalf("%s: AddComment: %v", tc.file, err)
		}
		if b, _ := os.ReadFile(path); string(b) != tc.want {
			t.Errorf("%s after AddComment:\n%q\nwant\n%q", tc.file, b, tc.want)
		}
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, want 0600", tc.file, fi.Mode().Perm())
		}
		if err := r.RemoveComment(tc.file, tc.after+1); err != nil {
			t.Fatalf("%s: RemoveComment: %v", tc.file, err)
		}
		if b, _ := os.ReadFile(path); string(b) != tc.src {
			t.Errorf("%s after RemoveComment:\n%q\nwant\n%q", tc.file, b, tc.src)
		}
	}
	os.Symlink("a.go", filepath.Join(r.Root, "link.go"))
	os.WriteFile(filepath.Join(r.Root, "open.go"), []byte("// <review>\nx\n"), 0o600)
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"changed line", r.AddComment("a.go", 1, "package b", "x"), "changed on disk"},
		{"line past end", r.AddComment("a.go", 9, "", "x"), "out of range"},
		{"symlink", r.AddComment("link.go", 1, "package a", "x"), "symlink"},
		{"missing file", r.AddComment("nope.go", 1, "", "x"), "no such file"},
		{"not a tag", r.RemoveComment("a.go", 1), "not a <review> open tag"},
		{"unclosed", r.RemoveComment("open.go", 1), "no </review>"},
	} {
		if tc.err == nil || !strings.Contains(tc.err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, tc.err, tc.want)
		}
	}
}

func TestViewed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := &Repo{Root: "/some/repo"}
	r.SetViewed("a.go", "111", true)
	r.SetViewed("b.go", "222", true)
	r.SetViewed("b.go", "222", false)
	sum := sha256.Sum256([]byte(r.Root))
	path := filepath.Join(os.Getenv("HOME"), ".local", "share", "skepsis", hex.EncodeToString(sum[:])[:12]+".viewed")
	b, err := os.ReadFile(path)
	if f := strings.Split(strings.TrimSuffix(string(b), "\n"), "\t"); err != nil || len(f) != 3 || f[0] != "a.go" || f[1] != "111" || len(f[2]) != 13 {
		t.Errorf("viewed file %s: %q, %v", path, b, err)
	}
	got := r.viewed(map[string]string{"a.go": "111", "b.go": "222", "c.go": "333"})
	if !reflect.DeepEqual(got, map[string]string{"a.go": "111"}) {
		t.Errorf("viewed = %v, want only a.go", got)
	}
	if got := r.viewed(map[string]string{"a.go": "999"}); len(got) != 0 {
		t.Errorf("a changed file stayed viewed: %v", got)
	}
	os.WriteFile(path, []byte("old.go\t444\t1000\nnew.go\t555\n"+string(b)), 0o644)
	if got := r.viewed(map[string]string{"old.go": "444", "new.go": "555", "a.go": "111"}); !reflect.DeepEqual(got, map[string]string{"a.go": "111"}) {
		t.Errorf("expired and undated entries kept: %v", got)
	}
}

func TestFileHashes(t *testing.T) {
	const patch = "diff --git a/x.go b/x.go\nindex 1111..2222 100644\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git a/old.txt b/new.txt\nrename from old.txt\nrename to new.txt\n" +
		"diff --git a/run b/run\nold mode 100644\nnew mode 100755\n" +
		"diff --git a/y.go b/y.go\nindex 3333..4444\n"
	h := FileHashes(patch)
	rename := hashOf("diff --git a/old.txt b/new.txt\nrename from old.txt\nrename to new.txt")
	mode := hashOf("diff --git a/run b/run\nold mode 100644\nnew mode 100755")
	want := map[string]string{"x.go": "2222", "new.txt": rename, "run": mode, "y.go": "4444"}
	if !reflect.DeepEqual(h, want) {
		t.Errorf("FileHashes = %v, want %v", h, want)
	}
}

func TestDiffArgs(t *testing.T) {
	for _, tc := range []struct {
		vcs                 string
		o                   Options
		cmd                 string
		inline, ends        bool
		left, right         string
		expandErr, unparsed bool
	}{
		{vcs: "jj", o: Options{}, cmd: "jj diff --from 'fork_point(trunk() | @)'", inline: true, ends: true, left: "fork_point(trunk() | @)"},
		{vcs: "jj", o: Options{Revisions: "@"}, cmd: "jj diff -r @", inline: true, ends: true, left: "@-"},
		{vcs: "jj", o: Options{Revisions: "xyz"}, cmd: "jj diff -r xyz", ends: true, left: "xyz-", right: "xyz"},
		{vcs: "jj", o: Options{Revisions: "@-..@"}, cmd: "jj diff -r @-..@", inline: true, ends: true, left: "@-"},
		{vcs: "jj", o: Options{Revisions: "a..b"}, cmd: "jj diff -r a..b", ends: true, left: "a", right: "b"},
		{vcs: "jj", o: Options{Revisions: "a::b"}, cmd: "jj diff -r a::b", ends: true, left: "a::b-", right: "a::b"},
		{vcs: "jj", o: Options{Revisions: "..@"}, cmd: "jj diff -r ..@", inline: true},
		{vcs: "jj", o: Options{From: "main"}, cmd: "jj diff --from main", inline: true, ends: true, left: "main"},
		{vcs: "jj", o: Options{From: "root()", To: "@-"}, cmd: "jj diff --from 'root()' --to @-", ends: true, left: "root()", right: "@-"},
		{vcs: "jj", o: Options{To: "@"}, cmd: "jj diff --to @", inline: true, ends: true, left: "@"},
		{vcs: "jj", o: Options{Revisions: "@", Paths: []string{"a b", "c"}}, cmd: "jj diff -r @ -- 'a b' c", inline: true, ends: true, left: "@-"},
		{vcs: "git", o: Options{Revisions: "HEAD"}, cmd: "git diff HEAD", inline: true, ends: true, left: "HEAD"},
		{vcs: "git", o: Options{Revisions: "a..b"}, cmd: "git diff a..b", ends: true, left: "a", right: "b"},
		{vcs: "git", o: Options{Revisions: "a...b"}, cmd: "git diff a...b"},
		{vcs: "git", o: Options{From: "main"}, cmd: "git diff main", inline: true, ends: true, left: "main"},
		{vcs: "git", o: Options{To: "x"}, cmd: "git diff HEAD x", ends: true, left: "HEAD", right: "x"},
		{vcs: "git", o: Options{From: "it's", To: "x"}, cmd: `git diff 'it'\''s' x`, ends: true, left: "it's", right: "x"},
	} {
		r := &Repo{VCS: tc.vcs, paths: tc.o.Paths}
		if tc.vcs == "jj" {
			r.jjDiff(tc.o)
		} else if err := r.gitDiff(tc.o); err != nil {
			t.Errorf("%+v: %v", tc.o, err)
		}
		got := struct {
			cmd          string
			inline, ends bool
			left, right  string
		}{r.Command(), r.inline, r.ends, r.left, r.right}
		want := struct {
			cmd          string
			inline, ends bool
			left, right  string
		}{tc.cmd, tc.inline, tc.ends, tc.left, tc.right}
		if got != want {
			t.Errorf("%s %+v:\n got %+v\nwant %+v", tc.vcs, tc.o, got, want)
		}
	}
}

func TestSend(t *testing.T) {
	dir := t.TempDir()
	r := &Repo{Root: "/r", VCS: "jj", dir: dir, args: []string{"-r", "@"}, inline: true,
		send: "cat > sent.json; echo $SKEPSIS_ROOT > root"}
	c := Comment{File: "a.go", Side: "additions", Lines: [2]int{3, 4}, Text: "hm", Blob: "abc",
		Code: []CodeRun{{Lines: [2]int{2, 5}, Text: "x\ny\nz\nw"}}}
	if err := r.Send(c); err != nil {
		t.Fatal(err)
	}
	var got payload
	b, _ := os.ReadFile(filepath.Join(dir, "sent.json"))
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("payload %s: %v", b, err)
	}
	want := payload{Version: 1, Root: "/r", File: "a.go", Side: "additions", Lines: [2]int{3, 4}, Code: c.Code, Blob: &c.Blob, Text: "hm"}
	want.Diff.Command, want.Diff.ToWorkingCopy = "jj diff -r @", true
	want.Prompt = prompt(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("payload\n%+v\nwant\n%+v", got, want)
	}
	if !strings.Contains(string(b), `"blob":"abc"`) || !strings.HasSuffix(string(b), "}\n") {
		t.Errorf("payload text %q", b)
	}
	if root, _ := os.ReadFile(filepath.Join(dir, "root")); string(root) != "/r\n" {
		t.Errorf("SKEPSIS_ROOT = %q", root)
	}
	r.send = "cat > sent.json"
	r.Send(Comment{Text: "x"})
	b, _ = os.ReadFile(filepath.Join(dir, "sent.json"))
	if !strings.Contains(string(b), `"blob":null`) || !strings.Contains(string(b), `"code":null`) {
		t.Errorf("empty blob and code: %s", b)
	}
	for _, tc := range []struct{ send, want string }{
		{"echo nope >&2; exit 3", "nope"},
		{"exit 3", "--send command: exit status 3"},
		{"", "no --send command"},
	} {
		r.send = tc.send
		if err := r.Send(Comment{Text: "x"}); err == nil || err.Error() != tc.want {
			t.Errorf("send %q: %v, want %q", tc.send, err, tc.want)
		}
	}
}

func sh(t *testing.T, dir, script string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", script, err, out)
	}
}

func TestJJ(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("no jj")
	}
	dir := t.TempDir()
	sh(t, dir, `jj git init . && printf 'package a\n\nfunc A() {}\n' > a.go && printf 'x\n' > old.txt &&
		jj commit -m base && printf 'package a\n\nfunc A() { return }\n' > a.go && mv old.txt new.txt && mkdir sub`)
	r, err := Open(filepath.Join(dir, "sub"), Options{Revisions: "@"})
	if err != nil {
		t.Fatal(err)
	}
	if r.VCS != "jj" || !r.CommentsEnabled() || !r.Expandable() || r.Command() != "jj diff -r @" || r.Name() != filepath.Base(dir) {
		t.Errorf("Open = %s %q comments=%v expandable=%v", r.VCS, r.Command(), r.CommentsEnabled(), r.Expandable())
	}
	s, err := r.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.FileHashes["a.go"]; !ok || !strings.Contains(s.Patch, "rename to new.txt") || s.PatchHash != hashOf(s.Patch) {
		t.Errorf("patch:\n%s", s.Patch)
	}
	if *s.Syntaxes["a.go"] != slashC || s.Syntaxes["new.txt"] != nil {
		t.Errorf("syntaxes %v %v", s.Syntaxes["a.go"], s.Syntaxes["new.txt"])
	}
	old, new, err := r.Contents("a.go", "")
	if err != nil || old == nil || new == nil || !strings.Contains(*old, "A() {}") || !strings.Contains(*new, "return") {
		t.Errorf("Contents = %v, %v, %v", old, new, err)
	}
	if old, new, _ := r.Contents("new.txt", "old.txt"); old == nil || *old != "x\n" || new == nil || *new != "x\n" {
		t.Errorf("renamed = %v, %v", old, new)
	}
	if old, new, _ := r.Contents("missing.txt", ""); old != nil || new != nil {
		t.Errorf("missing file = %v, %v", old, new)
	}
	if h, files, err := r.Peek(); err != nil || h != s.PatchHash || !maps.Equal(files, s.FileHashes) {
		t.Errorf("Peek = %q, %v, %v, want %q, %v", h, files, err, s.PatchHash, s.FileHashes)
	}
}

func TestGit(t *testing.T) {
	dir := t.TempDir()
	sh(t, dir, `git init -q -b main . && git -c user.email=t@t -c user.name=t commit -q --allow-empty -m root &&
		printf 'a\n' > f.py && git add f.py && git -c user.email=t@t -c user.name=t commit -q -m base &&
		git checkout -q -b topic && printf 'b\n' > f.py && ln -s f.py link`)
	r, err := Open(dir, Options{Git: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Command(), "git diff --merge-base main") || !r.CommentsEnabled() {
		t.Errorf("default git diff %q, comments %v", r.Command(), r.CommentsEnabled())
	}
	s, err := r.Load()
	if err != nil || !strings.Contains(s.Patch, "+b") {
		t.Fatalf("Load: %v\n%v", err, s)
	}
	old, new, _ := r.Contents("f.py", "")
	if old == nil || *old != "a\n" || new == nil || *new != "b\n" {
		t.Errorf("Contents = %v, %v", old, new)
	}
	if _, new, _ := r.Contents("link", ""); new == nil || *new != "f.py" {
		t.Errorf("symlink contents = %v, want the target path", new)
	}
	if r, _ := Open(dir, Options{Git: true, Revisions: "HEAD~1..HEAD"}); r.CommentsEnabled() || r.right != "HEAD" {
		t.Errorf("git range: right %q comments %v", r.right, r.CommentsEnabled())
	}
	if _, _, err := (&Repo{}).Contents("x", ""); err == nil {
		t.Error("Contents without endpoints succeeded")
	}
}

func TestMessages(t *testing.T) {
	jj := t.TempDir()
	if _, err := exec.LookPath("jj"); err == nil {
		sh(t, jj, `jj git init . && printf 'a\n' > a.go && jj commit -m 'first change' &&
			printf 'b\n' > a.go && jj describe -m 'second change

with a body'`)
	}
	git := t.TempDir()
	sh(t, git, `git init -q -b main . && printf 'a\n' > a.go && git add a.go && git -c user.email=t@t -c user.name=t commit -q -m one &&
		printf 'b\n' > a.go && git -c user.email=t@t -c user.name=t commit -q -am 'two

with a body'`)
	for _, tc := range []struct {
		name, dir string
		o         Options
		want      []string // first lines of the messages shown
		single    bool     // shown as the one MessagePath
	}{
		{"jj one", jj, Options{Revisions: "@"}, []string{"second change"}, true},
		{"jj parent", jj, Options{Revisions: "@-"}, []string{"first change"}, true},
		{"jj range", jj, Options{From: "root()"}, []string{"first change", "second change"}, false},
		{"jj default diff", jj, Options{}, nil, false},
		{"git from", git, Options{Git: true, From: "HEAD~1"}, []string{"two"}, true},
		{"git range", git, Options{Git: true, Revisions: "HEAD~1..HEAD"}, []string{"two"}, true},
		{"git empty range", git, Options{Git: true, Revisions: "HEAD"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := exec.LookPath("jj"); err != nil && tc.dir == jj {
				t.Skip("no jj")
			}
			r, err := Open(tc.dir, tc.o)
			if err != nil {
				t.Fatal(err)
			}
			s, err := r.Load()
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for name := range s.Pseudo {
				if _, ok := s.FileHashes[name]; !ok || !strings.HasPrefix(name, MessagePath) || (name == MessagePath) != tc.single {
					t.Errorf("pseudo-file %q (single %v, hashed %v)", name, tc.single, ok)
				}
				_, text, _ := r.Contents(name, "")
				got = append(got, strings.SplitN(*text, "\n", 2)[0])
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("messages = %q, want %q", got, tc.want)
			}
			if tc.single && !strings.HasPrefix(s.Patch, "diff --git a/"+MessagePath+" b/"+MessagePath+"\nnew file mode 100644\n--- /dev/null\n+++ b/"+MessagePath+"\n@@ -0,0 +1,") {
				t.Errorf("patch does not lead with the message:\n%s", s.Patch)
			}
		})
	}
	// A message file in the working copy, as the review skill writes, wins.
	sh(t, git, `mkdir .review && printf 'draft\n' > .review/COMMIT_MSG && git add .review`)
	r, _ := Open(git, Options{Git: true, From: "HEAD~1"})
	s, _ := r.Load()
	if len(s.Pseudo) != 0 || strings.Count(s.Patch, "b/"+MessagePath+"\n") != 2 || !strings.Contains(s.Patch, "+draft") {
		t.Errorf("real message file: pseudo %v, patch:\n%s", s.Pseudo, s.Patch)
	}
}
