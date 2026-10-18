package authoring

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalibrationProposalIsSourceBoundAndOneUndo(t *testing.T) {
	editor, _ := editorFixture(t)
	path, err := editor.RecordingPath("healing_01")
	require.NoError(t, err)
	clip, err := editor.LoadRecording(context.Background(), path)
	require.NoError(t, err)
	before, revision := editor.Recording("healing_01"), editor.Revision()
	draft, err := editor.BeginCalibration("healing_01", clip)
	require.NoError(t, err)
	require.Equal(t, revision, editor.Revision())
	require.Equal(t, before, editor.Recording("healing_01"))
	view := draft.Suggestion()
	view.GainDB = 100
	require.NotEqual(t, view.GainDB, draft.Suggestion().GainDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, editor.ApplyCalibration(ctx, draft), context.Canceled)
	require.Equal(t, revision, editor.Revision())
	require.NoError(t, editor.ApplyCalibration(context.Background(), draft))
	after := editor.Recording("healing_01")
	require.Equal(t, draft.Suggestion().GainDB, *after.CalibrationGainDB)
	require.Equal(t, CalibrationAuto, after.CalibrationMode())
	require.Equal(t, revision+1, editor.Revision())
	require.True(t, editor.Undo())
	require.Equal(t, before, editor.Recording("healing_01"))
	require.ErrorIs(t, editor.ApplyCalibration(context.Background(), draft), ErrDocumentStale)
}

func TestCalibrationProposalRejectsContentReplacement(t *testing.T) {
	editor, bytes := editorFixture(t)
	path, err := editor.RecordingPath("healing_01")
	require.NoError(t, err)
	clip, err := editor.LoadRecording(context.Background(), path)
	require.NoError(t, err)
	draft, err := editor.BeginCalibration("healing_01", clip)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	bytes[len(bytes)-1] ^= 1
	require.NoError(t, os.WriteFile(path, bytes, info.Mode()))
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
	require.ErrorIs(t, editor.ApplyCalibration(context.Background(), draft), ErrRecordingSourceChanged)
	require.False(t, editor.Dirty())
}
