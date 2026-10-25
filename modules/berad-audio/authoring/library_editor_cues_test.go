package authoring

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func section(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	span, exists := objectSpans(data)[name]
	require.True(t, exists, name)
	return span.data
}

func TestCreateAndDeleteCueKeepOtherEntriesByteForByte(t *testing.T) {
	editor, _ := editorFixture(t)
	before, err := os.ReadFile(editor.Path())
	require.NoError(t, err)
	cue := Cue{Name: "New cue", RecordingID: "healing_02", VolumeModifier: 1, HowToHear: "Press it"}
	require.NoError(t, editor.CreateCue("sfx/new", cue, ""))
	require.Equal(t, UnclassifiedProfile, editor.CueProfile("sfx/new"))
	require.ErrorIs(t, editor.CreateCue("sfx/new", cue, ""), ErrAudioLibrary, "an ID is never replaced")
	require.ErrorIs(t, editor.CreateCue("sfx/other", cue, "missing"), ErrAudioLibrary)
	require.NoError(t, editor.SaveChanged())
	created, err := os.ReadFile(editor.Path())
	require.NoError(t, err)
	require.Equal(t, section(t, before, "recordings"), section(t, created, "recordings"))
	require.Contains(t, string(created), "\n    \"sfx/new\": {", "a new cue starts its own line")
	require.Contains(t, string(created), string(section(t, section(t, before, "cues"), "sfx/SoundHeal")))
	require.Contains(t, string(created), "\n      \"unclassified\": [", "new taxonomy entries are indented like their neighbours")
	_, err = ParseDocument(created, nil)
	require.NoError(t, err)

	require.NoError(t, editor.DeleteCue("sfx/new"))
	require.NoError(t, editor.SaveChanged())
	deleted, err := os.ReadFile(editor.Path())
	require.NoError(t, err)
	require.Equal(t, section(t, before, "cues"), section(t, deleted, "cues"), "deleting restores the other entries exactly")
	require.Equal(t, section(t, section(t, before, "taxonomy"), "cueProfiles"),
		section(t, section(t, deleted, "taxonomy"), "cueProfiles"))
	require.Contains(t, editor.RecordingIDs(), "healing_02", "the deleted cue's recording stays")
	require.ErrorIs(t, editor.DeleteCue("sfx/new"), ErrAudioLibrary)
}

func TestDeleteCueRefusesWhileRoutingNamesIt(t *testing.T) {
	editor, _ := editorFixture(t)
	routing := editor.Routing()
	routing.Ambient = map[string]AmbientProfile{"sfx/SoundHeal": {RadiusTiles: 4}}
	routing.Tiles = map[string]TileAudio{"fountain": {AmbientCue: "sfx/SoundHeal"}}
	require.NoError(t, editor.SetRouting(routing))
	require.Equal(t, []string{"tile fountain"}, editor.RoutingUses("sfx/SoundHeal"))
	require.ErrorIs(t, editor.DeleteCue("sfx/SoundHeal"), ErrAudioLibrary)

	routing.Tiles = nil
	require.NoError(t, editor.SetRouting(routing))
	require.Empty(t, editor.RoutingUses("sfx/SoundHeal"))
	require.NoError(t, editor.DeleteCue("sfx/SoundHeal"))
	require.Empty(t, editor.Routing().Ambient, "the cue's own ambient settings go with it")
	require.Equal(t, "", editor.CueProfile("sfx/SoundHeal"))
	require.Contains(t, editor.ClassificationProfiles(), "panel", "a profile no cue uses is kept")
}

func TestDeleteRecordingOnlyWhenNoCuePlaysIt(t *testing.T) {
	editor, _ := editorFixture(t)
	require.ErrorIs(t, editor.DeleteRecording("healing_01"), ErrAudioLibrary)
	require.NoError(t, editor.DeleteRecording("healing_02"))
	require.Equal(t, []string{"healing_01"}, editor.RecordingIDs())
	require.Equal(t, "", editor.RecordingFamily("healing_02"))
	require.NoError(t, editor.SaveChanged())
	_, err := os.Stat(filepath.Join(filepath.Dir(editor.Path()), "signal.wav"))
	require.NoError(t, err, "files stay on disk")
}

