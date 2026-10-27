package main

import (
	"cmp"
	"slices"
	"strings"
)

type TreeNode struct {
	Name   string
	Dir    bool
	File   *File
	Kids   []*TreeNode
	Closed bool
}

type TreeRow struct {
	N     *TreeNode
	Depth int
}

// buildTree arranges files the way skepsis lists them: directories before
// files at each level, single-child directory chains joined into one row.
func buildTree(files []*File) *TreeNode {
	root := &TreeNode{Dir: true}
	for _, f := range files {
		dir, base := splitPath(f.Path)
		n := root
		for _, p := range dir {
			i := slices.IndexFunc(n.Kids, func(k *TreeNode) bool { return k.Dir && k.Name == p })
			if i < 0 {
				i = len(n.Kids)
				n.Kids = append(n.Kids, &TreeNode{Name: p, Dir: true})
			}
			n = n.Kids[i]
		}
		n.Kids = append(n.Kids, &TreeNode{Name: base, File: f})
	}
	var fix func(n *TreeNode)
	fix = func(n *TreeNode) {
		for n != root && len(n.Kids) == 1 && n.Kids[0].Dir {
			n.Name += "/" + n.Kids[0].Name
			n.Kids = n.Kids[0].Kids
		}
		slices.SortStableFunc(n.Kids, func(a, b *TreeNode) int {
			return cmp.Or(cmp.Compare(b2i(b.Dir), b2i(a.Dir)), strings.Compare(a.Name, b.Name))
		})
		for _, k := range n.Kids {
			fix(k)
		}
	}
	fix(root)
	return root
}

func splitPath(p string) (dir []string, base string) {
	parts := strings.Split(p, "/")
	return parts[:len(parts)-1], parts[len(parts)-1]
}

// files lists the tree's files in display order.
func (n *TreeNode) files(out []*File) []*File {
	if n.File != nil {
		return append(out, n.File)
	}
	for _, k := range n.Kids {
		out = k.files(out)
	}
	return out
}

// flatten lists the visible rows of the tree with their depth.
func flatten(root *TreeNode) []TreeRow {
	var rows []TreeRow
	var walk func(n *TreeNode, d int)
	walk = func(n *TreeNode, d int) {
		for _, k := range n.Kids {
			rows = append(rows, TreeRow{k, d})
			if k.Dir && !k.Closed {
				walk(k, d+1)
			}
		}
	}
	walk(root, 0)
	return rows
}
