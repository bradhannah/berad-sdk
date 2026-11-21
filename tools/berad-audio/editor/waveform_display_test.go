package editorui

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestFooterPacksAcrossWidthWithoutDroppingTrimHints(t *testing.T) {
	model := fixture(t)
	wav := calibrationWAV(4000)
	wav = append(wav[:44], bytes.Repeat(wav[44:], 6)...)
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(wav)-44))
	path := filepath.Join(t.TempDir(), "three-seconds.wav")
	require.NoError(t, os.WriteFile(path, wav, 0o644))
	clip, err := model.doc.LoadRecording(context.Background(), path)
	require.NoError(t, err)
	require.Equal(t, int64(3000), clip.DurationMs())
	model.clip = clip
	model.width = 80
	narrow := model.footerLines()
	model.width = 180
	wide := model.footerLines()
	require.Less(t, len(wide), len(narrow))
	require.LessOrEqual(t, len(wide), 2, "wide terminals should not keep fixed short shortcut rows")
	for _, rows := range [][]string{narrow, wide} {
		plain := ansi.Strip(strings.Join(rows, "\n"))
		for _, hint := range []string{"Enter play", "+ - level", "i o trim", "? all keys", "q quit"} {
			require.Contains(t, plain, hint)
		}
	}
	model.beginInput(fieldStart, "0")
	require.Contains(t, ansi.Strip(strings.Join(model.footerLines(), "\n")), "m:ss.mmm or seconds")
}

func TestWaveformStaysModestSoTheListGetsTheHeight(t *testing.T) {
	model := fixture(t)
	model.width, model.height = 120, 40
	require.Equal(t, 8, model.layout().wave)
	require.GreaterOrEqual(t, model.listHeight(), 21)
	model.width, model.height = 160, 60
	require.LessOrEqual(t, model.layout().wave, maximumWaveHeight)
	model.width, model.height, model.focus = 60, 24, focusWave
	fillSoundscape(t, model)
	assertFullScreen(t, model)
	require.GreaterOrEqual(t, model.listHeight(), 1)
	require.GreaterOrEqual(t, model.layout().wave, smallestWaveHeight, "retain actual waveform rows on small screens")
}

func TestLogWaveformMakesQuietAudioVisibleAndPreservesDigitalSilence(t *testing.T) {
	const height = 8
	require.Zero(t, waveformDisplayUnits(0, height))
	require.Equal(t, "─", waveformCell(0, height-1, height))
	last := 0
	for _, peak := range []float64{1.0 / 32768, 0.001, 0.01, 0.1, 1} {
		units := waveformDisplayUnits(peak, height)
		require.Greater(t, units, 0)
		require.GreaterOrEqual(t, units, last)
		require.NotEqual(t, "─", waveformCell(units, height-1, height))
		last = units
	}
	require.GreaterOrEqual(t, waveformDisplayUnits(0.01, height), waveformCellSteps,
		"-40 dB audio should occupy more than a barely visible fractional cell")
	require.Equal(t, height*waveformCellSteps, waveformDisplayUnits(1, height))
	require.Zero(t, waveformDisplayUnits(math.NaN(), height))
	model := fixture(t)
	path := filepath.Join(t.TempDir(), "quiet.wav")
	require.NoError(t, os.WriteFile(path, calibrationWAV(32), 0o644))
	clip, err := model.doc.LoadRecording(context.Background(), path)
	require.NoError(t, err)
	model.clip = clip
	peak := clip.PeakBetween(0, clip.DurationMs())
	voice := model.preview.(*fakeVoice)
	beforeGain := voice.gain
	model.drawWaveform(audio.PlaybackWindow{StartMs: 0, EndMs: 40}, 0, 0, 40, 40, height)
	require.Equal(t, peak, clip.PeakBetween(0, clip.DurationMs()))
	require.Equal(t, beforeGain, voice.gain, "display scaling must never change playback gain")
	require.False(t, model.doc.Dirty())
}
