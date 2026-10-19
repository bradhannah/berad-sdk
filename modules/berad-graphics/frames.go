package graphics

import (
	"fmt"
	"image"
	"math"
	"slices"
)

// Frame separates stored pixels from the logical canvas, content offset and anchor.
// Draw origin is host position - Anchor + Offset, so padding never moves the object.
type Frame struct {
	pixels                  *Image
	logical, offset, anchor image.Point
}

const maximumFrames = 65536

// NewFrame retains immutable pixels with explicit logical pixel coordinates.
func NewFrame(pixels *Image, logical, offset, anchor image.Point) (Frame, error) {
	if pixels == nil {
		return Frame{}, fmt.Errorf("%w: frame pixels required", ErrInvalid)
	}
	if err := validateSize(logical.X, logical.Y); err != nil {
		return Frame{}, err
	}
	const coordinateLimit = 1 << 30
	for _, coordinate := range []int{offset.X, offset.Y, anchor.X, anchor.Y} {
		if coordinate < -coordinateLimit || coordinate > coordinateLimit {
			return Frame{}, fmt.Errorf("%w: frame coordinate limit", ErrInvalid)
		}
	}

	return Frame{pixels: pixels, logical: logical, offset: offset, anchor: anchor}, nil
}

// Image returns immutable storage, not a mutable device image.
func (frame Frame) Image() *Image { return frame.pixels }

// LogicalSize returns the unchanged logical canvas size.
func (frame Frame) LogicalSize() image.Point { return frame.logical }

// Offset returns the stored-image origin relative to the logical canvas.
func (frame Frame) Offset() image.Point { return frame.offset }

// Anchor returns the logical point placed at the host's drawing position.
func (frame Frame) Anchor() image.Point { return frame.anchor }

// Pad adds storage while compensating the content offset and retaining its anchor.
func (frame Frame) Pad(insets Insets) (Frame, error) {
	padded, err := frame.pixels.Pad(insets)
	if err != nil {
		return Frame{}, err
	}

	return NewFrame(padded, frame.logical, frame.offset.Sub(image.Pt(insets.Left, insets.Top)), frame.anchor)
}

// Animation freezes a uniform-duration sequence of arbitrary anchored frames.
type Animation struct {
	frames           []Frame
	interval, period int64
}

// NewAnimation requires nonempty frames and positive milliseconds per frame.
func NewAnimation(frames []Frame, intervalMs int64) (*Animation, error) {
	if len(frames) == 0 || len(frames) > maximumFrames || intervalMs <= 0 ||
		intervalMs > math.MaxInt64/int64(len(frames)) {
		return nil, fmt.Errorf("%w: animation frames/interval", ErrInvalid)
	}
	for _, frame := range frames {
		if frame.pixels == nil {
			return nil, fmt.Errorf("%w: uninitialized animation frame", ErrInvalid)
		}
	}

	return &Animation{frames: slices.Clone(frames), interval: intervalMs, period: intervalMs * int64(len(frames))}, nil
}

// CycleIndex samples an explicit integer tick/offset without overflowing their sum.
// Hosts own conversion from their clock to ticks; negative ticks wrap positively.
func CycleIndex(count int, tick, offset int64) (int, error) {
	if count <= 0 || count > maximumFrames {
		return 0, fmt.Errorf("%w: cycle count", ErrInvalid)
	}
	index := (tick%int64(count) + offset%int64(count)) % int64(count)
	if index < 0 {
		index += int64(count)
	}

	return int(index), nil
}

// At samples explicit elapsed milliseconds; negative time wraps into the same cycle.
func (animation *Animation) At(elapsedMs int64) Frame {
	time := elapsedMs % animation.period
	if time < 0 {
		time += animation.period
	}

	return animation.frames[time/animation.interval]
}
