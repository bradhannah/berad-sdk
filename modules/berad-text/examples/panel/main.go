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

const (
	labelPaint    = "label"
	accentPaint   = "accent"
	panelPadding  = 24
	sectionGap    = 20
	fieldPadding  = 6
	rowGap        = 4
	coverLeft     = 100
	coverTop      = 16
	coverRight    = 320
	coverBottom   = 120
	coverMargin   = 8
	minimumSize   = 12
	maximumSize   = 32
	sizeStep      = 2
	initialSize   = 22
	minimumWidth  = 360
	minimumHeight = 280
	initialWidth  = 640
	initialHeight = 420
	iconWidth     = 5
	iconHeight    = 7
	iconAdvance   = 6
	logEntries    = 4
	logBytes      = 512
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
	content, err := text.NewContent([]text.Span{{Text: value, Font: g.font, Paint: "", Tag: ""}})
	if err != nil {
		return fmt.Errorf("compose log entry: %w", err)
	}
	if _, err = g.log.Append(content); err != nil {
		return fmt.Errorf("append log entry: %w", err)
	}
	g.dirty = true

	return nil
}

func (g *game) rebuild() error {
	var fontOptions text.FontOptions
	fontOptions.Size = g.size
	font, err := g.source.NewFont(fontOptions)
	if err != nil {
		return fmt.Errorf("size heading font: %w", err)
	}
	g.font = font
	icon, err := g.icons.NewFont(fontOptions)
	if err != nil {
		return fmt.Errorf("size icon font: %w", err)
	}
	content, err := text.NewContent([]text.Span{
		{Text: "Reactor ", Font: font, Paint: labelPaint, Tag: ""},
		{Text: "\ufffc", Font: icon, Paint: accentPaint, Tag: "reactor"},
		{Text: " online. Click the icon or edit the field.\n" +
			"Space: cover   T: theme   +/-: size\nResize the window to reflow.", Font: font, Paint: labelPaint, Tag: ""},
	})
	if err != nil {
		return fmt.Errorf("compose heading: %w", err)
	}
	g.panel.Set(content)
	var wrapped text.LayoutOptions
	wrapped.Width = float64(g.width - panelPadding*2)
	g.heading, err = text.NewLayout(content, wrapped)
	if err != nil {
		return fmt.Errorf("layout heading: %w", err)
	}
	field, err := text.NewContent([]text.Span{{Text: g.input, Font: font, Paint: accentPaint, Tag: ""}})
	if err != nil {
		return fmt.Errorf("compose field: %w", err)
	}
	var unwrapped text.LayoutOptions
	g.field, err = text.NewLayout(field, unwrapped)
	if err != nil {
		return fmt.Errorf("layout field: %w", err)
	}
	g.rows = nil
	for _, entry := range g.log.Entries() {
		layout, layoutErr := text.NewLayout(entry, wrapped)
		if layoutErr != nil {
			return fmt.Errorf("layout log entry: %w", layoutErr)
		}
		g.rows = append(g.rows, layout)
	}
	g.updatePalette()
	g.dirty = false

	return nil
}

//nolint:mnd // These literal RGBA channels define the example's authored palette.
func (g *game) updatePalette() {
	g.palette = text.Palette{
		labelPaint:  color.NRGBA{R: 220, G: 225, B: 235, A: 255},
		accentPaint: color.NRGBA{R: 80, G: 210, B: 255, A: 255},
	}
	if g.alternate {
		g.palette[accentPaint] = color.NRGBA{R: 255, G: 190, B: 70, A: 255}
	}
}

func (g *game) fieldBounds() image.Rectangle {
	y := panelPadding + int(g.heading.Bounds().Height()) + sectionGap

	return image.Rect(panelPadding, y, g.width-panelPadding, y+int(g.field.Bounds().Height())+fieldPadding*2)
}

func (g *game) blocker() image.Rectangle {
	if !g.cover {
		return image.Rect(0, 0, 0, 0)
	}

	return image.Rect(coverLeft, coverTop, min(g.width-coverMargin, coverRight), coverBottom)
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
		if err := g.pointerPressed(); err != nil {
			return err
		}
	}
	if g.focused {
		if err := g.editField(); err != nil {
			return err
		}
	} else {
		g.updateControls()
	}
	if g.dirty {
		return g.rebuild()
	}

	return nil
}

