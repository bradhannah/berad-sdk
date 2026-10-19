package audio

import "math"

// QuadraticDistanceGain preserves the existing tile-audio profile: full gain
// within one caller unit, quadratic falloff outside it, silence at/beyond radius.
// Radius zero is a non-spatial bed. This is not inverse-square attenuation.
// Invalid or negative input is silent. The caller owns units and map topology.
func QuadraticDistanceGain(distance, radius float64) float64 {
	if !finite(distance) || !finite(radius) || distance < 0 || radius < 0 {
		return 0
	}
	if radius == 0 {
		return 1
	}
	if distance >= radius {
		return 0
	}
	if distance <= 1 {
		return 1
	}
	ratio := distance / radius

	return math.Max(0, 1-ratio*ratio)
}
