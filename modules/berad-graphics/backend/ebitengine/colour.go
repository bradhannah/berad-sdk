package ebitengine

import (
	"fmt"
	"math"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/colorm"
)

// ColourAdjustment uses Ebitengine's HSV colour matrix on straight colour values
// before premultiplied drawing. Hue is radians; zero saturation/value are meaningful.
// It is distinct from independent RGB gain or additive emission.
type ColourAdjustment struct{ HueRadians, Saturation, Value float64 }

// DefaultColourAdjustment returns identity hue/saturation/value settings.
func DefaultColourAdjustment() ColourAdjustment {
	return ColourAdjustment{HueRadians: 0, Saturation: 1, Value: 1}
}

// DrawAdjusted borrows source/target and applies the selected engine colour matrix
// before the host's colour scale, transform and blend. It creates no retained cache.
func DrawAdjusted(
	target, source *ebiten.Image, adjustment ColourAdjustment, options ebiten.DrawImageOptions,
) error {
	if source == nil || target == nil || !adjustment.valid() {
		return fmt.Errorf("%w: colour adjustment", graphics.ErrInvalid)
	}
	const cycle = 2 * math.Pi
	var matrix colorm.ColorM
	matrix.ChangeHSV(math.Remainder(adjustment.HueRadians, cycle), adjustment.Saturation, adjustment.Value)
	colorm.DrawImage(target, source, matrix, &colorm.DrawImageOptions{
		GeoM: options.GeoM, ColorScale: options.ColorScale, Blend: options.Blend, Filter: options.Filter,
	})

	return nil
}

func (adjustment ColourAdjustment) valid() bool {
	const maximumFactor = 16
	for _, value := range []float64{adjustment.HueRadians, adjustment.Saturation, adjustment.Value} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return adjustment.Saturation >= 0 && adjustment.Saturation <= maximumFactor &&
		adjustment.Value >= 0 && adjustment.Value <= maximumFactor
}
