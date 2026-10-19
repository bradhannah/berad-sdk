package graphics

import (
	"fmt"
	"image"
	"math"
)

// CoverageFull is Q12 full coverage for ordered dithering.
const CoverageFull = 4096

// DitherKeeps applies a fixed Bayer4 threshold at supplied pattern coordinates.
// Coordinates may be negative. The host chooses world/local alignment and coverage.
func DitherKeeps(coverage uint16, column, row int) bool {
	if coverage > CoverageFull {
		panic("graphics coverage exceeds Q12 one")
	}

	bayer := [4][4]uint16{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}
	const periodMask, thresholdUnit = 3, 128
	rank := bayer[row&periodMask][column&periodMask]

	return coverage > (2*rank+1)*thresholdUnit
}

// Shuffler supplies bounded presentation-only randomness during construction.
type Shuffler interface{ Intn(n int) int }

// Dissolve is a mutable CPU coverage permutation. Progress/reset consume no RNG.
// The owner serializes calls; dimensions count mask cells, not visible image pixels.
type Dissolve struct {
	width, height, current int
	order                  []image.Point
	coverage               []byte
	hide                   bool
}

// NewDissolve creates deterministic reveal (hide=false) or conceal (hide=true).
func NewDissolve(width, height int, hide bool, random Shuffler) (*Dissolve, error) {
	if err := validateSize(width, height); err != nil {
		return nil, err
	}
	if random == nil {
		return nil, fmt.Errorf("%w: dissolve requires random source", ErrInvalid)
	}
	// Coordinate pairs are accounted separately from image channels.
	const coordinateBytes = 16
	if width*height > maxImageBytes/coordinateBytes {
		return nil, fmt.Errorf("%w: dissolve ordering budget", ErrInvalid)
	}
	result := &Dissolve{width: width, height: height, current: 0, order: make([]image.Point, 0, width*height),
		coverage: make([]byte, width*height), hide: hide}
	for y := range height {
		for x := range width {
			result.order = append(result.order, image.Pt(x, y))
		}
	}
	for index := len(result.order) - 1; index > 0; index-- {
		selected := random.Intn(index + 1)
		if selected < 0 || selected > index {
			return nil, fmt.Errorf("%w: random source returned out-of-range value", ErrInvalid)
		}
		result.order[index], result.order[selected] = result.order[selected], result.order[index]
	}
	result.Reset()

	return result, nil
}

// Reset restores initial coverage without replacing or reshuffling the ordering.
func (dissolve *Dissolve) Reset() {
	dissolve.current = 0
	value := byte(0)
	if dissolve.hide {
		value = channelMaximum
	}
	for index := range dissolve.coverage {
		dissolve.coverage[index] = value
	}
}

// Update clamps finite progress to [0,1], supports reversal and reports completion.
func (dissolve *Dissolve) Update(progress float64) (bool, error) {
	if !finite(progress) {
		return false, fmt.Errorf("%w: nonfinite dissolve progress", ErrInvalid)
	}
	target := int(float64(len(dissolve.order)) * min(max(progress, 0), 1))
	value := byte(channelMaximum)
	if dissolve.hide {
		value = 0
	}
	for dissolve.current < target {
		point := dissolve.order[dissolve.current]
		dissolve.coverage[point.Y*dissolve.width+point.X] = value
		dissolve.current++
	}
	for dissolve.current > target {
		dissolve.current--
		point := dissolve.order[dissolve.current]
		dissolve.coverage[point.Y*dissolve.width+point.X] = channelMaximum - value
	}

	return dissolve.current == len(dissolve.order), nil
}

// Pixels returns detached premultiplied white/transparent RGBA retention pixels.
func (dissolve *Dissolve) Pixels() []byte {
	result := make([]byte, len(dissolve.coverage)*channels)
	dissolve.WritePixels(result)

	return result
}

// WritePixels fills a caller-owned correctly sized buffer without allocation.
func (dissolve *Dissolve) WritePixels(destination []byte) {
	if len(destination) != len(dissolve.coverage)*channels {
		panic("dissolve destination size mismatch")
	}
	for index, value := range dissolve.coverage {
		for channel := range channels {
			destination[index*channels+channel] = value
		}
	}
}

// VisitLeading reports recently revealed cells. The host controls highlight use.
func (dissolve *Dissolve) VisitLeading(fraction float64, visit func(image.Point)) {
	if dissolve.hide || dissolve.current == len(dissolve.order) || !finite(fraction) {
		return
	}
	count := int(float64(len(dissolve.order)) * min(max(fraction, 0), 1))
	for index := max(0, dissolve.current-count); index < dissolve.current; index++ {
		visit(dissolve.order[index])
	}
}

// Pulse samples a sine cycle without a clock or hidden position hash. It preserves
// independent low/high values, including values greater than one for RGB gain.
func Pulse(elapsedMs, periodMs int64, low, high float64) (float64, error) {
	if periodMs <= 0 || !finite(low) || !finite(high) || low > high ||
		math.Abs(low) > maxDimension || math.Abs(high) > maxDimension {
		return 0, fmt.Errorf("%w: pulse interval/range", ErrInvalid)
	}
	const fullCycle = 2 * math.Pi
	phase := float64(elapsedMs%periodMs) / float64(periodMs) * fullCycle

	return (high+low)/2 + (high-low)/2*math.Sin(phase), nil
}
