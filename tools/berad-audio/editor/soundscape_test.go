package editorui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func soundscapeFixture(t *testing.T) *model {
	t.Helper()
	m := fixture(t)
	for _, cueID := range []string{"ambient/AmbientFire", "ambient/AmbientForest", "ambient/AmbientStream", "music/track/town"} {
		cue := m.doc.Cue(cueID).WithRecordings([]string{"healing_01"})
		require.NoError(t, m.doc.SetCue(cueID, cue))
	}
	require.NoError(t, m.doc.Save())
	return m
}

func selectSoundscapeMoment(t *testing.T, m *model, cueID string) {
	t.Helper()
	m.workspace, m.focus = workspaceMoments, focusBrowser
	m.search = ""
	m.refreshKeys()
	m.selectID(cueID)
	finish(m, m.loadSelection())
	require.NotNil(t, m.clip)
}

func TestSoundscapeTwoBedsMusicAndSFXContinueWhileBrowsing(t *testing.T) {
	m := soundscapeFixture(t)
	// The fixture's Dispel slot is deliberately empty; this test needs a second
	// audible SFX to exercise replacement, persistent slot gain/mute and layers.
	require.NoError(t, m.doc.SetCueRecordings("sfx/SoundDispel", []string{"healing_02"}))
	require.NoError(t, m.doc.Save())
	m.browserKey("m")
	require.True(t, m.soundscapeVisible, "empty soundscape must be discoverable")
	for _, cue := range []string{"ambient/AmbientFire", "ambient/AmbientForest", "music/track/town"} {
		selectSoundscapeMoment(t, m, cue)
		m.addMix()
	}
	require.Len(t, m.mix, 3)
	a1, a2, music := m.layerAt(ambientOne), m.layerAt(ambientTwo), m.layerAt(musicSlot)
	require.Equal(t, modeLoop, a1.mode)
	require.Equal(t, modeLoop, a2.mode)
	require.Equal(t, modeOnce, music.mode)
	selectSoundscapeMoment(t, m, "sfx/SoundHeal")
	finish(m, m.previewNextClip())
	fx := m.layerAt(sfxSlot)
	require.NotNil(t, fx)
	require.True(t, fx.voice.Playing())
	require.Same(t, fx.voice, m.previewVoice())
	fx.muted, fx.gain = true, 0.5
	selectSoundscapeMoment(t, m, "sfx/SoundDispel")
	require.True(t, fx.voice.Playing(), "browsing does not cut off the triggered sound")
	finish(m, m.previewNextClip())
	require.True(t, m.layerAt(sfxSlot).muted, "slot mute survives another SFX trigger")
	require.Equal(t, 0.5, m.layerAt(sfxSlot).gain)
	require.False(t, fx.voice.Active(), "the next SFX replaces the previous SFX voice")
	for _, layer := range []*mixLayer{a1, a2, music} {
		require.True(t, layer.voice.Playing())
		require.Equal(t, 1, layer.voice.(*fakeVoice).starts)
	}
	m.browserKey("m")
	finish(m, m.mixKey(tea.KeyMsg{Type: tea.KeyEsc}))
	require.Equal(t, focusBrowser, m.focus)
	require.True(t, m.soundscapeVisible)
	m.browserKey("d")
	require.False(t, m.soundscapeVisible)
	status := ansi.Strip(m.soundscapeStatus(160))
	for _, name := range []string{"Fire", "Forest", "Monarch", "Dispel"} {
		require.Contains(t, status, name)
	}
	require.False(t, m.doc.Dirty(), "soundscape never changes authored gains or assignments")
	m.close()
	for _, layer := range m.mix {
		require.False(t, layer.voice.Active())
	}
}

func TestSoundscapeAmbientReplacementIsExplicitAndCancellable(t *testing.T) {
	m := soundscapeFixture(t)
	for _, cue := range []string{"ambient/AmbientFire", "ambient/AmbientForest"} {
		selectSoundscapeMoment(t, m, cue)
		m.addMix()
	}
	first, second := m.layerAt(ambientOne), m.layerAt(ambientTwo)
	selectSoundscapeMoment(t, m, "ambient/AmbientStream")
	m.addMix()
	require.NotNil(t, m.pendingAmbient)
	require.True(t, first.voice.Playing())
	require.True(t, second.voice.Playing())
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	require.Nil(t, m.pendingAmbient)
	require.Same(t, second, m.layerAt(ambientTwo))
	m.addMix()
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m.key(tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, "ambient/AmbientStream", m.layerAt(ambientTwo).cueID)
	require.False(t, second.voice.Active())
	require.True(t, first.voice.Playing())
	require.Equal(t, 1, first.voice.(*fakeVoice).starts)
}

