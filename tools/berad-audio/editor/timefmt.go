package editorui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	secondsPerMinute = 60
	percentPerUnit   = 100
	msPerTenth       = 100
	shortSoundMs     = 10 * msPerSecond
)

// clock writes source milliseconds as minutes:seconds. Precise keeps the
// thousandths, which trimming needs; lists and the playback line do not.
func clock(milliseconds int64, precise bool) string {
	milliseconds = max(0, milliseconds)
	minutes := milliseconds / (secondsPerMinute * msPerSecond)
	seconds := milliseconds / msPerSecond % secondsPerMinute
	if !precise {
		return fmt.Sprintf("%d:%02d", minutes, seconds)
	}

	return fmt.Sprintf("%d:%02d.%03d", minutes, seconds, milliseconds%msPerSecond)
}

// brief writes a time within a sound of the given length: a sound under ten
// seconds keeps tenths, so a short effect does not read as 0:00.
func brief(milliseconds, length int64) string {
	if length >= shortSoundMs {
		return clock(milliseconds, false)
	}

	return fmt.Sprintf("%s.%d", clock(milliseconds, false), max(0, milliseconds)%msPerSecond/msPerTenth)
}

// parseClock reads "m:ss", "m:ss.mmm" or plain seconds into milliseconds.
func parseClock(text string) (int64, bool) {
	minutesText, secondsText, hasMinutes := strings.Cut(strings.TrimSpace(text), ":")
	if !hasMinutes {
		minutesText, secondsText = "0", minutesText
	}
	minutes, err := strconv.ParseInt(minutesText, 10, 64)
	if err != nil || minutes < 0 {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(secondsText, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 ||
		hasMinutes && seconds >= secondsPerMinute {
		return 0, false
	}

	return minutes*secondsPerMinute*msPerSecond + int64(math.Round(seconds*msPerSecond)), true
}

// percent writes a 0..1 gain as a whole percentage.
func percent(gain float64) string {
	return strconv.Itoa(int(math.Round(gain*percentPerUnit))) + "%"
}