func TestSetRecordingProvenanceNeedsNoSource(t *testing.T) {
	editor, _ := editorFixture(t)
	require.NoError(t, os.Remove(filepath.Join(filepath.Dir(editor.Path()), "signal.wav")))
	provenance := editor.Recording("healing_01").Provenance
	provenance.SourceCreator, provenance.License = "A. Composer", "CC0-1.0"
	require.NoError(t, editor.SetRecordingProvenance("healing_01", provenance))
	require.Equal(t, "CC0-1.0", editor.Recording("healing_01").Provenance.License)
	provenance.Processing = nil
	require.ErrorIs(t, editor.SetRecordingProvenance("healing_01", provenance), ErrAudioLibrary)
	require.ErrorIs(t, editor.SetRecordingProvenance("missing", provenance), ErrAudioLibrary)
}

func TestSaveChangedDecodesOnlyWhatChanged(t *testing.T) {
	editor, wav := editorFixture(t)
	source := filepath.Join(filepath.Dir(editor.Path()), "signal.wav")
	require.NoError(t, os.Remove(source))
	cue := editor.Cue("sfx/SoundHeal")
	cue.VolumeModifier = 0.5
	require.NoError(t, editor.SetCue("sfx/SoundHeal", cue))
	require.Error(t, editor.Clone().Save(), "Save decodes every assigned source")
	require.NoError(t, editor.SaveChanged(), "an unchanged missing source does not block saving")

	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(editor.Path()), "other.wav"), []byte("not audio"), 0o600))
	recording := editor.Recording("healing_02")
	other := "other.wav"
	recording.File = &other
	require.NoError(t, editor.SetRecording("healing_02", recording))
	require.Error(t, editor.SaveChanged(), "a changed file is decoded")
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(editor.Path()), "other.wav"), wav, 0o600))
	require.NoError(t, editor.SaveChanged())
}

func TestChangedOnDiskIsItsOwnError(t *testing.T) {
	editor, _ := editorFixture(t)
	require.NoError(t, editor.CheckDisk())
	data, err := os.ReadFile(editor.Path())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(editor.Path(), append(data, '\n'), 0o600))
	require.ErrorIs(t, editor.CheckDisk(), ErrLibraryChangedOnDisk)
	cue := editor.Cue("sfx/SoundHeal")
	cue.VolumeModifier = 0.5
	require.NoError(t, editor.SetCue("sfx/SoundHeal", cue))
	err = editor.SaveChanged()
	require.ErrorIs(t, err, ErrLibraryChangedOnDisk)
	require.ErrorIs(t, err, ErrAudioLibrary, "it is still an authoring error")
}

func TestCreateLibraryThenBuildIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	require.NoError(t, CreateLibrary(path))
	require.Error(t, CreateLibrary(path), "an existing library is never replaced")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = ParseDocument(data, nil)
	require.NoError(t, err)

	editor, err := OpenLibraryEditor(path)
	require.NoError(t, err)
	require.NoError(t, editor.AddRecording("chime"))
	require.NoError(t, editor.CreateCue("sfx/chime", Cue{Name: "Chime", RecordingID: "chime", VolumeModifier: 1,
		HowToHear: "Play it"}, ""))
	require.NoError(t, editor.SaveChanged())
	reopened, err := OpenLibraryEditor(path)
	require.NoError(t, err)
	require.Equal(t, []string{"sfx/chime"}, reopened.CueIDs())
	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(saved, []byte("{\n  \"schemaVersion\": 4,\n")))
	require.False(t, reopened.Recording("chime").Provenance.CommercialUseAllowed(), "a new slot is not cleared")
	require.Equal(t, 1, strings.Count(string(saved), "\"sfx/chime\": {"))
}
