package editorui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestImportReceiptRemainsVisibleAfterUndo(t *testing.T) {
	m := fixture(t)
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "candidate.wav")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	finish(m, m.importFile(path))
	receipt := m.doc.LastPublication()
	require.True(t, receipt.Created)
	require.True(t, receipt.Assigned)
	require.Contains(t, m.status, "File retained:")
	finish(m, m.history(false))
	require.Contains(t, m.status, "remain on disk")
	require.FileExists(t, receipt.Path)
	require.Contains(t, strings.Join(m.detailLines(1000), "\n"), "Unassigned file:")
}

func TestStaleSaveCompletionPreservesDraftAndPublishedBaseline(t *testing.T) {
	m := fixture(t)
	m.setGain(-2)
	operation := m.save()
	recording := m.doc.Recording(m.recordingID())
	recording.Note = "newer input"
	require.NoError(t, m.doc.SetRecording(m.recordingID(), recording))
	finish(m, operation)
	require.Equal(t, "newer input", m.doc.Recording(m.recordingID()).Note)
	require.True(t, m.doc.Dirty())
	require.Contains(t, m.status, "current draft retained")
	finish(m, m.save())
	require.False(t, m.doc.Dirty(), "the real published baseline avoids a false external-edit conflict")
}

func TestReceiptExplainsRetainedUnassignedFileAndPlatformLimit(t *testing.T) {
	receipt := audio.PublicationOutcome{State: audio.PublicationSyncPending, Path: "sounds/imported/clip.wav",
		Import: true, Created: true, Assigned: false, DirectorySyncSupported: true}
	text := publicationReceiptText(receipt, true)
	require.Contains(t, text, "not assigned")
	require.Contains(t, text, receipt.Path)
	require.Contains(t, text, "unconfirmed")
	require.Contains(t, text, "Ctrl+s retries")
	receipt.State, receipt.DirectorySyncSupported = audio.PublicationComplete, false
	require.Contains(t, publicationReceiptText(receipt, false), "limited by this platform")
}
