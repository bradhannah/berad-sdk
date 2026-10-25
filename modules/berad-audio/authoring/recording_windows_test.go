package authoring_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
)

// burstsFixture writes a stereo WAV of milliseconds, silent but for a square
// wave during each burst, given as [start, end) milliseconds.
func burstsFixture(t *testing.T, milliseconds int, bursts [][2]int) *authoring.RecordingAudio {
	t.Helper()
	const framesPerMs = authoring.EditorSampleRate / 1000
	frames := milliseconds * framesPerMs
	data := make([]byte, 44+frames*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], authoring.EditorSampleRate)
	binary.LittleEndian.PutUint32(data[28:], authoring.EditorSampleRate*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(frames*4))
	for _, burst := range bursts {
		for frame := burst[0] * framesPerMs; frame < burst[1]*framesPerMs; frame++ {
			sample := int16(4000)
			if frame%100 < 50 {
				sample = -sample
			}
			binary.LittleEndian.PutUint16(data[44+frame*4:], uint16(sample))
			binary.LittleEndian.PutUint16(data[44+frame*4+2:], uint16(sample))
		}
	}
	path := filepath.Join(t.TempDir(), "bursts.wav")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	clip, err := authoring.LoadRecording(path, ebitengine.Decode)
	require.NoError(t, err)

	return clip
}

func TestSuggestedWindowsFindEachSoundBetweenSilences(t *testing.T) {
	clip := burstsFixture(t, 1000, [][2]int{{100, 200}, {230, 300}, {600, 700}})
	require.Equal(t, []authoring.PlaybackWindow{{StartMs: 90, EndMs: 310}, {StartMs: 590, EndMs: 710}},
		clip.SuggestedWindows(100), "a 30 ms gap stays inside one sound")
	require.Equal(t, []authoring.PlaybackWindow{
		{StartMs: 90, EndMs: 210}, {StartMs: 220, EndMs: 310}, {StartMs: 590, EndMs: 710},
	}, clip.SuggestedWindows(20), "a shorter silence splits it, and padding never overlaps")
	whole, found := clip.SuggestedWindow()
	require.True(t, found)
	require.Equal(t, authoring.PlaybackWindow{StartMs: 90, EndMs: 710}, *whole, "the single window spans them all")
}

func TestSuggestedWindowsOfSilenceAreNone(t *testing.T) {
	require.Empty(t, burstsFixture(t, 300, nil).SuggestedWindows(50))
}
