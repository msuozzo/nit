package main

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
)

func TestBuildTree(t *testing.T) {
	var files []*File
	for _, p := range []string{"a/b/c.go", "a/b/d.go", "a/x.go", "z.go", "README", "deep/one/two/f.go"} {
		files = append(files, &File{Path: p})
	}
	root := buildTree(files)
	rows := func() []string {
		var out []string
		for _, r := range flatten(root) {
			name := r.N.Name
			if r.N.Dir {
				name += "/"
			}
			out = append(out, strconv.Itoa(r.Depth)+" "+name)
		}
		return out
	}
	want := []string{"0 a/", "1 b/", "2 c.go", "2 d.go", "1 x.go", "0 deep/one/two/", "1 f.go", "0 README", "0 z.go"}
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("flatten = %q, want %q", got, want)
	}
	var paths []string
	for _, f := range root.files(nil) {
		paths = append(paths, f.Path)
	}
	if want := []string{"a/b/c.go", "a/b/d.go", "a/x.go", "deep/one/two/f.go", "README", "z.go"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("files = %q, want %q", paths, want)
	}
	root.Kids[0].Closed = true
	if got := rows(); !reflect.DeepEqual(got, []string{"0 a/", "0 deep/one/two/", "1 f.go", "0 README", "0 z.go"}) {
		t.Errorf("closed dir rows = %q", got)
	}
	if n := buildTree(nil); len(n.Kids) != 0 || len(flatten(n)) != 0 {
		t.Errorf("empty tree = %+v", n)
	}
}

func TestNameOrder(t *testing.T) {
	for _, tc := range []struct{ first, second string }{
		{"form.go", "form_test.go"},
		{"form.go", "form.txt"},
		{".review", "a"},
		{"COMMIT_MSG", "COMMIT_MSG.abc"},
		{"main.go", "Makefile"},
		{"README", "readme.md"},
		{"a_b.go", "ab.go"},
		{"package.gohtml", "packages.gohtml"},
	} {
		if nameOrder(tc.first, tc.second) >= 0 || nameOrder(tc.second, tc.first) <= 0 {
			t.Errorf("%q should sort before %q", tc.first, tc.second)
		}
	}
	var got []string
	for _, r := range flatten(buildTree([]*File{{Path: "x/form_test.go"}, {Path: "x/form.go"}, {Path: "a.go"}, {Path: ".review/COMMIT_MSG"}})) {
		got = append(got, r.N.Name)
	}
	if want := []string{".review", "COMMIT_MSG", "x", "form.go", "form_test.go", "a.go"}; !slices.Equal(got, want) {
		t.Errorf("tree order = %q, want %q", got, want)
	}
}
