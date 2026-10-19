package authoring

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/stretchr/testify/require"
)

func dirtyLibrary(t *testing.T) *LibraryEditor {
	t.Helper()
	editor, _ := editorFixture(t)
	cue := editor.Cue("sfx/SoundHeal")
	cue.VolumeModifier = 0.25
	require.NoError(t, editor.SetCue("sfx/SoundHeal", cue))
	return editor
}

func TestManifestDirectorySyncFailureIsPublishedAndRetryable(t *testing.T) {
	editor := dirtyLibrary(t)
	failure := errors.New("directory flush failed")
	calls := 0
	editor.persistence.syncDirectory = func(string) error { calls++; return failure }
	require.ErrorIs(t, editor.Save(), failure)
	require.False(t, editor.Dirty(), "published bytes are the conflict baseline")
	require.True(t, editor.NeedsSync(), "durability work must survive the error")
	require.Equal(t, PublicationSyncPending, editor.LastPublication().State)
	reopened, err := OpenLibraryEditor(editor.Path())
	require.NoError(t, err)
	require.Equal(t, 0.25, reopened.Cue("sfx/SoundHeal").VolumeModifier)
	published, err := os.ReadFile(editor.Path())
	require.NoError(t, err)
	editor.persistence.publish = func(string, string, []byte, os.FileMode, func(string) error) error {
		t.Fatal("durability retry must not rewrite an unchanged manifest")
		return nil
	}
	require.ErrorIs(t, editor.Save(), failure)
	require.Equal(t, 2, calls)
	editor.persistence.syncDirectory = func(string) error { calls++; return nil }
	require.NoError(t, editor.Save())
	require.False(t, editor.NeedsSync())
	require.Equal(t, 3, calls)
	after, err := os.ReadFile(editor.Path())
	require.NoError(t, err)
	require.Equal(t, published, after)
}

func TestPrepublicationFailureLeavesManifestAndBaselineUntouched(t *testing.T) {
	for _, stage := range []string{"temporary write", "file flush", "publication"} {
		t.Run(stage, func(t *testing.T) {
			editor := dirtyLibrary(t)
			before, err := os.ReadFile(editor.Path())
			require.NoError(t, err)
			failure := errors.New(stage)
			if stage == "publication" {
				editor.persistence.replace = func(string, string) error { return failure }
			} else {
				editor.persistence.publish = func(string, string, []byte, os.FileMode, func(string) error) error {
					return failure // The shared flushed-temporary-file boundary rejected publication.
				}
			}
			require.ErrorIs(t, editor.Save(), failure)
			require.True(t, editor.Dirty())
			require.False(t, editor.NeedsSync())
			require.Equal(t, PublicationNotWritten, editor.LastPublication().State)
			after, err := os.ReadFile(editor.Path())
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, before, editor.saved)
		})
	}
}

func TestCooperativePublisherLockAndStaleWriterConflict(t *testing.T) {
	first := dirtyLibrary(t)
	second, err := OpenLibraryEditor(first.Path())
	require.NoError(t, err)
	unlock, err := first.persistence.lock(first.Path())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, unlock()) })
	require.ErrorIs(t, second.Save(), ErrAudioLibraryBusy)
	unlock()
	require.NoError(t, first.Save())
	cue := second.Cue("sfx/SoundHeal")
	cue.VolumeModifier = 0.5
	require.NoError(t, second.SetCue("sfx/SoundHeal", cue))
	require.ErrorContains(t, second.Save(), "changed on disk")
	reopened, err := OpenLibraryEditor(first.Path())
	require.NoError(t, err)
	require.Equal(t, 0.25, reopened.Cue("sfx/SoundHeal").VolumeModifier)
}

func TestCloneDetachesDurabilityWork(t *testing.T) {
	editor := dirtyLibrary(t)
	editor.requireDirectorySync(filepath.Dir(editor.Path()))
	clone := editor.Clone()
	clone.persistence.syncDirectory = func(string) error { return nil }
	require.NoError(t, clone.flushPendingDirectories())
	require.True(t, editor.NeedsSync())
	require.False(t, clone.NeedsSync())
}

