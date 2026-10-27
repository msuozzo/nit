package main

import (
	"slices"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestChangedFiles(t *testing.T) {
	old := map[string]string{"a.go": "1", "b.go": "2", "gone.go": "3"}
	for _, tc := range []struct {
		new  map[string]string
		want []string
	}{
		{old, nil},
		{map[string]string{"a.go": "1", "b.go": "9", "gone.go": "3"}, []string{"b.go"}},
		{map[string]string{"a.go": "1", "b.go": "2", "gone.go": "3", "new.go": "4"}, []string{"new.go"}},
		{map[string]string{"b.go": "2", "a.go": "5"}, []string{"a.go", "gone.go"}},
	} {
		if got := changedFiles(old, tc.new); !slices.Equal(got, tc.want) {
			t.Errorf("changedFiles(%v) = %v, want %v", tc.new, got, tc.want)
		}
	}
}

func TestFollow(t *testing.T) {
	rows := []Row{{Kind: RowFile}, {Kind: RowLine}, {Kind: RowFile}}
	for len(rows) < 60 {
		rows = append(rows, Row{Kind: RowLine})
	}
	rows[22].Kind = RowFile
	for _, tc := range []struct{ top, cur, want int }{
		{0, 2, 0},
		{20, 2, 2},
		{20, 22, 18},
		{20, 21, 17},
	} {
		a := &App{lay: &Layout{Rows: rows}, ph: 30, top: tc.top, cur: tc.cur}
		if a.follow(); a.top != tc.want {
			t.Errorf("follow from top %d to row %d: top = %d, want %d", tc.top, tc.cur, a.top, tc.want)
		}
	}
}

func TestTreeToggle(t *testing.T) {
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(110, 40)
	a := &App{s: sim, lay: &Layout{Rows: []Row{{Kind: RowFile}}}, showTree: true, treeW: 34}
	a.geom()
	for i, want := range []int{0, 34, 0, 34} {
		if i > 0 {
			a.normalKey("b")
		}
		if a.tw != want {
			t.Errorf("after %d b presses at 110 columns: tree width %d, want %d", i, a.tw, want)
		}
	}
}
