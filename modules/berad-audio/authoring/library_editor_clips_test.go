package authoring

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDuplicateRegionSharesFileAndAtomicallyAppendsOneUndoableVariant(t *testing.T) {
	editor, _ := editorFixture(t)
	require.NoError(t, editor.AddRecording("healing_03"))
	require.Equal(t, "unreviewed", editor.RecordingFamily("healing_03"))
	name, err := editor.NextRecordingID("healing_01")
	require.NoError(t, err)
	require.Equal(t, "healing_04", name)
	before := cloneAudioValue(editor.library)
	path, err := editor.RecordingPath("healing_01")
	require.NoError(t, err)
	bytesBefore, err := os.ReadFile(path)
	require.NoError(t, err)
	window := PlaybackWindow{StartMs: 20, EndMs: 60}
	clip, err := editor.LoadRecording(context.Background(), path)
	require.NoError(t, err)
	draft, err := editor.BeginRecordingRegion("healing_01", clip)
	require.NoError(t, err)
	require.NoError(t, editor.DuplicateRecordingRegion(context.Background(), draft, name, &window, "sfx/SoundHeal"))
	created := editor.Recording(name)
	require.Equal(t, editor.Recording("healing_01").File, created.File)
	require.Equal(t, editor.Recording("healing_01").Provenance, created.Provenance)
	require.Equal(t, &window, created.PlaybackWindow)
	require.Nil(t, editor.Recording("healing_01").PlaybackWindow, "the original clip is not edited")
	require.Equal(t, []string{"healing_01", "healing_04"}, editor.Cue("sfx/SoundHeal").Recordings())
	require.Contains(t, editor.Usages(name), "sfx/SoundHeal")
	require.True(t, editor.Undo())
	require.Equal(t, before, editor.library, "one undo removes both clip and membership")
	require.True(t, editor.Redo())
	require.NoError(t, editor.Save())
	reopened, err := OpenLibraryEditor(editor.Path())
	require.NoError(t, err)
	require.Equal(t, editor.Cue("sfx/SoundHeal"), reopened.Cue("sfx/SoundHeal"))
	bytesAfter, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bytesBefore, bytesAfter)
}

func TestDuplicateRegionRejectsInvalidBoundsNamesAndCyclesWithoutPartialEdits(t *testing.T) {
	editor, _ := editorFixture(t)
	path, err := editor.RecordingPath("healing_01")
	require.NoError(t, err)
	clip, err := editor.LoadRecording(context.Background(), path)
	require.NoError(t, err)
	draft, err := editor.BeginRecordingRegion("healing_01", clip)
	require.NoError(t, err)
	before := cloneAudioValue(editor.library)
	for _, test := range []struct {
		name, cue string
		window    PlaybackWindow
	}{
		{"healing_02", "", PlaybackWindow{StartMs: 10, EndMs: 20}},
		{"new", "", PlaybackWindow{StartMs: 70, EndMs: 20}},
		{"new", "", PlaybackWindow{StartMs: 0, EndMs: 1000}},
		{"new", "music/track/town", PlaybackWindow{StartMs: 0, EndMs: 20}},
	} {
		require.Error(t, editor.DuplicateRecordingRegion(context.Background(), draft, test.name, &test.window, test.cue))
	}
	require.Equal(t, before, editor.library)
	require.False(t, editor.Dirty())
	require.NoError(t, editor.AddRecording("footfall_009"))
	next, err := editor.NextRecordingID("footfall_009")
	require.NoError(t, err)
	require.Equal(t, "footfall_010", next)
}
