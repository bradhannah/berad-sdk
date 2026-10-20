// Package graphics supplies immutable image operations independent of game policy.
package graphics

import (
	"bytes"
	"errors"
	"fmt"
	"image"
)

const (
	channels      = 4
	alphaChannel  = 3
	maxDimension  = 8192
	maxImageBytes = 32 << 20
)

// ErrInvalid identifies invalid input or a request exceeding supported limits.
var ErrInvalid = errors.New("invalid graphics request")

// Image is a frozen zero-origin RGBA channel snapshot. Pointer identity is stable
// while retained; neither Pixels nor CopyRGBA exposes its authoritative storage.
type Image struct {
	width, height int
	pixels        []byte
}

func validateSize(width, height int) error {
	if width <= 0 || height <= 0 || width > maxDimension || height > maxDimension ||
		width > maxImageBytes/channels/height {
		return fmt.Errorf("%w: image exceeds dimension/byte limits", ErrInvalid)
	}

	return nil
}

// NewPixels snapshots tightly packed RGBA channels. Callers supply the alpha
// convention; independent-channel/emissive results must use the intended blend.
func NewPixels(width, height int, pixels []byte) (*Image, error) {
	if err := validateSize(width, height); err != nil {
		return nil, err
	}
	if len(pixels) != width*height*channels {
		return nil, fmt.Errorf("%w: pixel payload does not match dimensions", ErrInvalid)
	}

	return &Image{width: width, height: height, pixels: bytes.Clone(pixels)}, nil
}

// Bounds returns zero-origin storage bounds.
func (source *Image) Bounds() image.Rectangle { return image.Rect(0, 0, source.width, source.height) }

// Pixels returns a detached tightly packed channel buffer.
func (source *Image) Pixels() []byte { return bytes.Clone(source.pixels) }

// Bytes reports channel payload, not object/driver overhead.
func (source *Image) Bytes() int { return len(source.pixels) }

// CopyRGBA returns an independently mutable image.
func (source *Image) CopyRGBA() *image.RGBA {
	return &image.RGBA{Pix: source.Pixels(), Stride: source.width * channels, Rect: source.Bounds()}
}

// Insets specifies independent transparent padding in pixels for native region copies.
type Insets struct{ Left, Top, Right, Bottom int }
