// Command panel demonstrates supplied fonts, styled text, wrapping and repainting.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"

	text "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	labelPaint    = "label"
	accentPaint   = "accent"
	panelPadding  = 24
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
)

type game struct {
	renderer      *text.Renderer
	source, icons *text.Source
	layout        *text.Layout
	palette       text.Palette
	width         int
	size          float64
	dirty         bool
	alternate     bool
	err           error
}

func (g *game) rebuild() error {
	var options text.FontOptions
	options.Size = g.size
	font, err := g.source.NewFont(options)
	if err != nil {
		return fmt.Errorf("size text font: %w", err)
	}
	icon, err := g.icons.NewFont(options)
	if err != nil {
		return fmt.Errorf("size icon font: %w", err)
	}
	content, err := text.NewContent([]text.Span{
		{Text: "Reactor ", Font: font, Paint: labelPaint},
		{Text: "\ufffc", Font: icon, Paint: accentPaint},
		{Text: " online.\n\nSupplied outline and bitmap fonts share styled layout. " +
			"Resize the window to reflow without rewriting the source.\n\n" +
			"T: change paint   +/-: change font size", Font: font, Paint: labelPaint},
	})
	if err != nil {
		return fmt.Errorf("compose text: %w", err)
	}
	var wrapped text.LayoutOptions
	wrapped.Width = float64(g.width - panelPadding*2)
	g.layout, err = text.NewLayout(content, wrapped)
	if err != nil {
		return fmt.Errorf("layout text: %w", err)
	}
	g.dirty = false

	return nil
}

//nolint:mnd // Literal RGBA channels define the example's authored palette.
func (g *game) updatePalette() {
	g.palette = text.Palette{
		labelPaint:  color.NRGBA{R: 220, G: 225, B: 235, A: 255},
		accentPaint: color.NRGBA{R: 80, G: 210, B: 255, A: 255},
	}
	if g.alternate {
		g.palette[accentPaint] = color.NRGBA{R: 255, G: 190, B: 70, A: 255}
	}
}

func (g *game) Update() error {
	if g.err != nil {
		return g.err
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyT) {
		g.alternate = !g.alternate
		g.updatePalette() // Repainting retains the existing layout.
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		g.size = min(maximumSize, g.size+sizeStep)
		g.dirty = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		g.size = max(minimumSize, g.size-sizeStep)
		g.dirty = true
	}
	if g.dirty {
		return g.rebuild()
	}

	return nil
}

//nolint:mnd // Literal RGBA channels define the example's authored background.
func (g *game) Draw(target *ebiten.Image) {
	target.Fill(color.RGBA{R: 20, G: 24, B: 32, A: 255})
	if g.layout == nil {
		return
	}
	var options text.DrawOptions
	options.GeoM.Translate(panelPadding, panelPadding)
	if err := g.renderer.Draw(target, g.layout, g.palette, options); err != nil {
		g.err = err
	}
}

func (g *game) Layout(width, height int) (int, int) {
	width, height = max(minimumWidth, width), max(minimumHeight, height)
	if g.width != width {
		g.width, g.dirty = width, true
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
			Region: atlas.Bounds(), Advance: iconAdvance, BearingY: -iconHeight, BearingX: 0,
		}})
	if err != nil {
		return fmt.Errorf("construct example icons: %w", err)
	}
	renderer, err := text.NewRenderer(text.DefaultRendererOptions())
	if err != nil {
		return fmt.Errorf("construct example renderer: %w", err)
	}
	defer renderer.Close()
	host := new(game)
	host.source, host.icons, host.renderer = source, icons, renderer
	host.width, host.size = initialWidth, initialSize
	host.updatePalette()
	if err := host.rebuild(); err != nil {
		return err
	}
	ebiten.SetWindowSize(initialWidth, initialHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("Berad Text — styled layout")
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
