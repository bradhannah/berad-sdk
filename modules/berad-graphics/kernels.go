package graphics

import (
	"image"
	"image/color"
	"math"
)

func (operation Operation) execute(source *Image, output []byte, bounds image.Rectangle) {
	switch operation.kind {
	case operationOutline, operationCheckerOutline:
		operation.outline(source, output)
	case operationContour:
		operation.outline(source, output)
		intermediate := &Image{width: source.width, height: source.height, pixels: append([]byte(nil), output...)}
		Outline(operation.second, 0).outline(intermediate, output)
		for index := 0; index < len(output); index += channels {
			if source.pixels[index+alphaChannel] != 0 {
				clear(output[index : index+channels])
			}
		}
	case operationScroll, operationWave, operationRise, operationReplace,
		operationTint, operationGain, operationOpacity, operationAlphaScale:
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if operation.selected(source, x, y) {
					operation.pixel(source, output, bounds, x, y)
				}
			}
		}
	case operationInvalid:
		panic("validated graphics operation is invalid")
	}
}

func (operation Operation) pixel(source *Image, output []byte, bounds image.Rectangle, column, row int) {
	index := (row*source.width + column) * channels
	switch operation.kind {
	case operationScroll:
		sourceRow := modulo(row-modulo(operation.frame, source.height), source.height)
		copy(output[index:index+channels], source.pixels[(sourceRow*source.width+column)*channels:][:channels])
	case operationReplace:
		copy(output[index:index+channels], operation.replacement.pixels[index:index+channels])
	case operationWave, operationRise:
		if source.pixels[index+alphaChannel] != 0 {
			sampleX, sampleY := operation.sample(bounds, column, row)
			copy(output[index:index+channels], source.pixels[(sampleY*source.width+sampleX)*channels:][:channels])
		}
	case operationOpacity:
		for channel := range channels {
			output[index+channel] = byte(float64(source.pixels[index+channel]) * operation.factor)
		}
	case operationAlphaScale:
		output[index+alphaChannel] = byte(float64(source.pixels[index+alphaChannel]) * operation.factor)
	case operationGain, operationTint:
		if source.pixels[index+alphaChannel] != 0 {
			operation.colour(source.pixels[index:index+channels], output[index:index+channels])
		}
	case operationInvalid, operationOutline, operationContour, operationCheckerOutline:
		panic("non-pixel graphics operation")
	}
}

func (operation Operation) sample(bounds image.Rectangle, column, row int) (int, int) {
	if operation.kind == operationWave {
		column += int(math.Sin(operation.phase+float64(row)*operation.rowPhase) * operation.factor)
	} else {
		// Fixed deterministic spatial hash defines this procedural kernel, not a game clock/RNG.
		const columnPrime, rowPrime, phasePrime, modulus = 31, 17, 23, 100
		const riseThreshold, jitterPrime, leftThreshold, rightThreshold = 30, 7, 20, 40
		seed := (column*columnPrime + row*rowPrime + operation.frame*phasePrime) % modulus
		if seed < riseThreshold {
			row++
		}
		jitter := seed * jitterPrime % modulus
		if jitter < leftThreshold {
			column--
		} else if jitter < rightThreshold {
			column++
		}
	}

	return max(bounds.Min.X, min(bounds.Max.X-1, column)), max(bounds.Min.Y, min(bounds.Max.Y-1, row))
}

func (operation Operation) colour(input, output []byte) {
	factors := [3]float64{operation.factor, operation.factor, operation.factor}
	if operation.kind == operationTint {
		factors = [3]float64{float64(operation.first.R) / channelMaximum,
			float64(operation.first.G) / channelMaximum, float64(operation.first.B) / channelMaximum}
	}
	for channel, factor := range factors {
		output[channel] = byte(min(float64(channelMaximum), float64(input[channel])*factor))
	}
}

func (source *Image) alpha(x, y int) byte {
	if x < 0 || y < 0 || x >= source.width || y >= source.height {
		return 0
	}

	return source.pixels[(y*source.width+x)*channels+alphaChannel]
}

func (operation Operation) outline(source *Image, output []byte) {
	for row := range source.height {
		for column := range source.width {
			if source.alpha(column, row) <= operation.threshold {
				continue
			}
			if source.alpha(column-1, row) > operation.threshold && source.alpha(column+1, row) > operation.threshold &&
				source.alpha(column, row-1) > operation.threshold && source.alpha(column, row+1) > operation.threshold {
				continue
			}
			operation.outlineNeighbors(source, output, column, row)
		}
	}
}

func (operation Operation) outlineNeighbors(source *Image, output []byte, x, y int) {
	for row := max(0, y-1); row <= min(source.height-1, y+1); row++ {
		for column := max(0, x-1); column <= min(source.width-1, x+1); column++ {
			index := (row*source.width + column) * channels
			if source.alpha(column, row) > operation.threshold || output[index+alphaChannel] != 0 {
				continue
			}
			paint := operation.first
			if operation.kind == operationCheckerOutline && (column+row+operation.frame)&1 != 0 {
				paint = operation.second
			}
			putColour(output[index:index+channels], paint)
		}
	}
}

func putColour(destination []byte, paint color.RGBA) {
	destination[0], destination[1], destination[2], destination[3] = paint.R, paint.G, paint.B, paint.A
}

// Silhouette creates binary white occupancy from alpha, ignoring source RGB.
func (source *Image) Silhouette(threshold byte) *Image {
	output := make([]byte, len(source.pixels))
	for index := 0; index < len(output); index += channels {
		if source.pixels[index+alphaChannel] > threshold {
			paint := color.RGBA{R: channelMaximum, G: channelMaximum, B: channelMaximum, A: channelMaximum}
			putColour(output[index:index+channels], paint)
		}
	}

	return &Image{width: source.width, height: source.height, pixels: output}
}
