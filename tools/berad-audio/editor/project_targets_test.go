package editorui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestEditorUsesConsumerTargetsAndVocabulary(t *testing.T) {
	document := audio.LibraryDocument{SchemaVersion: 3,
		Recordings: map[string]audio.Recording{"signal": {Provenance: audio.RecordingProvenance{
			OriginalFilename: "unknown", SourceType: "unknown", SourceURL: "unknown", SourceCreator: "unknown", License: "unknown", ImportedFrom: "unknown", Processing: []audio.ProcessingStep{}}}},
		Cues: map[string]audio.Cue{"panel/accept": {Name: "Accept", RecordingID: "signal", VolumeModifier: 1, HowToHear: "Confirm a panel"}},
		Taxonomy: audio.Taxonomy{RecordingFamilies: map[string][]string{"signal": {"signal"}}, CueProfiles: map[string][]string{"panel": {"panel/accept"}},
			Profiles: map[string]audio.CueClassification{"panel": {TargetAcousticFamily: "signal", Phase: "accept", Outcome: "confirm", MixRole: "interface", Emitters: []string{"panel"}, Contexts: []string{"menu"}}}},
	}
	data, err := json.Marshal(document)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "library.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	targets := audio.AuthoringTargets{Tiles: []audio.TileTarget{{ID: "plate", Name: "Metal plate", Frames: []int{1}}}, Groups: map[string]audio.AssignmentGroup{"music": {Targets: map[string]string{"hangar": "Hangar"}}}, Families: []string{"signal"}, CueCategories: map[string]string{"panel/accept": "music"}}
	options := testAuthoringOptions()
	options.Targets = targets
	editor, err := audio.OpenLibraryProject(path, options)
	require.NoError(t, err)
	targets.Groups["music"].Targets["hangar"] = "mutated caller label"
	model := newModel(editor)
	t.Cleanup(model.close)
	require.Equal(t, "Hangar", model.targets.Groups["music"].Targets["hangar"])
	require.Len(t, model.routing.tiles, 1)
	require.Equal(t, "plate", model.routing.tiles[0].ID)
	require.Equal(t, []string{"signal"}, editor.AcousticFamilies())
	require.Equal(t, previewMusic, model.categoryForPreview("signal", "panel/accept"))
	require.True(t, model.matchesCueFacet("panel/accept", "category", "music"), "search must use the same supplied category as monitoring")
	require.Contains(t, model.View(), "signal")
}

func TestNeutralCueNamesWorkInVariantsAndSurfacePicker(t *testing.T) {
	m := fixture(t)
	document := m.doc.Document()
	const identity = "music/not-a-music-role"
	document.Cues[identity] = document.Cues["sfx/SoundHeal"]
	document.Taxonomy.CueProfiles["magic_activation"] = append(document.Taxonomy.CueProfiles["magic_activation"], identity)
	require.NoError(t, m.doc.ApplyDocument(m.doc.Revision(), document))
	finish(m, m.selectWorkspace(workspaceMoments))
	m.selectID(identity)
	finish(m, m.loadSelection())
	require.Equal(t, previewSFX, m.cuePreviewCategory(identity), "prefix must not silently determine category")
	m.beginDuplicate()
	require.NotNil(t, m.duplicate)
	require.Equal(t, identity, m.duplicate.eligibleCue)
	m.cancelCrop()
	finish(m, m.openSurfaceAddition())
	found := false
	for _, choice := range m.routing.menu.choices {
		found = found || choice.id == identity
	}
	require.True(t, found, "surface eligibility must use the same category policy")
}
