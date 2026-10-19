// Command panel demonstrates host-owned input and stacking over Berad Text.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"unicode/utf8"

	text "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

type game struct {
	renderer                         *text.Renderer
	source, icons                    *text.Source
	font                             *text.Font
	panel                            text.Panel
	log                              *text.Log
	heading, field                   *text.Layout
	rows                             []*text.Layout
	palette                          text.Palette
	input                            string
	caret                            int
	width, height                    int
	size                             float64
	dirty, alternate, cover, focused bool
	err                              error
}

func (g *game) record(value string) error {
	content, err := text.NewContent([]text.Span{{Text: value, Font: g.font}})
	if err != nil {
		return err
	}
	if _, err = g.log.Append(content); err != nil {
		return err
	}
	g.dirty = true
	return nil
}

func (g *game) rebuild() error {
	font, err := g.source.NewFont(text.FontOptions{Size: g.size})
	if err != nil {
		return err
	}
	g.font = font
	icon, err := g.icons.NewFont(text.FontOptions{Size: g.size})
	if err != nil {
		return err
	}
	content, err := text.NewContent([]text.Span{
		{Text: "Reactor ", Font: font, Paint: "label"},
		{Text: "\ufffc", Font: icon, Paint: "accent", Tag: "reactor"},
		{Text: " online. Click the icon or edit the field.\nSpace: cover   T: theme   +/-: size\nResize the window to reflow.", Font: font, Paint: "label"},
	})
	if err != nil {
		return err
	}
	g.panel.Set(content)
	g.heading, err = text.NewLayout(content, text.LayoutOptions{Width: float64(g.width - 48)})
	if err != nil {
		return err
	}
	field, err := text.NewContent([]text.Span{{Text: g.input, Font: font, Paint: "accent"}})
	if err != nil {
		return err
	}
	g.field, err = text.NewLayout(field, text.LayoutOptions{})
	if err != nil {
		return err
	}
	g.rows = nil
	for _, entry := range g.log.Entries() {
		layout, layoutErr := text.NewLayout(entry, text.LayoutOptions{Width: float64(g.width - 48)})
		if layoutErr != nil {
			return layoutErr
		}
		g.rows = append(g.rows, layout)
	}
	g.updatePalette()
	g.dirty = false
	return nil
}

func (g *game) updatePalette() {
	g.palette = text.Palette{"label": color.NRGBA{R: 220, G: 225, B: 235, A: 255}, "accent": color.NRGBA{R: 80, G: 210, B: 255, A: 255}}
	if g.alternate {
		g.palette["accent"] = color.NRGBA{R: 255, G: 190, B: 70, A: 255}
	}
}

func (g *game) fieldBounds() image.Rectangle {
	y := 24 + int(g.heading.Bounds().Height()) + 20
	return image.Rect(24, y, g.width-24, y+int(g.field.Bounds().Height())+12)
}

func (g *game) blocker() image.Rectangle {
	if !g.cover {
		return image.Rectangle{}
	}
	return image.Rect(100, 16, min(g.width-8, 320), 120)
}

// A geometric candidate is consulted only after host stacking/clip eligibility.
// Blank pixels of an opaque covering panel block just as its visible controls do.
func annotationAt(layout *text.Layout, screen image.Point, origin image.Point, blocker image.Rectangle) string {
	if screen.In(blocker) {
		return ""
	}
	for _, hit := range layout.HitTestLocal(text.Point{X: float64(screen.X - origin.X), Y: float64(screen.Y - origin.Y)}) {
		if len(hit.Tags) > 0 {
			return hit.Tags[0]
		}
	}
	return ""
}

