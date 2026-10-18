package authoring

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/stretchr/testify/require"
)

func regionFixture(t *testing.T) (*LibraryEditor, *RecordingAudio, *RecordingRegionDraft) {
	t.Helper()
	editor, _ := editorFixture(t)
	recording := editor.Recording("healing_01")
	gain := 0.0
	recording.CalibrationPolicy, recording.CalibrationGainDB = CalibrationAuto, &gain
	require.NoError(t, editor.SetRecording("healing_01", recording))
	path, err := editor.RecordingPath("healing_01")
	require.NoError(t, err)
	clip, err := LoadRecording(path, ebitengine.Decode)
	require.NoError(t, err)
	draft, err := editor.BeginRecordingRegion("healing_01", clip)
	require.NoError(t, err)
	return editor, clip, draft
}

func TestRegionPreviewApplyAndDuplicateUseIdenticalCalibration(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "apply", true: "duplicate"}[duplicate], func(t *testing.T) {
			editor, clip, draft := regionFixture(t)
			before, revision := encodeAudioValue(editor.library), editor.Revision()
			window := PlaybackWindow{StartMs: 20, EndMs: 60}
			preview, err := draft.Preview(&window)
			require.NoError(t, err)
			require.Equal(t, before, encodeAudioValue(editor.library))
			require.Equal(t, revision, editor.Revision())
			target := "healing_01"
			if duplicate {
				target = "region_test_01"
				err = editor.DuplicateRecordingRegion(context.Background(), draft, target, &window, "sfx/SoundHeal")
			} else {
				err = editor.ApplyRecordingRegion(context.Background(), draft, &window)
			}
			require.NoError(t, err)
			require.Equal(t, revision+1, editor.Revision())
			after := editor.Recording(target)
			require.Equal(t, preview.Window, after.PlaybackWindow)
			require.Equal(t, preview.CalibrationDB, *after.CalibrationGainDB)
			source, err := os.ReadFile(clip.Path())
			require.NoError(t, err)
			require.Equal(t, clip.data, source)
			require.True(t, editor.Undo())
			require.Equal(t, before, encodeAudioValue(editor.library), "one Undo restores metadata and membership together")
			require.True(t, editor.Redo())
			require.Equal(t, after, editor.Recording(target))
		})
	}
}

func TestRegionPreviewNeedsNoSourceIOAndCannotPublishMissingSource(t *testing.T) {
	editor, clip, draft := regionFixture(t)
	revision := editor.Revision()
	require.NoError(t, os.Remove(clip.Path()))
	window := PlaybackWindow{StartMs: 20, EndMs: 60}
	_, err := draft.Preview(&window)
	require.NoError(t, err, "live draft analysis uses the loaded immutable snapshot")
	require.ErrorIs(t, editor.ApplyRecordingRegion(context.Background(), draft, &window), ErrRecordingSourceChanged)
	require.Equal(t, revision, editor.Revision())
}

func TestRegionRejectsSameSizeSameTimestampContentReplacement(t *testing.T) {
	editor, clip, draft := regionFixture(t)
	info, err := os.Stat(clip.Path())
	require.NoError(t, err)
	replacement := bytes.Clone(clip.data)
	replacement[len(replacement)-1] ^= 1
	require.NoError(t, os.WriteFile(clip.Path(), replacement, info.Mode()))
	require.NoError(t, os.Chtimes(clip.Path(), info.ModTime(), info.ModTime()))
	require.NoError(t, clip.CheckSourceStamp(), "a cheap stamp is deliberately not a content guarantee")
	revision := editor.Revision()
	window := PlaybackWindow{StartMs: 20, EndMs: 60}
	require.ErrorIs(t, editor.ApplyRecordingRegion(context.Background(), draft, &window), ErrRecordingSourceChanged)
	require.ErrorIs(t, editor.DuplicateRecordingRegion(context.Background(), draft, "region_test_01", &window, ""),
		ErrRecordingSourceChanged)
	require.Equal(t, revision, editor.Revision())
	require.NotContains(t, editor.RecordingIDs(), "region_test_01")
}

func TestRegionRejectsStaleRevisionInvalidBoundsAndCanceledVerification(t *testing.T) {
	editor, _, draft := regionFixture(t)
	for _, window := range []PlaybackWindow{{StartMs: 70, EndMs: 20}, {StartMs: 0, EndMs: 101}} {
		_, err := draft.Preview(&window)
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, editor.ApplyRecordingRegion(ctx, draft, nil), context.Canceled)
	require.NoError(t, editor.SetRecordingLicense("healing_01", "unknown"))
	require.ErrorIs(t, editor.ApplyRecordingRegion(context.Background(), draft, nil), ErrRecordingRegionStale)
}