func externalImport(t *testing.T) (*LibraryEditor, string, []byte) {
	t.Helper()
	editor, wav := editorFixture(t)
	path := filepath.Join(t.TempDir(), "external.wav")
	require.NoError(t, os.WriteFile(path, wav, 0o644))
	return editor, path, wav
}

func TestImportPublishesCompleteBytesWithoutOverwritingAndUndoRetainsThem(t *testing.T) {
	editor, source, expected := externalImport(t)
	original := editor.Recording("healing_01")
	link := editor.persistence.link
	editor.persistence.link = func(temporary, destination string) error {
		bytes, err := os.ReadFile(temporary)
		require.NoError(t, err)
		require.Equal(t, expected, bytes, "only a complete flushed temporary file reaches publication")
		_, err = os.Stat(destination)
		require.True(t, os.IsNotExist(err))
		return link(temporary, destination)
	}
	require.NoError(t, editor.ImportRecording("healing_01", source))
	receipt := editor.LastPublication()
	require.True(t, receipt.Created)
	require.True(t, receipt.Assigned)
	require.Equal(t, PublicationComplete, receipt.State)
	require.True(t, editor.Undo())
	require.Equal(t, original, editor.Recording("healing_01"))
	retained := editor.RetainedImports()
	require.Equal(t, []ImportReceipt{{Path: receipt.Path, Referenced: false}}, retained)
	actual, err := os.ReadFile(receipt.Path)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	editor.persistence.link = link
	require.NoError(t, editor.ImportRecording("healing_01", source))
	require.False(t, editor.LastPublication().Created, "retry reuses matching content without replacing it")
	require.True(t, editor.RetainedImports()[0].Referenced)
}

func TestImportDirectoryFailureReportsRetainedUnassignedFileAndRetry(t *testing.T) {
	editor, source, _ := externalImport(t)
	before := editor.Recording("healing_01")
	failure := errors.New("import directory flush failed")
	editor.persistence.syncDirectory = func(string) error {
		if editor.publication.Created {
			return failure
		}
		return nil
	}
	require.ErrorIs(t, editor.ImportRecording("healing_01", source), ErrAudioPublicationSync)
	receipt := editor.LastPublication()
	require.Equal(t, PublicationSyncPending, receipt.State)
	require.True(t, receipt.Created)
	require.False(t, receipt.Assigned)
	require.True(t, editor.NeedsSync())
	require.Equal(t, before, editor.Recording("healing_01"))
	require.FileExists(t, receipt.Path)
	editor.persistence.syncDirectory = func(string) error { return nil }
	require.NoError(t, editor.ImportRecording("healing_01", source))
	require.False(t, editor.NeedsSync())
	require.True(t, editor.LastPublication().Assigned)
}

func TestImportRefusesCollidingContentAndPrevalidatesAutoGain(t *testing.T) {
	editor, source, wav := externalImport(t)
	clip, err := LoadRecording(source, ebitengine.Decode)
	require.NoError(t, err)
	destination := filepath.Join(filepath.Dir(editor.Path()),
		importRelativePath("healing_01", clip))
	require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
	require.NoError(t, os.WriteFile(destination, []byte("different existing file"), 0o644))
	require.ErrorContains(t, editor.ImportRecording("healing_01", source), "different contents")
	actual, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "different existing file", string(actual))
	require.False(t, editor.Dirty())
	second, silent, _ := externalImport(t)
	silence := bytes.Clone(wav)
	clear(silence[44:])
	require.NoError(t, os.WriteFile(silent, silence, 0o644))
	recording := second.Recording("healing_01")
	gain := 0.0
	recording.CalibrationPolicy, recording.CalibrationGainDB = CalibrationAuto, &gain
	require.NoError(t, second.SetRecording("healing_01", recording))
	require.Error(t, second.ImportRecording("healing_01", silent))
	require.Empty(t, second.RetainedImports(), "failed derived metadata does not leave an orphan copy")
	require.Equal(t, recording, second.Recording("healing_01"))
}

