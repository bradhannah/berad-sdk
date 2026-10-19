package ebitengine

import (
	"fmt"
	"image"
	"math"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	"github.com/hajimehoshi/ebiten/v2"
)

// CopyRegion returns an owned, zero-origin GPU copy; it performs no CPU readback.
func CopyRegion(source *ebiten.Image, region image.Rectangle, padding graphics.Insets) (*ebiten.Image, error) {
	bounds, err := Bounds(source)
	if err != nil {
		return nil, err
	}
	const maximumDimension, maximumBytes, channels = 8192, 32 << 20, 4
	if region.Empty() || !region.In(bounds) || !validPadding(padding, maximumDimension) {
		return nil, fmt.Errorf("%w: region/padding", graphics.ErrInvalid)
	}
	width, height := region.Dx()+padding.Left+padding.Right, region.Dy()+padding.Top+padding.Bottom
	if width > maximumDimension || height > maximumDimension || width > maximumBytes/channels/height {
		return nil, fmt.Errorf("%w: padded image size", graphics.ErrInvalid)
	}
	crop, ok := source.SubImage(region).(*ebiten.Image)
	if !ok {
		panic("Ebitengine SubImage returned a foreign image")
	}
	result := ebiten.NewImage(width, height)
	options := new(ebiten.DrawImageOptions)
	options.GeoM.Translate(float64(padding.Left), float64(padding.Top))
	result.DrawImage(crop, options)

	return result, nil
}

func validPadding(padding graphics.Insets, maximum int) bool {
	return padding.Left >= 0 && padding.Top >= 0 && padding.Right >= 0 && padding.Bottom >= 0 &&
		padding.Left <= maximum && padding.Top <= maximum && padding.Right <= maximum && padding.Bottom <= maximum
}

// Surface owns a mutable CPU/GPU image pair. Zero value is ready; Close is terminal.
// Pixel borrows last until resize/close; Image borrows must not outlive the surface.
type Surface struct {
	image         *ebiten.Image
	pixels        []byte
	width, height int
	closed        bool
}

// Resize clears the returned tightly packed RGBA buffer, reusing same-size storage.
func (surface *Surface) Resize(width, height int) ([]byte, error) {
	if surface.closed {
		return nil, ErrClosed
	}
	const maximumDimension, maximumBytes, channels = 8192, 32 << 20, 4
	if width <= 0 || height <= 0 || width > maximumDimension || height > maximumDimension ||
		width > maximumBytes/channels/height {
		return nil, fmt.Errorf("%w: surface dimensions", graphics.ErrInvalid)
	}
	if surface.width != width || surface.height != height {
		if surface.image != nil {
			surface.image.Deallocate()
		}
		surface.image = ebiten.NewImageWithOptions(image.Rect(0, 0, width, height), &ebiten.NewImageOptions{Unmanaged: true})
		surface.pixels = make([]byte, width*height*channels)
		surface.width, surface.height = width, height
	}
	clear(surface.pixels)

	return surface.pixels, nil
}

// Upload commits the buffer; construction/Resize must precede it.
func (surface *Surface) Upload() error {
	if surface.closed || surface.image == nil {
		return ErrClosed
	}
	surface.image.WritePixels(surface.pixels)

	return nil
}

// Image returns a borrowed mutable-surface view, valid until resize/closure.
func (surface *Surface) Image() *ebiten.Image { return surface.image }

// Draw composites the uploaded surface into a caller-owned target.
func (surface *Surface) Draw(target *ebiten.Image, options ebiten.DrawImageOptions) error {
	if surface.closed || surface.image == nil {
		return ErrClosed
	}
	if target == nil {
		return fmt.Errorf("%w: nil surface target", graphics.ErrInvalid)
	}
	target.DrawImage(surface.image, &options)

	return nil
}

// Close releases owned storage; borrowed targets are never closed.
func (surface *Surface) Close() {
	if surface.closed {
		return
	}
	if surface.image != nil {
		surface.image.Deallocate()
	}
	surface.image, surface.pixels, surface.closed = nil, nil, true
}

// DrawAdditiveChannels scales RGB and alpha independently before the supplied
// blend. Under BlendLighter, changing alpha does not modulate added RGB. This
// explicit operation preserves existing emissive effects, not an opacity promise.
// Placement/filter/blend are supplied; colour scale is defined by paint/alpha.
func DrawAdditiveChannels(
	target, source *ebiten.Image, red, green, blue byte, alpha float32, options ebiten.DrawImageOptions,
) error {
	if target == nil || source == nil || math.IsNaN(float64(alpha)) || math.IsInf(float64(alpha), 0) || alpha < 0 {
		return fmt.Errorf("%w: additive inputs", graphics.ErrInvalid)
	}
	const channelMaximum = 255
	options.ColorScale.Reset()
	options.ColorScale.Scale(float32(red)/channelMaximum, float32(green)/channelMaximum,
		float32(blue)/channelMaximum, alpha)
	target.DrawImage(source, &options)

	return nil
}
