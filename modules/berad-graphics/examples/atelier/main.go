// Command atelier demonstrates supplied irregular artwork and deterministic effects.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	backend "github.com/bradhannah/berad-sdk/modules/berad-graphics/backend/ebitengine"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	canvasWidth, canvasHeight = 640, 360
	artWidth, artHeight       = 17, 23
	frameCount                = 8
	frameInterval             = 100
	previewScale              = 4
	columnSpacing, rowSpacing = 110, 140
	margin                    = 30
	cycleTicks                = 180
	sineRange                 = 2
	maskColumn                = 2
	proceduralColumn          = 3
	channelMaximum            = 255
	additiveGreen             = 120
	additiveBlue              = 40
)

type host struct {
	processor          *backend.Processor
	sequence           *graphics.Animation
	art, contour, mask *ebiten.Image
	dissolve           *graphics.Dissolve
	surface            backend.Surface
	buffer             []byte
	ticks              int64
	failure            error
}

func (scene *host) Update() error {
	if scene.failure != nil {
		return scene.failure
	}
	scene.ticks++
	progress := (1 - math.Cos(float64(scene.ticks)*sineRange*math.Pi/cycleTicks)) / sineRange
	if _, err := scene.dissolve.Update(progress); err != nil {
		return fmt.Errorf("animate dissolve: %w", err)
	}
	scene.dissolve.WritePixels(scene.buffer)
	if err := scene.surface.Upload(); err != nil {
		return fmt.Errorf("upload dissolve: %w", err)
	}

	return nil
}

func placement(column, row int) ebiten.DrawImageOptions {
	var options ebiten.DrawImageOptions
	options.GeoM.Scale(previewScale, previewScale)
	options.GeoM.Translate(float64(margin+column*columnSpacing), float64(margin+row*rowSpacing))

	return options
}

func (scene *host) Draw(target *ebiten.Image) {
	target.Fill(color.Black)
	options := placement(0, 0)
	target.DrawImage(scene.art, &options)
	options = placement(1, 0)
	target.DrawImage(scene.contour, &options)
	options = placement(maskColumn, 0)
	target.DrawImage(scene.mask, &options)
	const tickMilliseconds = 1000.0 / 60
	frame := scene.sequence.At(int64(float64(scene.ticks) * tickMilliseconds))
	if err := scene.processor.DrawFrame(target, frame, placement(proceduralColumn, 0)); err != nil {
		scene.failure = err
	}
	adjustment := backend.DefaultColourAdjustment()
	adjustment.HueRadians = float64(scene.ticks) * 2 * math.Pi / cycleTicks
	if err := backend.DrawAdjusted(target, scene.art, adjustment, placement(0, 1)); err != nil {
		scene.failure = err
	}
	if err := scene.surface.Draw(target, placement(1, 1)); err != nil {
		scene.failure = err
	}
	alpha, err := graphics.Pulse(scene.ticks, cycleTicks, 0, 1)
	if err != nil {
		scene.failure = err

		return
	}
	options = placement(maskColumn, 1)
	options.Blend = ebiten.BlendLighter
	if err := backend.DrawAdditiveChannels(target, scene.mask,
		channelMaximum, additiveGreen, additiveBlue, float32(alpha), options); err != nil {
		scene.failure = err
	}
}

func (*host) Layout(int, int) (int, int) { return canvasWidth, canvasHeight }

//nolint:mnd // These literal channels and stepped contours define synthetic example artwork.
func artwork() image.Image {
	atlas := image.NewRGBA(image.Rect(10, 20, 10+artWidth, 20+artHeight))
	for y := range artHeight {
		for x := range artWidth {
			if x > 2 && x < 14 && y > 1 && y < 21 && (x < 6 || x > 10 || y < 8 || y > 14) {
				atlas.SetRGBA(10+x, 20+y, color.RGBA{R: byte(40 + x*5), G: byte(30 + y*4), B: 20, A: 180})
			}
		}
	}

	return atlas
}

func prepare(processor *backend.Processor) (*host, error) {
	source, err := graphics.NewImage(artwork())
	if err != nil {
		return nil, fmt.Errorf("import artwork: %w", err)
	}
	var frames []graphics.Frame
	for index := range frameCount {
		var selection graphics.Selection
		const amplitude, rowPhase, framePhase = 1.1, .5, .8
		framePixels, err := source.Apply(graphics.Rise(index, selection),
			graphics.Wave(amplitude, float64(index)*framePhase, rowPhase, selection), graphics.ScrollY(index))
		if err != nil {
			return nil, fmt.Errorf("prepare procedural frame: %w", err)
		}
		frame, err := graphics.NewFrame(framePixels, source.Bounds().Size(), image.Pt(0, 0), image.Pt(0, 0))
		if err != nil {
			return nil, fmt.Errorf("configure frame: %w", err)
		}
		frames = append(frames, frame)
	}
	scene := new(host)
	scene.processor = processor
	scene.sequence, err = graphics.NewAnimation(frames, frameInterval)
	if err != nil {
		return nil, fmt.Errorf("prepare sequence: %w", err)
	}
	scene.art, err = processor.DeriveImage(source)
	if err != nil {
		return nil, fmt.Errorf("upload artwork: %w", err)
	}
	face := color.RGBA{R: 0, G: channelMaximum, B: 0, A: channelMaximum}
	backing := color.RGBA{R: 0, G: 0, B: 0, A: channelMaximum}
	scene.contour, err = processor.DeriveImage(source, graphics.Contour(face, backing))
	if err != nil {
		return nil, fmt.Errorf("derive contour: %w", err)
	}
	scene.mask, err = processor.DeriveImage(source.Silhouette(0))
	if err != nil {
		return nil, fmt.Errorf("derive silhouette: %w", err)
	}
	//nolint:gosec // Fixed presentation-only example seed; no security or gameplay randomness.
	random := rand.New(rand.NewSource(1))
	scene.dissolve, err = graphics.NewDissolve(artWidth, artHeight, false, random)
	if err != nil {
		return nil, fmt.Errorf("prepare dissolve: %w", err)
	}
	scene.buffer, err = scene.surface.Resize(artWidth, artHeight)
	if err != nil {
		return nil, fmt.Errorf("prepare surface: %w", err)
	}

	return scene, nil
}

func run() error {
	processor, err := backend.NewProcessor(backend.DefaultLimits())
	if err != nil {
		return fmt.Errorf("prepare graphics: %w", err)
	}
	defer processor.Close()
	scene, err := prepare(processor)
	if err != nil {
		return err
	}
	defer scene.surface.Close()
	ebiten.SetWindowSize(canvasWidth, canvasHeight)
	ebiten.SetWindowTitle("Berad Graphics: source / contour / mask / procedural; hue / dissolve / additive alpha")
	if err := ebiten.RunGame(scene); err != nil {
		return fmt.Errorf("graphics demo: %w", err)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
