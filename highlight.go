package main

import (
	"strings"

	"github.com/msuozzo/jj-highlight/syntax"
)

// highlightLines highlights lines as one text and splits the tokens back out
// per line. A token spanning lines contributes a segment to each.
func highlightLines(path string, lines []string) [][]Seg {
	segs := make([][]Seg, len(lines))
	if len(lines) == 0 || !syntax.Supported(path) {
		return segs
	}
	toks, _ := syntax.Classify(path, []byte(strings.Join(lines, "\n")+"\n"))
	off, t := 0, 0
	for i, l := range lines {
		end := off + len(l)
		for t < len(toks) && int(toks[t].End) <= off {
			t++
		}
		pos := off
		for k := t; k < len(toks) && int(toks[k].Start) < end; k++ {
			s, e := max(int(toks[k].Start), off), min(int(toks[k].End), end)
			if s > pos {
				segs[i] = append(segs[i], Seg{End: s - off})
			}
			if e > s {
				segs[i], pos = append(segs[i], Seg{End: e - off, Class: toks[k].Class}), e
			}
		}
		off = end + 1
	}
	return segs
}

// fragmentHL highlights the lines the hunks show, side by side, until the
// full contents arrive.
func fragmentHL(f *File) [2]map[int][]Seg {
	var res [2]map[int][]Seg
	for side, p := range [2]string{f.OldPath, f.Path} {
		var lines []string
		var nums []int
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if n := l.num(side); n > 0 {
					lines, nums = append(lines, l.Text), append(nums, n)
				}
			}
		}
		res[side] = map[int][]Seg{}
		for i, s := range highlightLines(p, lines) {
			res[side][nums[i]] = s
		}
	}
	return res
}

// fullHL highlights whole file contents, keyed by 1-based line number.
func fullHL(f *File, old, new []string) [2]map[int][]Seg {
	var res [2]map[int][]Seg
	for side, lines := range [2][]string{old, new} {
		res[side] = make(map[int][]Seg, len(lines))
		for i, s := range highlightLines([2]string{f.OldPath, f.Path}[side], lines) {
			res[side][i+1] = s
		}
	}
	return res
}
