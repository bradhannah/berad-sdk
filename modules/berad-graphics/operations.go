package graphics

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

type operationKind uint8

const (
	operationInvalid operationKind = iota
	operationOutline
	operationContour
	operationCheckerOutline
	operationGain
	operationAlphaScale
	operationScroll
	operationWave
	operationRise
	operationReplace
)

const (
	channelMaximum = 255
	operationLimit = 16
)

// Mask is an immutable binary destination-selection mask, not fractional coverage.
type Mask struct {
	width, height int
	selected      []byte
}

// NewMask copies one byte per pixel; zero rejects and any nonzero value selects.
func NewMask(width, height int, selected []byte) (*Mask, error) {
	if err := validateSize(width, height); err != nil {
		return nil, err
	}
	if len(selected) != width*height {
		return nil, fmt.Errorf("%w: mask dimensions", ErrInvalid)
	}
	owned := make([]byte, len(selected))
	for index, value := range selected {
		if value != 0 {
			owned[index] = 1
		}
	}

	return &Mask{width: width, height: height, selected: owned}, nil
}

// Selection freezes an optional rectangular/mask restriction on destination writes.
// Its zero value selects the whole image. Sampling remains bounded by its rectangle.
type Selection struct {
	bounds     image.Rectangle
	restricted bool
	mask       *Mask
}

// Select limits writes to the explicit rectangle and optional matching mask.
func Select(bounds image.Rectangle, mask *Mask) Selection {
	return Selection{bounds: bounds, restricted: true, mask: mask}
}

// Masked selects across the full image using an immutable mask.
func Masked(mask *Mask) Selection {
	return Selection{bounds: image.Rect(0, 0, 0, 0), restricted: false, mask: mask}
}

// Operation is a frozen, comparable specification. Its full value can be used in
// cache equality; sources/masks are strongly retained, never encoded as raw addresses.
type Operation struct {
	kind                    operationKind
	selection               Selection
	first, second           color.RGBA
	factor, phase, rowPhase float64
	frame                   int
	threshold               byte
	replacement             *Image
}

// RetainedBytes reports mask/replacement payload for conservative cache accounting.
func (operation Operation) RetainedBytes() int64 {
	var size int64
	if operation.selection.mask != nil {
		size += int64(len(operation.selection.mask.selected))
	}
	if operation.replacement != nil {
		size += int64(operation.replacement.Bytes())
	}

	return size
}

// Outline expands four-neighbor source edges into eight transparent neighbors.
func Outline(paint color.RGBA, threshold byte) Operation {
	var operation Operation
	operation.kind, operation.first, operation.threshold = operationOutline, paint, threshold

	return operation
}

// Contour adds face/backing rings and removes every original nontransparent body pixel.
func Contour(face, backing color.RGBA) Operation {
	operation := Outline(face, 0)
	operation.kind, operation.second = operationContour, backing

	return operation
}

// CheckerOutline alternates supplied colours by local pixel parity and explicit phase.
func CheckerOutline(first, second color.RGBA, phase int) Operation {
	operation := Outline(first, 0)
	operation.kind, operation.second, operation.frame = operationCheckerOutline, second, phase&1

	return operation
}

// RGBGain independently multiplies RGB with 8-bit saturation; zero means black.
// This includes alpha-zero emission. Alpha is unchanged; gain above one can
// produce emissive RGB greater than alpha, by explicit contract.
func RGBGain(factor float64, selection Selection) Operation {
	var operation Operation
	operation.kind, operation.factor, operation.selection = operationGain, factor, selection

	return operation
}

// AlphaChannelScale changes alpha alone while preserving RGB. This is deliberately
// not ordinary opacity and retains the caller's independent-channel appearance.
func AlphaChannelScale(factor float64, selection Selection) Operation {
	operation := RGBGain(factor, selection)
	operation.kind = operationAlphaScale

	return operation
}

// ScrollY scrolls the whole image downward with wrap, including negative offsets.
func ScrollY(offset int) Operation {
	var operation Operation
	operation.kind, operation.frame = operationScroll, offset

	return operation
}

// Wave displaces source samples horizontally by trunc(sin(phase+y*rowPhase)*amplitude).
// Nontransparent selected destinations sample within the selection rectangle.
func Wave(amplitude, phase, rowPhase float64, selection Selection) Operation {
	operation := RGBGain(amplitude, selection)
	operation.kind, operation.phase, operation.rowPhase = operationWave, phase, rowPhase

	return operation
}

// Rise samples a deterministic upward/jitter pattern at an explicit integer phase.
// It consumes no RNG; selection controls destination writes, not source colours.
func Rise(frame int, selection Selection) Operation {
	const phasePeriod = 100
	var operation Operation
	operation.kind, operation.frame, operation.selection = operationRise, modulo(frame, phasePeriod), selection

	return operation
}

// Replace substitutes selected pixels from a same-sized immutable image.
func Replace(replacement *Image, selection Selection) Operation {
	var operation Operation
	operation.kind, operation.replacement, operation.selection = operationReplace, replacement, selection

	return operation
}

// Apply executes up to sixteen ordered operations, without mutating the source.
func (source *Image) Apply(operations ...Operation) (*Image, error) {
	if source == nil || len(operations) > operationLimit {
		return nil, fmt.Errorf("%w: source or operation limit", ErrInvalid)
	}
	current := source
	for _, operation := range operations {
		bounds, err := operation.validate(current)
		if err != nil {
			return nil, err
		}
		output := current.Pixels()
		operation.execute(current, output, bounds)
		current = &Image{width: source.width, height: source.height, pixels: output}
	}

	return current, nil
}

func (operation Operation) validate(source *Image) (image.Rectangle, error) {
	bounds := source.Bounds()
	if operation.selection.restricted {
		bounds = operation.selection.bounds
	}
	if bounds.Empty() || !bounds.In(source.Bounds()) || !operation.validParameters() {
		return bounds, fmt.Errorf("%w: unsupported operation/parameters", ErrInvalid)
	}
	if mask := operation.selection.mask; mask != nil && (mask.width != source.width || mask.height != source.height) {
		return bounds, fmt.Errorf("%w: selection mask size", ErrInvalid)
	}
	if operation.kind == operationAlphaScale {
		if operation.factor > 1 {
			return bounds, fmt.Errorf("%w: alpha factor exceeds one", ErrInvalid)
		}
	}
	if operation.kind == operationReplace &&
		(operation.replacement == nil || operation.replacement.Bounds() != source.Bounds()) {
		return bounds, fmt.Errorf("%w: replacement size", ErrInvalid)
	}

	return bounds, nil
}

func (operation Operation) validParameters() bool {
	const maximumPhase = 1 << 30

	return operation.kind > operationInvalid && operation.kind <= operationReplace &&
		finite(operation.factor) && finite(operation.phase) && finite(operation.rowPhase) &&
		operation.factor >= 0 && operation.factor <= maxDimension &&
		math.Abs(operation.phase) <= maximumPhase && math.Abs(operation.rowPhase) <= maximumPhase
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func modulo(value, divisor int) int {
	value %= divisor
	if value < 0 {
		value += divisor
	}

	return value
}

func (operation Operation) selected(source *Image, x, y int) bool {
	mask := operation.selection.mask

	return mask == nil || mask.selected[y*source.width+x] != 0
}
