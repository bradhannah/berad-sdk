// Command panel demonstrates caller-owned text resources and geometry.
package main

import (
	"fmt"
	"image/color"
	"log"

	text "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/goregular"
)

type game struct {
	renderer *text.Renderer
	layout   *text.Layout
	palette  text.Palette
	err      error
}

func (g *game) Update() error { return g.err }
func (g *game) Draw(target *ebiten.Image) {
	target.Fill(color.RGBA{R: 20, G: 24, B: 32, A: 255})
	options := text.DrawOptions{}
	options.GeoM.Translate(24, 24)
	g.err = g.renderer.Draw(target, g.layout, g.palette, options)
}
func (*game) Layout(int, int) (int, int) { return 640, 360 }
func run() error {
	source, err := text.ParseFont(goregular.TTF)
	if err != nil {
		return err
	}
	font, err := source.NewFont(text.FontOptions{Size: 24})
	if err != nil {
		return err
	}
	content, err := text.NewContent([]text.Span{
		{Text: "Independent text rendering\n", Font: font, Paint: "heading"},
		{Text: "This host supplies its own font, colours, content and bounds. The renderer owns only its private cache.", Font: font},
	})
	if err != nil {
		return err
	}
	layout, err := text.NewLayout(content, text.LayoutOptions{Width: 560, LineSpacing: 34})
	if err != nil {
		return err
	}
	renderer, err := text.NewRenderer(text.DefaultRendererOptions())
	if err != nil {
		return err
	}
	defer renderer.Close()
	host := &game{renderer: renderer, layout: layout, palette: text.Palette{"heading": color.NRGBA{R: 80, G: 210, B: 255, A: 255}}}
	ebiten.SetWindowSize(640, 360)
	ebiten.SetWindowTitle("Berad Text")
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
