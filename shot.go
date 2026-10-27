package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/msuozzo/nit/review"
)

// runShot drives the app on a simulated screen with a script of input
// events and renders the result to PNG, so the UI can be checked without a
// terminal. Tokens: keys (j, C-d, enter, C-enter), wait:MS, click:X,Y,
// press:X,Y, move:X,Y, release:X,Y, hover:X,Y, wheel:up|down:X,Y:N,
// type:TEXT (URL-escaped), shot:FILE.
func runShot(repo *review.Repo, th *Theme, out, size, script string) error {
	var w, h int
	fmt.Sscanf(size, "%dx%d", &w, &h)
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		return err
	}
	sim.SetSize(w, h)
	a := newApp(sim, repo, th)
	done := make(chan error, 1)
	ready := func() bool {
		ch := make(chan bool)
		a.post(func() { ch <- a.lay != nil })
		return <-ch
	}
	snap := func(path string) error {
		ch := make(chan error)
		a.post(func() {
			a.draw()
			ch <- renderPNG(sim, path)
		})
		return <-ch
	}
	go func() {
		for !ready() {
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(600 * time.Millisecond)
		for _, t := range strings.Fields(script) {
			if p, ok := strings.CutPrefix(t, "shot:"); ok {
				if err := snap(p); err != nil {
					done <- err
					return
				}
				continue
			}
			inject(sim, t)
			time.Sleep(60 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond)
		err := snap(out)
		a.post(func() { a.quit = true })
		done <- err
	}()
	a.run()
	return <-done
}

var shotKeys = map[string]tcell.Key{
	"enter": tcell.KeyEnter, "esc": tcell.KeyEsc, "tab": tcell.KeyTab, "bs": tcell.KeyBackspace2,
	"up": tcell.KeyUp, "down": tcell.KeyDown, "left": tcell.KeyLeft, "right": tcell.KeyRight,
	"pgdn": tcell.KeyPgDn, "pgup": tcell.KeyPgUp, "home": tcell.KeyHome, "end": tcell.KeyEnd,
}

// inject replays one script token on the simulated screen.
func inject(sim tcell.SimulationScreen, t string) {
	name, arg, _ := strings.Cut(t, ":")
	xy := func(s string) (x, y int) {
		fmt.Sscanf(s, "%d,%d", &x, &y)
		return x, y
	}
	switch name {
	case "wait":
		var ms int
		fmt.Sscanf(arg, "%d", &ms)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	case "click":
		x, y := xy(arg)
		sim.InjectMouse(x, y, tcell.Button1, 0)
		time.Sleep(20 * time.Millisecond)
		sim.InjectMouse(x, y, tcell.ButtonNone, 0)
	case "press", "move":
		x, y := xy(arg)
		sim.InjectMouse(x, y, tcell.Button1, 0)
	case "release", "hover":
		x, y := xy(arg)
		sim.InjectMouse(x, y, tcell.ButtonNone, 0)
	case "wheel":
		dir, rest, _ := strings.Cut(arg, ":")
		pos, count, _ := strings.Cut(rest, ":")
		x, y := xy(pos)
		n := 1
		if count != "" {
			fmt.Sscanf(count, "%d", &n)
		}
		b := tcell.WheelDown
		if dir == "up" {
			b = tcell.WheelUp
		}
		for range n {
			sim.InjectMouse(x, y, b, 0)
			time.Sleep(8 * time.Millisecond)
		}
	case "type":
		s, _ := url.PathUnescape(arg)
		for _, r := range s {
			sim.InjectKey(tcell.KeyRune, r, 0)
		}
	default:
		injectKey(sim, t)
	}
}

var shotMods = map[byte]tcell.ModMask{'C': tcell.ModCtrl, 'M': tcell.ModAlt, 'S': tcell.ModShift}

// injectKey presses a key named like keyName spells it, such as C-M-enter.
func injectKey(sim tcell.SimulationScreen, t string) {
	mod := tcell.ModNone
	for len(t) > 2 && t[1] == '-' && shotMods[t[0]] != 0 {
		mod |= shotMods[t[0]]
		t = t[2:]
	}
	switch rs := []rune(t); {
	case t == "space":
		sim.InjectKey(tcell.KeyRune, ' ', mod)
	case shotKeys[t] != 0:
		sim.InjectKey(shotKeys[t], 0, mod)
	case len(rs) == 1:
		sim.InjectKey(tcell.KeyRune, rs[0], mod)
	}
}

type faceSet struct {
	faces []font.Face
	fonts []*sfnt.Font
}

var shotFaces [4]*faceSet // regular, bold, italic, bold italic

func loadFaces() error {
	if shotFaces[0] != nil {
		return nil
	}
	data, err := os.ReadFile("/System/Library/Fonts/Menlo.ttc")
	if err != nil {
		return err
	}
	coll, err := opentype.ParseCollection(data)
	if err != nil {
		return err
	}
	var fallbacks []*sfnt.Font
	for _, p := range []string{"/System/Library/Fonts/Apple Symbols.ttf", "/Library/Fonts/Arial Unicode.ttf"} {
		if d, err := os.ReadFile(p); err == nil {
			if f, err := opentype.Parse(d); err == nil {
				fallbacks = append(fallbacks, f)
			}
		}
	}
	for i := range shotFaces {
		f, err := coll.Font(i)
		if err != nil {
			return err
		}
		fs := &faceSet{}
		for _, g := range append([]*sfnt.Font{f}, fallbacks...) {
			face, err := opentype.NewFace(g, &opentype.FaceOptions{Size: 13, DPI: 72, Hinting: font.HintingFull})
			if err != nil {
				return err
			}
			fs.faces, fs.fonts = append(fs.faces, face), append(fs.fonts, g)
		}
		shotFaces[i] = fs
	}
	return nil
}

// pick returns the first face with a glyph for r, else the main face.
func (fs *faceSet) pick(r rune) font.Face {
	var buf sfnt.Buffer
	for i, f := range fs.fonts {
		if idx, err := f.GlyphIndex(&buf, r); err == nil && idx != 0 {
			return fs.faces[i]
		}
	}
	return fs.faces[0]
}

func renderPNG(sim tcell.SimulationScreen, path string) error {
	if err := loadFaces(); err != nil {
		return err
	}
	cells, w, h := sim.GetContents()
	const cw, ch, asc = 8, 17, 13
	img := image.NewRGBA(image.Rect(0, 0, w*cw, h*ch))
	rgb := func(c tcell.Color, def color.RGBA) color.RGBA {
		if !c.Valid() || c == tcell.ColorDefault {
			return def
		}
		r, g, b := c.RGB()
		return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
	}
	cx, cy, cvis := sim.GetCursor()
	for y := range h {
		for x := range w {
			cell := cells[y*w+x]
			fg, bg, attr := cell.Style.Decompose()
			fgc, bgc := rgb(fg, color.RGBA{230, 237, 243, 255}), rgb(bg, color.RGBA{13, 17, 23, 255})
			rect := image.Rect(x*cw, y*ch, (x+1)*cw, (y+1)*ch)
			draw.Draw(img, rect, image.NewUniform(bgc), image.Point{}, draw.Src)
			if len(cell.Runes) == 0 || cell.Runes[0] == ' ' {
				continue
			}
			r := cell.Runes[0]
			if blockGlyph(img, rect, r, fgc) {
				continue
			}
			fi := b2i(attr&tcell.AttrBold != 0) | 2*b2i(attr&tcell.AttrItalic != 0)
			d := font.Drawer{Dst: img, Src: image.NewUniform(fgc), Face: shotFaces[fi].pick(r), Dot: fixed.P(x*cw, y*ch+asc)}
			d.DrawString(string(r))
		}
	}
	if cvis {
		draw.Draw(img, image.Rect(cx*cw, cy*ch, cx*cw+2, (cy+1)*ch), image.NewUniform(color.RGBA{255, 255, 255, 255}), image.Point{}, draw.Src)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// blockGlyph draws block elements and box lines as rectangles, the way
// terminals render them, so they join up between cells.
func blockGlyph(img *image.RGBA, r image.Rectangle, ch rune, c color.RGBA) bool {
	w, h := r.Dx(), r.Dy()
	mx, my := r.Min.X+w/2, r.Min.Y+h/2
	var rs []image.Rectangle
	switch ch {
	case '▎':
		rs = append(rs, image.Rect(r.Min.X, r.Min.Y, r.Min.X+w/4+1, r.Max.Y))
	case '▐':
		rs = append(rs, image.Rect(mx, r.Min.Y, r.Max.X, r.Max.Y))
	case '│':
		rs = append(rs, image.Rect(mx, r.Min.Y, mx+1, r.Max.Y))
	case '─':
		rs = append(rs, image.Rect(r.Min.X, my, r.Max.X, my+1))
	case '━':
		rs = append(rs, image.Rect(r.Min.X, my-1, r.Max.X, my+1))
	case '╭':
		rs = append(rs, image.Rect(mx, my, r.Max.X, my+1), image.Rect(mx, my, mx+1, r.Max.Y))
	case '╮':
		rs = append(rs, image.Rect(r.Min.X, my, mx+1, my+1), image.Rect(mx, my, mx+1, r.Max.Y))
	case '╰':
		rs = append(rs, image.Rect(mx, my, r.Max.X, my+1), image.Rect(mx, r.Min.Y, mx+1, my+1))
	case '╯':
		rs = append(rs, image.Rect(r.Min.X, my, mx+1, my+1), image.Rect(mx, r.Min.Y, mx+1, my+1))
	default:
		return false
	}
	for _, x := range rs {
		draw.Draw(img, x, image.NewUniform(c), image.Point{}, draw.Src)
	}
	return true
}
