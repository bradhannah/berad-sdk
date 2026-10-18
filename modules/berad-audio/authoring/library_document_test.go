package authoring

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/stretchr/testify/require"
)

func TestNeutralDocumentRegionHistoryAndFocusedSave(t *testing.T) {
	editor, _ := editorFixture(t)
	document := editor.Document()
	cue := document.Cues["sfx/SoundHeal"]
	document.Cues = map[string]Cue{"panel/accept": cue}
	document.Taxonomy.CueProfiles = map[string][]string{"panel": {"panel/accept"}}
	selected := "panel/accept"
	document.Routing = Routing{Ambient: map[string]AmbientProfile{}, Tiles: map[string]TileAudio{},
		Surfaces: map[string][]string{}, Emitters: map[string]string{},
		Groups: map[string]map[string]*string{"music": {"fallback": &selected, "quiet_room": nil}}}
	data, err := json.MarshalIndent(document, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(editor.Path(), data, 0o600))
	editor, err = OpenLibraryProject(editor.Path(), EditorOptions{Decode: ebitengine.Decode,
		Calibration: func(string) CalibrationOptions { return DefaultCalibrationOptions() },
		Targets: AuthoringTargets{Groups: map[string]AssignmentGroup{
			"music": {Label: "Station", DefaultTarget: "fallback"}}}})
	require.NoError(t, err)
	require.NoError(t, editor.Save())
	after, err := os.ReadFile(editor.Path())
	require.NoError(t, err)
	require.Equal(t, data, after, "no-op Save must preserve exact formatting")
	value, explicit := editor.ResolveAssignment("music", "hallway")
	require.Equal(t, selected, value)
	require.False(t, explicit)
	value, explicit = editor.ResolveAssignment("music", "quiet_room")
	require.Empty(t, value)
	require.True(t, explicit, "explicit silence is not inheritance")
	path, err := editor.RecordingPath("healing_01")
	require.NoError(t, err)
	clip, err := editor.LoadRecording(context.Background(), path)
	require.NoError(t, err)
	draft, err := editor.BeginRecordingRegion("healing_01", clip)
	require.NoError(t, err)
	window := &PlaybackWindow{StartMs: 20, EndMs: 60}
	preview, err := draft.Preview(window)
	require.NoError(t, err)
	require.NoError(t, editor.ApplyRecordingRegion(context.Background(), draft, window))
	require.Equal(t, preview.Window, editor.Recording("healing_01").PlaybackWindow)
	require.True(t, editor.Undo())
	require.True(t, editor.Redo())
	require.NoError(t, editor.Save())
	beforeRoots, afterRoots := objectSpans(data), objectSpans(editor.saved)
	require.Equal(t, beforeRoots["cues"].data, afterRoots["cues"].data)
	require.Equal(t, beforeRoots["routing"].data, afterRoots["routing"].data)
	reopened, err := OpenLibraryEditor(editor.Path())
	require.NoError(t, err)
	require.Equal(t, editor.Document(), reopened.Document())
	playback, err := reopened.RuntimeLibrary(1 << 20)
	require.NoError(t, err)
	require.NotNil(t, playback)
}

func TestRoutingV3GroupsRetainNullAndLargeIntegerValues(t *testing.T) {
	editor, _ := editorFixture(t)
	routing := Routing{Ambient: map[string]AmbientProfile{"sfx/SoundHeal": {FadeInMs: 9007199254740993}},
		Tiles: map[string]TileAudio{}, Surfaces: map[string][]string{}, Emitters: map[string]string{},
		Groups: map[string]map[string]*string{"music": {}, "dungeon": {"silent": nil}, "naval": {}}}
	require.NoError(t, editor.SetRouting(routing))
	require.NoError(t, editor.Save())
	original := bytes.Clone(editor.saved)
	routing = editor.Routing()
	selected := "sfx/SoundHeal"
	routing.Groups["music"]["new_target"] = &selected
	require.NoError(t, editor.SetRouting(routing))
	require.NoError(t, editor.Save())
	before := objectSpans(objectSpans(original)["routing"].data)
	after := objectSpans(objectSpans(editor.saved)["routing"].data)
	require.Equal(t, before["ambient"].data, after["ambient"].data, "preserve exact integers and untouched routing entries")
	reopened, err := OpenLibraryEditor(editor.Path())
	require.NoError(t, err)
	require.Equal(t, int64(9007199254740993), reopened.Routing().Ambient["sfx/SoundHeal"].FadeInMs)
	require.Contains(t, reopened.Routing().Groups["dungeon"], "silent")
	require.Nil(t, reopened.Routing().Groups["dungeon"]["silent"])
}

