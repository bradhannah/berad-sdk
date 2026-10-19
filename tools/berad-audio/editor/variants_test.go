package editorui

import (
	"testing"

	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func selectHealMoment(t *testing.T, editor *model) {
	t.Helper()
	editor.workspace = workspaceMoments
	editor.refreshKeys()
	editor.selectID("sfx/SoundHeal")
	finish(editor, editor.loadSelection())
	require.NotNil(t, editor.clip)
}

func TestDuplicateDraftPreviewsBeforeAtomicCreateAndCanAppend(t *testing.T) {
	editor := fixture(t)
	selectHealMoment(t, editor)
	editor.beginDuplicate()
	require.NotNil(t, editor.duplicate)
	require.Equal(t, "healing_03", editor.duplicate.id)
	require.False(t, editor.doc.Dirty())
	editor.commitDuplicateField(fieldStart, "500")
	editor.commitDuplicateField(fieldEnd, "100")
	editor.duplicateKey("a")
	require.Nil(t, editor.applyCrop())
	require.False(t, editor.doc.Dirty(), "invalid bounds cannot create half a clip or group membership")
	editor.commitDuplicateField(fieldEnd, "700")
	editor.duplicateKey("p")
	voice := editor.preview.(*fakeVoice)
	require.Equal(t, &audio.PlaybackWindow{StartMs: 500, EndMs: 700}, voice.window)
	require.False(t, editor.doc.Dirty(), "audition is not Create")
	finish(editor, editor.applyCrop())
	require.Nil(t, editor.duplicate)
	require.Equal(t, []string{"healing_01", "healing_03"}, editor.doc.Cue("sfx/SoundHeal").Recordings())
	require.Equal(t, editor.doc.Recording("healing_01").File, editor.doc.Recording("healing_03").File)
	require.True(t, editor.doc.Undo())
	require.False(t, editor.doc.Dirty(), "one undo restores both the recording list and cue")
}

func TestMomentPreviewCyclesLocallyAndVariantsRemainEditable(t *testing.T) {
	editor := fixture(t)
	require.NoError(t, editor.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, editor)
	for _, want := range []string{"healing_02", "healing_01", "healing_02", "healing_01"} {
		finish(editor, editor.previewNextClip())
		require.Equal(t, want, editor.recordingID())
	}
	finish(editor, editor.openVariants())
	require.Equal(t, []string{"healing_01", "healing_02"}, editor.keys)
	finish(editor, editor.editVariants(1, false))
	require.Equal(t, []string{"healing_02", "healing_01"}, editor.doc.Cue("sfx/SoundHeal").Recordings())
	finish(editor, editor.editVariants(0, true))
	require.Len(t, editor.doc.Cue("sfx/SoundHeal").Recordings(), 1)
	require.Empty(t, editor.doc.Cue("sfx/SoundHeal").Selection, "one remaining clip becomes a normal assignment")
}

func TestCueAvailabilityIncludesEveryCycleMember(t *testing.T) {
	editor := fixture(t)
	require.NoError(t, editor.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, editor)
	editor.availability["healing_01"] = audio.RecordingAvailability{State: audio.RecordingReady, Exists: true}
	editor.availability["healing_02"] = audio.RecordingAvailability{State: audio.RecordingMissing, Exists: false, Reason: "missing file"}
	status := editor.itemStatus("sfx/SoundHeal")
	require.Equal(t, audio.RecordingMissing, status.State)
	require.False(t, status.Exists)
	require.Contains(t, status.Reason, "healing_02")
}

func TestVariantPreviewDoesNotAdvanceWhenCanceled(t *testing.T) {
	editor := fixture(t)
	require.NoError(t, editor.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, editor)
	finish(editor, editor.previewNextClip())
	require.Equal(t, 1, editor.currentVariant["sfx/SoundHeal"])
	pending := editor.previewNextClip()
	require.NotNil(t, pending)
	editor.loadSelection() // A new request invalidates the pending next-clip preview.
	editor.Update(pending())
	require.Equal(t, 1, editor.currentVariant["sfx/SoundHeal"], "only successful preview advances the actual selection")
}
