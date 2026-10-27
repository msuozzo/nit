package main

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestKeyName(t *testing.T) {
	for _, tc := range []struct {
		k    tcell.Key
		r    rune
		mod  tcell.ModMask
		want string
	}{
		{tcell.KeyRune, 'j', 0, "j"},
		{tcell.KeyRune, ' ', 0, " "},
		{tcell.KeyRune, 'G', tcell.ModShift, "G"},
		{tcell.KeyRune, 'd', tcell.ModCtrl, "C-d"},
		{tcell.KeyCtrlD, 0, tcell.ModCtrl, "C-d"},
		{tcell.KeyCtrlD, 0, tcell.ModCtrl | tcell.ModShift, "C-d"},
		{tcell.KeyCtrlD, 0, tcell.ModCtrl | tcell.ModAlt, "C-M-d"},
		{tcell.KeyRune, 'b', tcell.ModAlt, "M-b"},
		{tcell.KeyRune, 'x', tcell.ModCtrl | tcell.ModAlt, "C-M-x"},
		{tcell.KeyEnter, 0, 0, "enter"},
		{tcell.KeyEnter, 0, tcell.ModCtrl, "C-enter"},
		{tcell.KeyEnter, 0, tcell.ModCtrl | tcell.ModAlt, "C-M-enter"},
		{tcell.KeyEnter, 0, tcell.ModShift, "S-enter"},
		{tcell.KeyTab, 0, 0, "tab"},
		{tcell.KeyBacktab, 0, 0, "S-tab"},
		{tcell.KeyBacktab, 0, tcell.ModShift, "S-tab"},
		{tcell.KeyBackspace, 0, 0, "bs"},
		{tcell.KeyBackspace2, 0, tcell.ModAlt, "M-bs"},
		{tcell.KeyEsc, 0, 0, "esc"},
		{tcell.KeyPgDn, 0, 0, "pgdn"},
		{tcell.KeyLeft, 0, tcell.ModCtrl, "C-left"},
		{tcell.KeyF1, 0, 0, "F1"},
	} {
		if got := keyName(tcell.NewEventKey(tc.k, tc.r, tc.mod)); got != tc.want {
			t.Errorf("keyName(%v, %q, %v) = %q, want %q", tc.k, tc.r, tc.mod, got, tc.want)
		}
	}
}

func TestPlainRune(t *testing.T) {
	for _, tc := range []struct {
		k    tcell.Key
		r    rune
		mod  tcell.ModMask
		want rune
		ok   bool
	}{
		{tcell.KeyRune, 'a', 0, 'a', true},
		{tcell.KeyRune, 'A', tcell.ModShift, 'A', true},
		{tcell.KeyRune, 'a', tcell.ModCtrl, 0, false},
		{tcell.KeyRune, 'a', tcell.ModAlt, 0, false},
		{tcell.KeyEnter, 0, 0, 0, false},
	} {
		if r, ok := plainRune(tcell.NewEventKey(tc.k, tc.r, tc.mod)); r != tc.want || ok != tc.ok {
			t.Errorf("plainRune(%v, %q, %v) = %q, %v", tc.k, tc.r, tc.mod, r, ok)
		}
	}
}

func TestAsciiLower(t *testing.T) {
	for in, want := range map[string]string{"AbC": "abc", "É x": "É x", "": ""} {
		if got := asciiLower(in); got != want || len(got) != len(in) {
			t.Errorf("asciiLower(%q) = %q, want %q", in, got, want)
		}
	}
}