func TestSoundscapeMusicCompletionLoopAndStopAreDistinct(t *testing.T) {
	m := soundscapeFixture(t)
	selectSoundscapeMoment(t, m, "music/track/town")
	m.looping = true // A standalone audition toggle must not silently loop pinned music.
	m.addMix()
	music := m.layerAt(musicSlot)
	voice := music.voice.(*fakeVoice)
	require.False(t, voice.loop)
	voice.playing = false
	_, state := m.layerState(music)
	require.Equal(t, "FINISHED", state)
	m.updateMix()
	require.Equal(t, 1, voice.starts)
	m.mixCursor = int(musicSlot)
	finish(m, m.mixKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}))
	require.True(t, voice.loop)
	require.Contains(t, strings.Join(m.soundscapeLines(80, 14), "\n"), "audition only")
	finish(m, m.mixKey(tea.KeyMsg{Type: tea.KeySpace}))
	_, state = m.layerState(music)
	require.Equal(t, "PAUSED", state)
	finish(m, m.mixKey(tea.KeyMsg{Type: tea.KeySpace}))
	music.mode = modeRepeat
	m.stopAll()
	starts := voice.starts
	m.now = func() time.Time { return music.lastTrigger.Add(time.Hour) }
	m.updateMix()
	require.Equal(t, starts, voice.starts, "Stop all must not allow repeat to restart later")
	_, state = m.layerState(music)
	require.Equal(t, "STOPPED", state)
	require.Len(t, m.mix, 1)
	m.clearSoundscape()
	require.Empty(t, m.mix)
}

func TestSoundscapeEmptySlotIsSilentAndPreservesLayers(t *testing.T) {
	m := soundscapeFixture(t)
	for _, cue := range []string{"ambient/AmbientFire", "ambient/AmbientForest", "music/track/town"} {
		selectSoundscapeMoment(t, m, cue)
		dispatchRune(m, 'a')
	}
	selectSoundscapeMoment(t, m, "sfx/SoundHeal")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	fx := m.layerAt(sfxSlot)
	require.NotNil(t, fx)
	layers := append([]*mixLayer(nil), m.mix...)
	m.selectID("sfx/SoundDispel")
	finish(m, m.loadSelection())
	require.Equal(t, "field_dispel_01", m.recordingID())
	require.Nil(t, m.doc.Recording(m.recordingID()).File)
	require.Nil(t, m.clip)
	require.Equal(t, audio.RecordingNoFile, m.doc.FileAvailability(m.recordingID()).State)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.False(t, m.preview.Active(), "an empty slot cannot audition the previous loaded recording")
	require.Same(t, fx, m.layerAt(sfxSlot), "a silent selection does not replace the existing comparison voice")
	require.Len(t, m.mix, len(layers))
	for _, layer := range layers {
		require.Same(t, layer, m.layerAt(layer.slot))
		require.True(t, layer.voice.Playing())
		require.Equal(t, 1, layer.voice.(*fakeVoice).starts)
	}
	require.False(t, m.doc.Dirty())
}

type failingSoundscapeVoice struct{ fakeVoice }

func (v *failingSoundscapeVoice) Play(*audio.RecordingAudio, *audio.PlaybackWindow, bool, float64) error {
	return errors.New("test player failed")
}

func TestSoundscapeFailedReplacementPreservesPlayingSlot(t *testing.T) {
	m := soundscapeFixture(t)
	selectSoundscapeMoment(t, m, "music/track/town")
	m.addMix()
	old := m.layerAt(musicSlot)
	m.newVoice = func() playback { return &failingSoundscapeVoice{} }
	m.addMix()
	require.Same(t, old, m.layerAt(musicSlot))
	require.True(t, old.voice.Playing())
	require.Contains(t, m.status, "test player failed")
}