func TestSchemaThreeCannotSilentlyAcquireNewRoutingSections(t *testing.T) {
	editor, _ := editorFixture(t)
	revision := editor.Revision()
	routing := editor.Routing()
	routing.Groups = map[string]map[string]*string{"new_schema_section": {"target": nil}}
	require.Error(t, editor.SetRouting(routing))
	require.Equal(t, revision, editor.Revision())
	require.False(t, editor.Dirty())
	var decoded Routing
	require.Error(t, json.Unmarshal([]byte(`{"new_schema_section":{"target":null}}`), &decoded))
}

func TestDetachedDocumentAndPolicyCannotMutateEditorOrOverrideNewerWork(t *testing.T) {
	editor, _ := editorFixture(t)
	view, revision := editor.Document(), editor.Revision()
	view.Recordings["healing_01"] = Recording{}
	require.NotNil(t, editor.Recording("healing_01").File)
	require.Error(t, editor.ApplyDocument(revision, view))
	require.Equal(t, revision, editor.Revision())
	view = editor.Document()
	require.NoError(t, editor.SetRecordingLicense("healing_01", "unknown"))
	require.ErrorIs(t, editor.ApplyDocument(revision, view), ErrDocumentStale)
	_, err := OpenLibraryProject(editor.Path(), EditorOptions{Decode: ebitengine.Decode,
		Calibration: func(string) CalibrationOptions { return DefaultCalibrationOptions() },
		Validate:    func(document LibraryDocument) error { clear(document.Recordings); return nil }})
	require.NoError(t, err, "consumer validation gets detached state")
}

func TestProjectRootAndInvalidJSONAreExplicit(t *testing.T) {
	editor, _ := editorFixture(t)
	_, err := OpenLibraryProject(editor.Path(), EditorOptions{})
	require.Error(t, err)
	data := bytes.Clone(editor.saved)
	for _, invalid := range [][]byte{append(bytes.Clone(data), []byte(" {}")...),
		bytes.Replace(data, []byte(`"schemaVersion": 3`), []byte(`"schemaVersion": 3, "schemaVersion": 3`), 1),
		bytes.Replace(data, []byte(`"file": "signal.wav"`), []byte(`"fileTypo": "signal.wav"`), 1)} {
		_, err := parseLibraryDocument(invalid, nil)
		require.Error(t, err)
	}
	other := filepath.Join(t.TempDir(), "manifest.json")
	require.NoError(t, os.WriteFile(other, data, 0o600))
	reopened, err := OpenLibraryProject(other, EditorOptions{AssetRoot: filepath.Dir(editor.Path()), Decode: ebitengine.Decode,
		Calibration: func(string) CalibrationOptions { return DefaultCalibrationOptions() }})
	require.NoError(t, err)
	require.NoError(t, reopened.Validate())
}

func TestConsumerImportPolicyIsDetachedAndCannotEscapeAssetRoot(t *testing.T) {
	editor, wav := editorFixture(t)
	source := filepath.Join(t.TempDir(), "chosen.wav")
	require.NoError(t, os.WriteFile(source, wav, 0o600))
	options := EditorOptions{Decode: ebitengine.Decode,
		Calibration: func(string) CalibrationOptions { return DefaultCalibrationOptions() },
		ImportDestination: func(_ string, previous Recording, _ string, _ string) (string, error) {
			*previous.File = "changed-by-policy.wav"
			return "../escaped.wav", nil
		}}
	reopened, err := OpenLibraryProject(editor.Path(), options)
	require.NoError(t, err)
	before := reopened.Recording("healing_01")
	require.Error(t, reopened.ImportRecording("healing_01", source))
	require.Equal(t, before, reopened.Recording("healing_01"))
	require.Empty(t, reopened.RetainedImports())
	options.ImportDestination = func(id string, _ Recording, _ string, fingerprint string) (string, error) {
		return "project-imports/" + id + "_" + fingerprint[:12] + ".wav", nil
	}
	reopened, err = OpenLibraryProject(editor.Path(), options)
	require.NoError(t, err)
	require.NoError(t, reopened.ImportRecording("healing_01", source))
	require.Contains(t, *reopened.Recording("healing_01").File, "project-imports/")
	require.True(t, reopened.Undo())
	require.FileExists(t, reopened.RetainedImports()[0].Path)
}
