package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/msuozzo/jj-highlight/syntax"
)

type Theme struct {
	Dark bool

	Bg, Panel, Raised         tcell.Color
	Fg, Muted, Subtle, Faint  tcell.Color
	Border                    tcell.Color
	Accent                    tcell.Color
	Green, Red, Amber, Purple tcell.Color

	AddBg, AddNum, AddWord tcell.Color
	DelBg, DelNum, DelWord tcell.Color
	HunkBg, HunkFg         tcell.Color
	FileBg                 tcell.Color
	CardBg                 tcell.Color
	Match, MatchCur        tcell.Color
	Thumb, Track           tcell.Color

	toks map[syntax.Class]tcell.Style
}

func hex(s string) tcell.Color { return tcell.GetColor(s) }

func mix(a, b tcell.Color, t float64) tcell.Color {
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	f := func(x, y int32) int32 { return x + int32(float64(y-x)*t+0.5) }
	return tcell.NewRGBColor(f(ar, br), f(ag, bg), f(ab, bb))
}

func darkTheme() *Theme {
	bg := hex("#0d1117")
	t := &Theme{
		Dark: true, Bg: bg, Panel: hex("#010409"), Raised: hex("#161b22"),
		Fg: hex("#e6edf3"), Muted: hex("#9198a1"), Subtle: hex("#6e7681"), Faint: hex("#3d444d"),
		Border: hex("#30363d"), Accent: hex("#4493f8"),
		Green: hex("#3fb950"), Red: hex("#f85149"), Amber: hex("#d29922"), Purple: hex("#ab7df8"),
	}
	t.AddBg, t.AddNum, t.AddWord = mix(bg, hex("#2ea043"), 0.15), mix(bg, hex("#3fb950"), 0.30), mix(bg, hex("#2ea043"), 0.40)
	t.DelBg, t.DelNum, t.DelWord = mix(bg, hex("#f85149"), 0.12), mix(bg, hex("#f85149"), 0.30), mix(bg, hex("#f85149"), 0.40)
	t.HunkBg, t.HunkFg = mix(bg, hex("#388bfd"), 0.10), t.Muted
	t.FileBg = t.Raised
	t.CardBg = mix(bg, t.Raised, 0.7)
	t.Match, t.MatchCur = hex("#ffe100"), hex("#ff8c1a")
	t.Thumb, t.Track = hex("#3d444d"), t.Panel
	t.syntaxColors("#ff7b72", "#a5d6ff", "#79c0ff", "#8b949e", "#ffa657", "#d2a8ff")
	return t
}

func lightTheme() *Theme {
	bg := hex("#ffffff")
	t := &Theme{
		Bg: bg, Panel: hex("#f6f8fa"), Raised: hex("#f6f8fa"),
		Fg: hex("#1f2328"), Muted: hex("#59636e"), Subtle: hex("#818b98"), Faint: hex("#d1d9e0"),
		Border: hex("#d1d9e0"), Accent: hex("#0969da"),
		Green: hex("#1a7f37"), Red: hex("#d1242f"), Amber: hex("#9a6700"), Purple: hex("#8250df"),
	}
	t.AddBg, t.AddNum, t.AddWord = hex("#dafbe1"), hex("#aceebb"), hex("#aceebb")
	t.DelBg, t.DelNum, t.DelWord = hex("#ffebe9"), hex("#ffcecb"), hex("#ffcecb")
	t.HunkBg, t.HunkFg = hex("#ddf4ff"), t.Muted
	t.FileBg = t.Raised
	t.CardBg = hex("#fbfcfd")
	t.Match, t.MatchCur = hex("#ffe100"), hex("#ff8c1a")
	t.Thumb, t.Track = hex("#c8d1da"), t.Panel
	t.syntaxColors("#cf222e", "#0a3069", "#0550ae", "#6e7781", "#953800", "#8250df")
	return t
}

func (t *Theme) st(fg, bg tcell.Color) tcell.Style {
	return tcell.StyleDefault.Foreground(fg).Background(bg)
}

// syntaxColors sets the token styles from GitHub's syntax palette.
func (t *Theme) syntaxColors(keyword, str, number, comment, typ, field string) {
	fg := tcell.StyleDefault.Foreground
	t.toks = map[syntax.Class]tcell.Style{
		syntax.Keyword:  fg(hex(keyword)),
		syntax.String:   fg(hex(str)),
		syntax.Number:   fg(hex(number)),
		syntax.Comment:  fg(hex(comment)).Italic(true),
		syntax.Type:     fg(hex(typ)),
		syntax.Field:    fg(hex(field)),
		syntax.Emphasis: fg(t.Fg).Italic(true),
		syntax.Strong:   fg(t.Fg).Bold(true),
		syntax.Conflict: fg(t.Muted),
		syntax.Todo:     fg(t.Fg).Bold(true),
		syntax.Fixme:    fg(t.Amber).Bold(true),
	}
}

// tok returns the foreground style for a token class, without a background.
func (t *Theme) tok(c syntax.Class) tcell.Style {
	if s, ok := t.toks[c]; ok {
		return s
	}
	return tcell.StyleDefault.Foreground(t.Fg)
}
