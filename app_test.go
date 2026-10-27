package main

import (
	"slices"
	"testing"
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