func (g *game) pointerPressed() error {
	positionX, positionY := ebiten.CursorPosition()
	point := image.Pt(positionX, positionY)
	if point.In(g.blocker()) {
		return nil
	}
	g.focused = point.In(g.fieldBounds())
	if g.focused {
		bounds := g.fieldBounds()
		site, err := g.field.NearestCaret(text.Point{
			X: float64(positionX - bounds.Min.X), Y: float64(positionY - bounds.Min.Y - fieldPadding),
		})
		if err != nil {
			return fmt.Errorf("locate field caret: %w", err)
		}
		g.caret = site.Offset
	} else if tag := annotationAt(g.heading, point, image.Pt(panelPadding, panelPadding), g.blocker()); tag != "" {
		return g.record("Host activated: " + tag)
	}

	return nil
}

func (g *game) editField() error {
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

	return nil
}

func (g *game) updateControls() {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.cover = !g.cover
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyT) {
		g.alternate = !g.alternate
		g.updatePalette() // Paint changes retain the existing layout objects.
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		g.size = min(maximumSize, g.size+sizeStep)
		g.dirty = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		g.size = max(minimumSize, g.size-sizeStep)
		g.dirty = true
	}
}

func (g *game) draw(target *ebiten.Image, layout *text.Layout, x, y float64) {
	options := text.DrawOptions{}
	options.GeoM.Translate(x, y)
	if err := g.renderer.Draw(target, layout, g.palette, options); err != nil {
		g.err = err
	}
}

//nolint:mnd // Background and cover RGBA literals are authored example colours.
func (g *game) Draw(target *ebiten.Image) {
	target.Fill(color.RGBA{R: 20, G: 24, B: 32, A: 255})
	if g.heading == nil {
		return
	}
	g.draw(target, g.heading, panelPadding, panelPadding)
	bounds := g.fieldBounds()
	vector.StrokeRect(target, float32(bounds.Min.X), float32(bounds.Min.Y),
		float32(bounds.Dx()), float32(bounds.Dy()), 1, color.White, false)
	g.draw(target, g.field, float64(bounds.Min.X), float64(bounds.Min.Y+fieldPadding))
	if g.focused {
		caret, err := g.field.CaretAt(g.caret, false)
		if err != nil {
			g.err = err
		} else {
			vector.FillRect(target, float32(bounds.Min.X)+float32(caret.Position.X),
				float32(bounds.Min.Y+fieldPadding), 1, float32(caret.Height), color.White, false)
		}
	}
	y := float64(bounds.Max.Y + sectionGap)
	for _, row := range g.rows {
		g.draw(target, row, panelPadding, y)
		y += row.Bounds().Height() + rowGap
	}
	if blocker := g.blocker(); !blocker.Empty() {
		vector.FillRect(target, float32(blocker.Min.X), float32(blocker.Min.Y),
			float32(blocker.Dx()), float32(blocker.Dy()), color.RGBA{R: 65, G: 70, B: 85, A: 255}, false)
	}
}

func (g *game) Layout(width, height int) (int, int) {
	width, height = max(minimumWidth, width), max(minimumHeight, height)
	if g.width != width || g.height != height {
		g.width, g.height, g.dirty = width, height, true
	}

	return width, height
}

func run() error {
	source, err := text.ParseFont(goregular.TTF)
	if err != nil {
		return fmt.Errorf("parse example font: %w", err)
	}
	atlas := image.NewRGBA(image.Rect(0, 0, iconWidth, iconHeight))
	for y := range iconHeight {
		for x := range iconWidth {
			if x == 0 || x == iconWidth-1 || y == 0 || y == iconHeight-1 {
				atlas.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	icons, err := text.NewBitmapSource(atlas, text.Metrics{Ascent: iconHeight, Descent: 0, LineGap: 0},
		map[rune]text.Glyph{'\ufffc': {
			Region: atlas.Bounds(), Advance: iconAdvance, BearingY: -iconHeight, BearingX: 0, Colored: false,
		}})
	if err != nil {
		return fmt.Errorf("construct example icons: %w", err)
	}
	renderer, err := text.NewRenderer(text.DefaultRendererOptions())
	if err != nil {
		return fmt.Errorf("construct example renderer: %w", err)
	}
	defer renderer.Close()
	history, err := text.NewLog(logEntries, logBytes)
	if err != nil {
		return fmt.Errorf("construct example log: %w", err)
	}
	host := new(game)
	host.source, host.icons, host.renderer, host.log = source, icons, renderer, history
	host.width, host.height, host.size = initialWidth, initialHeight, initialSize
	host.input, host.dirty = "Host-owned editing", true
	host.caret = len(host.input)
	if err := host.rebuild(); err != nil {
		return err
	}
	ebiten.SetWindowSize(initialWidth, initialHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("Berad Text — host input demo")
	if err := ebiten.RunGame(host); err != nil {
		return fmt.Errorf("text example: %w", err)
	}

	return host.err
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