func TestImportPublicationFailureDoesNotExposePartialDestination(t *testing.T) {
	editor, source, _ := externalImport(t)
	failure := errors.New("link failed")
	editor.persistence.link = func(string, string) error { return failure }
	require.ErrorIs(t, editor.ImportRecording("healing_01", source), failure)
	require.Equal(t, PublicationNotWritten, editor.LastPublication().State)
	require.NoFileExists(t, editor.LastPublication().Path)
	require.Empty(t, editor.RetainedImports())
	files, err := os.ReadDir(filepath.Dir(editor.LastPublication().Path))
	require.NoError(t, err)
	require.Empty(t, files, "failed publication cleans the temporary file")
}

func TestAuditionedImportRejectsChangedSource(t *testing.T) {
	editor, source, bytes := externalImport(t)
	clip, err := LoadRecording(source, ebitengine.Decode)
	require.NoError(t, err)
	bytes[len(bytes)-1] ^= 1
	require.NoError(t, os.WriteFile(source, bytes, 0o644))
	require.ErrorIs(t, editor.ImportRecordingSource("healing_01", clip), ErrRecordingSourceChanged)
	require.False(t, editor.Dirty())
	require.Empty(t, editor.RetainedImports())
}

func TestStaleDocumentMergeRetainsPublishedBaselineAndDurability(t *testing.T) {
	editor := dirtyLibrary(t)
	completed := editor.Clone()
	completed.persistence.syncDirectory = func(string) error { return errors.New("sync failed") }
	require.Error(t, completed.Save())
	recording := editor.Recording("healing_01")
	recording.Note = "newer draft"
	require.NoError(t, editor.SetRecording("healing_01", recording))
	revision := editor.Revision()
	require.NoError(t, editor.MergePublication(completed))
	require.Equal(t, revision, editor.Revision())
	require.Equal(t, "newer draft", editor.Recording("healing_01").Note)
	require.True(t, editor.Dirty())
	require.True(t, editor.NeedsSync())
	require.NoError(t, editor.Save(), "published baseline is retained rather than misclassified as an external edit")
	require.False(t, editor.Dirty())
	require.False(t, editor.NeedsSync())
}

func TestSuccessfulPublicationDisclosesUnsupportedDirectoryFlush(t *testing.T) {
	editor := dirtyLibrary(t)
	editor.persistence.directorySyncSupported = false
	editor.persistence.syncDirectory = func(string) error { return nil }
	require.NoError(t, editor.Save())
	require.Equal(t, PublicationComplete, editor.LastPublication().State)
	require.False(t, editor.LastPublication().DirectorySyncSupported)
}

func TestPostpublicationErrorStillRetainsTheRealManifestBaseline(t *testing.T) {
	editor := dirtyLibrary(t)
	publish := editor.persistence.publish
	failure := errors.New("post-publication failure")
	publications := 0
	editor.persistence.publish = func(root, pattern string, data []byte, mode os.FileMode, install func(string) error) error {
		publications++
		require.NoError(t, publish(root, pattern, data, mode, install))
		return failure
	}
	require.ErrorIs(t, editor.Save(), failure)
	require.False(t, editor.Dirty())
	require.True(t, editor.NeedsSync())
	require.Equal(t, PublicationSyncPending, editor.LastPublication().State)
	require.NoError(t, editor.Save(), "retry flushes rather than invoking the failing publisher again")
	require.Equal(t, 1, publications)
	require.False(t, editor.NeedsSync())
}

func TestAudioLibraryLockIsSharedAcrossProcesses(t *testing.T) {
	editor := dirtyLibrary(t)
	unlock, err := editor.persistence.lock(editor.Path())
	require.NoError(t, err)
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestAudioLibraryLockChildProcess", "-test.count=1")
	command.Env = append(os.Environ(), "BERAD_TEST_AUDIO_LOCK_MANIFEST="+editor.Path())
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestAudioLibraryLockChildProcess(t *testing.T) {
	path := os.Getenv("BERAD_TEST_AUDIO_LOCK_MANIFEST")
	if path == "" {
		return
	}
	editor, err := OpenLibraryEditor(path)
	require.NoError(t, err)
	require.ErrorIs(t, editor.Save(), ErrAudioLibraryBusy)
}
