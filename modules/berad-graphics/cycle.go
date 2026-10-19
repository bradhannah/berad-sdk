package graphics

import "fmt"

const maximumFrames = 65536

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