func (g *game) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.dirty {
		if err := g.rebuild(); err != nil {
			return err
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		point := image.Pt(x, y)
		if !point.In(g.blocker()) {
			g.focused = point.In(g.fieldBounds())
			if g.focused {
				bounds := g.fieldBounds()
				site, err := g.field.NearestCaret(text.Point{X: float64(x - bounds.Min.X), Y: float64(y - bounds.Min.Y - 6)})
				if err != nil {
					return err
				}
				g.caret = site.Offset
			} else if tag := annotationAt(g.heading, point, image.Pt(24, 24), g.blocker()); tag != "" {
				if err := g.record("Host activated: " + tag); err != nil {
					return err
				}
			}
		}
	}
	if g.focused {
		typed := string(ebiten.AppendInputChars(nil))
		if typed != "" && len(g.input)+len(typed) <= 128 {
			g.input = g.input[:g.caret] + typed + g.input[g.caret:]
			g.caret += len(typed)
			g.dirty = true
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && g.caret > 0 {
			_, size := utf8.DecodeLastRuneInString(g.input[:g.caret])
			g.input = g.input[:g.caret-size] + g.input[g.caret:]
			g.caret -= size
			g.dirty = true
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			g.focused = false
			if err := g.record("Host submitted: " + g.input); err != nil {
				return err
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.focused = false
		}
	} else {
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			g.cover = !g.cover
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyT) {
			g.alternate = !g.alternate
			g.updatePalette() // Paint changes retain the existing layout objects.
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
			g.size = min(32, g.size+2)
			g.dirty = true
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
			g.size = max(12, g.size-2)
			g.dirty = true
		}
	}
	if g.dirty {
		return g.rebuild()
	}
	return nil
}

func (g *game) draw(target *ebiten.Image, layout *text.Layout, x, y float64) {
	options := text.DrawOptions{}
	options.GeoM.Translate(x, y)
	if err := g.renderer.Draw(target, layout, g.palette, options); err != nil {
		g.err = err
	}
}

func (g *game) Draw(target *ebiten.Image) {
	target.Fill(color.RGBA{R: 20, G: 24, B: 32, A: 255})
	if g.heading == nil {
		return
	}
	g.draw(target, g.heading, 24, 24)
	bounds := g.fieldBounds()
	vector.StrokeRect(target, float32(bounds.Min.X), float32(bounds.Min.Y), float32(bounds.Dx()), float32(bounds.Dy()), 1, color.White, false)
	g.draw(target, g.field, float64(bounds.Min.X), float64(bounds.Min.Y+6))
	if g.focused {
		caret, err := g.field.CaretAt(g.caret, false)
		if err != nil {
			g.err = err
		} else {
			vector.FillRect(target, float32(bounds.Min.X)+float32(caret.Position.X), float32(bounds.Min.Y+6), 1, float32(caret.Height), color.White, false)
		}
	}
	y := float64(bounds.Max.Y + 20)
	for _, row := range g.rows {
		g.draw(target, row, 24, y)
		y += row.Bounds().Height() + 4
	}
	if blocker := g.blocker(); !blocker.Empty() {
		vector.FillRect(target, float32(blocker.Min.X), float32(blocker.Min.Y), float32(blocker.Dx()), float32(blocker.Dy()), color.RGBA{R: 65, G: 70, B: 85, A: 255}, false)
	}
}

func (g *game) Layout(width, height int) (int, int) {
	width, height = max(360, width), max(280, height)
	if g.width != width || g.height != height {
		g.width, g.height, g.dirty = width, height, true
	}
	return width, height
}

func run() error {
	source, err := text.ParseFont(goregular.TTF)
	if err != nil {
		return err
	}
	atlas := image.NewRGBA(image.Rect(0, 0, 5, 7))
	for y := range 7 {
		for x := range 5 {
			if x == 0 || x == 4 || y == 0 || y == 6 {
				atlas.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	icons, err := text.NewBitmapSource(atlas, text.Metrics{Ascent: 7}, map[rune]text.Glyph{'\ufffc': {Region: atlas.Bounds(), Advance: 6, BearingY: -7}})
	if err != nil {
		return err
	}
	renderer, err := text.NewRenderer(text.DefaultRendererOptions())
	if err != nil {
		return err
	}
	defer renderer.Close()
	history, err := text.NewLog(4, 512)
	if err != nil {
		return err
	}
	g := &game{source: source, icons: icons, renderer: renderer, log: history, width: 640, height: 420, size: 22, input: "Host-owned editing", dirty: true}
	g.caret = len(g.input)
	if err := g.rebuild(); err != nil {
		return err
	}
	ebiten.SetWindowSize(640, 420)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("Berad Text — host input demo")
	if err := ebiten.RunGame(g); err != nil {
		return fmt.Errorf("text example: %w", err)
	}
	return g.err
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
