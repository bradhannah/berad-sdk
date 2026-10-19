package editorui

import (
	"math"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestGameMixPreviewMatchesEachGameCategoryAtEverySlider(t *testing.T) {
	m := fixture(t)
	recording := m.doc.Recording("healing_01")
	db := -6.0
	recording.CalibrationGainDB = &db
	require.NoError(t, m.doc.SetRecording("healing_01", recording))
	calibration := math.Pow(10, db/20)
	for _, test := range []struct {
		cue      string
		category previewCategory
	}{
		{"sfx/SoundHeal", previewSFX},
		{"music/track/town", previewMusic},
		{"ambient/AmbientFire", previewAmbience},
	} {
		cue := m.doc.Cue(test.cue).WithRecordings([]string{"healing_01"})
		cue.VolumeModifier = 0.4
		require.NoError(t, m.doc.SetCue(test.cue, cue))
		for slider := 0; slider <= 10; slider++ {
			m.previewSliders[test.category] = slider
			expected := testSliderGain(slider) * calibration * cue.VolumeModifier
			require.InDelta(t, expected, m.auditionGain("healing_01", test.cue, true), 1e-12, "%s slider %d", test.cue, slider)
		}
	}
	m.referencePreview = true
	for _, cue := range []string{"sfx/SoundHeal", "music/track/town", "ambient/AmbientFire"} {
		require.InDelta(t, calibration, m.auditionGain("healing_01", cue, true), 1e-12)
	}
	require.Equal(t, 1.0, m.auditionGain("healing_01", "sfx/SoundHeal", false), "file candidates are uncalibrated")
}

func TestPreviewCategorySlidersAreIndependentLiveAndNeverSaved(t *testing.T) {
	m := fixture(t)
	require.Equal(t, [previewCategoryCount]int{7, 7, 7}, m.previewSliders)
	require.False(t, m.referencePreview)
	for _, cueID := range []string{"sfx/SoundHeal", "music/track/town", "ambient/AmbientFire"} {
		cue := m.doc.Cue(cueID).WithRecordings([]string{"healing_01"})
		require.NoError(t, m.doc.SetCue(cueID, cue))
		layer := &mixLayer{voice: &fakeVoice{}, clip: m.clip, recordingID: "healing_01", cueID: cueID, gain: 1, mode: modeOnce}
		m.mix = append(m.mix, layer)
		m.startMixLayer(layer)
	}
	require.NoError(t, m.doc.Save())
	before, err := os.ReadFile(m.doc.Path())
	require.NoError(t, err)
	sfx, music, ambient := m.mix[0].voice.(*fakeVoice), m.mix[1].voice.(*fakeVoice), m.mix[2].voice.(*fakeVoice)
	sfxGain, musicGain, ambientGain := sfx.gain, music.gain, ambient.gain
	m.focus = focusMix
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	require.True(t, m.volumeControls, "volume controls are reachable from the mixer")
	m.volumeCursor = int(previewMusic) + 1
	m.key(tea.KeyMsg{Type: tea.KeyLeft})
	require.Less(t, music.gain, musicGain)
	require.Equal(t, sfxGain, sfx.gain)
	require.Equal(t, ambientGain, ambient.gain)
	m.volumeCursor = int(previewSFX) + 1
	for range 20 {
		m.key(tea.KeyMsg{Type: tea.KeyLeft})
	}
	require.Zero(t, sfx.gain)
	m.volumeCursor = int(previewAmbience) + 1
	for range 20 {
		m.key(tea.KeyMsg{Type: tea.KeyRight})
	}
	require.Equal(t, 10, m.previewSliders[previewAmbience])
	require.Equal(t, 0.75*m.doc.Cue("ambient/AmbientFire").VolumeModifier, ambient.gain)
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	for _, voice := range []*fakeVoice{sfx, music, ambient} {
		require.Equal(t, 1.0, voice.gain, "Reference bypasses category and cue gain")
		require.Equal(t, 1, voice.starts, "changing monitoring volume must not restart playback")
	}
	m.mix[0].muted = true
	m.refreshMixGains()
	require.Zero(t, sfx.gain, "Reference still respects layer mute")
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.volumeControls)
	require.False(t, m.doc.Dirty())
	require.NoError(t, m.doc.Save())
	after, err := os.ReadFile(m.doc.Path())
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestRecordingCategoryInferenceAndExplicitOverride(t *testing.T) {
	m := fixture(t)
	require.Equal(t, previewMusic, m.categoryForPreview("ultima_theme", ""))
	require.Equal(t, previewAmbience, m.categoryForPreview("forest_nature_01", ""))
	require.Equal(t, previewSFX, m.categoryForPreview("cardboard_burning_01", ""), "mixed-use recording has a labelled SFX fallback")
	m.recordingCategory = previewAmbience
	require.Equal(t, previewAmbience, m.categoryForPreview("cardboard_burning_01", ""))
	require.Equal(t, previewSFX, m.categoryForPreview("cardboard_burning_01", "sfx/SoundFireCrackle"))
	require.Equal(t, previewAmbience, m.categoryForPreview("cardboard_burning_01", "ambient/AmbientFire"))
	selectHealMoment(t, m)
	finish(m, m.chooseFileOrAssignment())
	m.selectID("healing_02")
	m.previewSliders[previewSFX] = 7
	require.Equal(t, 0.55*m.doc.Cue("sfx/SoundHeal").VolumeModifier, m.gain(), "candidate uses destination cue category")
}

func TestPreviewControlsFitAndShowExactSliderMapping(t *testing.T) {
	m := fixture(t)
	m.openPreviewVolumes()
	for _, size := range [][2]int{{60, 24}, {100, 36}, {160, 70}} {
		m.width, m.height = size[0], size[1]
		assertFullScreen(t, m)
		view := ansi.Strip(m.View())
		require.Contains(t, view, "GAME MIX")
		require.Contains(t, view, "Music 7/10")
		require.Contains(t, view, "×0.55")
	}
	m.previewVolumeKey("r")
	require.Contains(t, ansi.Strip(m.View()), "REFERENCE")
	require.Contains(t, strings.Join(m.previewVolumeLines(60), "\n"), "Temporary")
}
