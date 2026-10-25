package authoring_test

// The format guard keeps schema-3, 4 and 5 libraries and version-1 and 2 project
// descriptors exactly as every editor and game reading them expects.
// testdata/format holds one of each using every field. A change that renames,
// retypes, adds or drops an encoded field fails here; it needs a new schema or
// descriptor version with a step-by-step upgrade, not an edit to these files.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
)

func readFormatFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "format", name))
	require.NoError(t, err)

	return data
}

// requireEncodes checks that a decoded value encodes to exactly the fixture's
// bytes, which is what Save writes for an edited entry.
func requireEncodes(t *testing.T, fixture json.RawMessage, value any, what string) {
	t.Helper()
	var want bytes.Buffer
	require.NoError(t, json.Compact(&want, fixture))
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.Equal(t, want.String(), string(encoded), what)
}

// requireLibraryEncodes checks that every entry of a library fixture encodes
// back to exactly its bytes.
func requireLibraryEncodes(t *testing.T, data []byte) authoring.LibraryDocument {
	t.Helper()
	document, err := authoring.ParseDocument(data, nil)
	require.NoError(t, err)
	var fixture struct {
		SchemaVersion int                        `json:"schemaVersion"`
		Recordings    map[string]json.RawMessage `json:"recordings"`
		Cues          map[string]json.RawMessage `json:"cues"`
		Taxonomy      json.RawMessage            `json:"taxonomy"`
		Routing       json.RawMessage            `json:"routing"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Len(t, document.Recordings, len(fixture.Recordings))
	require.Len(t, document.Cues, len(fixture.Cues))
	for id, entry := range fixture.Recordings {
		requireEncodes(t, entry, document.Recordings[id], "recording "+id)
	}
	for id, entry := range fixture.Cues {
		requireEncodes(t, entry, document.Cues[id], "cue "+id)
	}
	requireEncodes(t, fixture.Taxonomy, document.Taxonomy, "taxonomy")
	requireEncodes(t, fixture.Routing, document.Routing, "routing")
	require.Equal(t, fixture.SchemaVersion, document.SchemaVersion)

	return document
}

func TestSchemaThreeFormatIsFrozen(t *testing.T) {
	document := requireLibraryEncodes(t, readFormatFixture(t, "library.json"))
	requireFixtureValues(t, document)
	for id, recording := range document.Recordings {
		require.Nil(t, recording.Provenance.CommercialUse, id+": schema 3 records no commercial use")
	}
}

func TestSchemaFourFormatIsFrozen(t *testing.T) {
	document := requireLibraryEncodes(t, readFormatFixture(t, "library4.json"))
	requireFixtureValues(t, document)
	for id, cleared := range map[string]bool{"door_open_a": true, "door_open_b": true,
		"panel_signal": false, "river_loop": false} {
		require.Equal(t, cleared, *document.Recordings[id].Provenance.CommercialUse, id)
		require.Equal(t, cleared, document.Recordings[id].Provenance.CommercialUseAllowed(), id)
	}
}

func requireFixtureValues(t *testing.T, document authoring.LibraryDocument) {
	t.Helper()
	// Independent values, so a field decoding into the wrong place fails too.
	door := document.Recordings["door_open_a"]
	require.Equal(t, "sounds/door_open_a.wav", *door.File)
	require.InDelta(t, -4.5, *door.CalibrationGainDB, 0)
	require.Equal(t, authoring.CalibrationAuto, door.CalibrationPolicy)
	require.Equal(t, authoring.PlaybackWindow{StartMs: 120, EndMs: 1180}, *door.PlaybackWindow)
	require.Equal(t, "example-editor", door.Provenance.Processing[0].Tool)
	require.Equal(t, []string{"creak", "hinge"}, door.SearchTerms)
	require.Nil(t, document.Recordings["panel_signal"].File, "null is an intentional silent slot")
	cycle := document.Cues["door/open"]
	require.Equal(t, []string{"door_open_a", "door_open_b"}, cycle.RecordingIDs)
	require.Equal(t, "cycle", cycle.Selection)
	require.InDelta(t, 0.8, cycle.VolumeModifier, 0)
	require.Equal(t, "river_loop", document.Cues["bed/river"].RecordingID)
	require.Equal(t, []string{"gate"}, document.Taxonomy.ProfileSearchTerms["door"])
	require.InDelta(t, 8, document.Routing.Ambient["bed/river"].RadiusTiles, 0)
	require.InDelta(t, 4, *document.Routing.Tiles["water"].RadiusTiles, 0)
	require.Equal(t, "panel/accept", *document.Routing.Groups["music"]["fallback"])
	quiet, explicit := document.Routing.Groups["music"]["quiet"]
	require.True(t, explicit)
	require.Nil(t, quiet, "explicit silence stays distinct from a missing target")
}

func TestSchemaFiveFormatIsFrozen(t *testing.T) {
	document := requireLibraryEncodes(t, readFormatFixture(t, "library5.json"))
	requireFixtureValues(t, document)
	require.InDelta(t, 2.5, document.Cues["bed/river"].VolumeModifier, 0, "schema 5 plays a cue louder than unity")
	require.True(t, *document.Recordings["door_open_a"].Provenance.CommercialUse)
}

func TestLouderCuesBelongToSchemaFive(t *testing.T) {
	early := bytes.Replace(readFormatFixture(t, "library5.json"), []byte(`"schemaVersion": 5`),
		[]byte(`"schemaVersion": 4`), 1)
	_, err := authoring.ParseDocument(early, nil)
	require.ErrorIs(t, err, authoring.ErrLibrary, "schema 4 keeps a cue's gain at or below 1")
	tooLoud := bytes.Replace(readFormatFixture(t, "library5.json"), []byte(`"volumeModifier": 2.5`),
		[]byte(`"volumeModifier": 4`), 1)
	_, err = authoring.ParseDocument(tooLoud, nil)
	require.ErrorIs(t, err, authoring.ErrLibrary, "schema 5 stops at +12 dB")
	require.InDelta(t, 1, authoring.MaxCueGain(authoring.LibrarySchemaFour), 0)
	require.InDelta(t, 3.981, authoring.MaxCueGain(authoring.LibrarySchemaFive), 0.001)
}

func TestSchemaThreeFourAndFiveAreTheOnlyLibraryVersions(t *testing.T) {
	for _, change := range []struct{ fixture, from, to string }{
		{"library.json", `"schemaVersion": 3`, `"schemaVersion": 2`},
		{"library5.json", `"schemaVersion": 5`, `"schemaVersion": 6`},
	} {
		data := bytes.Replace(readFormatFixture(t, change.fixture), []byte(change.from), []byte(change.to), 1)
		_, err := authoring.ParseDocument(data, nil)
		require.ErrorIs(t, err, authoring.ErrLibrary, change.to)
	}
}

func TestCommercialUseBelongsToSchemaFour(t *testing.T) {
	early := bytes.Replace(readFormatFixture(t, "library4.json"), []byte(`"schemaVersion": 4`),
		[]byte(`"schemaVersion": 3`), 1)
	_, err := authoring.ParseDocument(early, nil)
	require.ErrorIs(t, err, authoring.ErrLibrary, "schema 3 has no commercialUse")
	missing := bytes.Replace(readFormatFixture(t, "library.json"), []byte(`"schemaVersion": 3`),
		[]byte(`"schemaVersion": 4`), 1)
	_, err = authoring.ParseDocument(missing, nil)
	require.ErrorIs(t, err, authoring.ErrLibrary, "schema 4 requires commercialUse")
}

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.json")
	require.NoError(t, os.WriteFile(path, readFormatFixture(t, name), 0o600))

	return path
}

func openFixture(t *testing.T, path string, schema int) *authoring.LibraryEditor {
	t.Helper()
	options := authoring.DefaultEditorOptions(ebitengine.Decode)
	options.SchemaVersion = schema
	editor, err := authoring.OpenLibraryProject(path, options)
	require.NoError(t, err)

	return editor
}

func TestUpgradeToSchemaFourChangesOnlyRecordingsAndVersion(t *testing.T) {
	path := copyFixture(t, "library.json")
	before := readFormatFixture(t, "library.json")
	editor := openFixture(t, path, authoring.LibrarySchemaFour)
	require.Equal(t, authoring.LibrarySchemaFour, editor.SchemaVersion())
	require.True(t, editor.Dirty(), "the upgrade waits for the next save")
	require.False(t, editor.Undo(), "Undo cannot take the upgrade back")
	require.NoError(t, editor.SaveChanged())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	document, err := authoring.ParseDocument(after, nil)
	require.NoError(t, err)
	require.Equal(t, authoring.LibrarySchemaFour, document.SchemaVersion)
	for id, recording := range document.Recordings {
		require.False(t, recording.Provenance.CommercialUseAllowed(), id+" is not cleared until its author says so")
	}
	for _, name := range []string{"cues", "taxonomy", "routing"} {
		require.Equal(t, rawSection(t, before, name), rawSection(t, after, name), name)
	}
	_, err = authoring.OpenLibraryProject(path, func() authoring.EditorOptions {
		options := authoring.DefaultEditorOptions(ebitengine.Decode)
		options.SchemaVersion = authoring.LibrarySchemaThree
		return options
	}())
	require.ErrorIs(t, err, authoring.ErrLibrary, "a library never moves back a schema")
}

func TestUpgradeToSchemaFiveChangesOnlyTheVersionFromFour(t *testing.T) {
	path := copyFixture(t, "library4.json")
	before := readFormatFixture(t, "library4.json")
	editor := openFixture(t, path, authoring.LibrarySchemaFive)
	require.Equal(t, authoring.LibrarySchemaFive, editor.SchemaVersion())
	require.NoError(t, editor.SaveChanged())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(bytes.Replace(before, []byte(`"schemaVersion": 4`), []byte(`"schemaVersion": 5`), 1)),
		string(after), "every byte but the version is kept")
}

func TestUpgradeFromThreeToFiveTakesEveryStep(t *testing.T) {
	path := copyFixture(t, "library.json")
	editor := openFixture(t, path, authoring.LibrarySchemaFive)
	require.NoError(t, editor.SaveChanged())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	document, err := authoring.ParseDocument(after, nil)
	require.NoError(t, err)
	require.Equal(t, authoring.LibrarySchemaFive, document.SchemaVersion)
	for id, recording := range document.Recordings {
		require.NotNil(t, recording.Provenance.CommercialUse, id+" gained schema 4's answer on the way")
		require.False(t, recording.Provenance.CommercialUseAllowed(), id)
	}
}

func TestCreatedLibrariesAreSchemaFive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	require.NoError(t, authoring.CreateLibrary(path))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	document, err := authoring.ParseDocument(data, nil)
	require.NoError(t, err)
	require.Equal(t, authoring.LibrarySchemaFive, document.SchemaVersion)
}

func TestSchemaThreeStaysSchemaThreeUnlessAsked(t *testing.T) {
	path := copyFixture(t, "library.json")
	editor := openFixture(t, path, 0)
	require.False(t, editor.Dirty())
	require.NoError(t, editor.AddRecording("new_slot"))
	require.NoError(t, editor.SaveChanged())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(after), `"schemaVersion": 3`)
	require.NotContains(t, string(after), "commercialUse")
}

func rawSection(t *testing.T, data []byte, name string) string {
	t.Helper()
	var sections map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &sections))

	return string(sections[name])
}

func TestDescriptorVersionOneFormatIsFrozen(t *testing.T) {
	path := filepath.Join("testdata", "format", "project.json")
	descriptor, err := authoring.ReadProjectDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, authoring.ProjectDescriptor{
		SchemaVersion: 1, Library: "library.json", AssetRoot: "media", ImportDirectory: "imported",
		Targets: authoring.AuthoringTargets{
			Tiles: []authoring.TileTarget{{ID: "water", Name: "Water", Frames: []int{3, 4}}},
			Groups: map[string]authoring.AssignmentGroup{"music": {
				Label: "Place", Targets: map[string]string{"fallback": "Anywhere", "quiet": "Quiet room"},
				DefaultTarget: "fallback", Category: "music",
			}},
			Families:      []string{"water", "wood"},
			CueCategories: map[string]string{"panel/accept": "music", "door/open": "sfx"},
		},
		Calibration: &authoring.CalibrationOptions{Method: authoring.ActiveRMS, TargetDB: -20},
		CalibrationFamilies: map[string]authoring.CalibrationOptions{
			"water": {Method: authoring.GatedLoudness, TargetDB: -28},
		},
		SliderGains:       []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1},
		RecordingFamilies: map[string]string{"water": "ambient"},
		TargetPolicies: map[string]authoring.TargetPolicy{"music|quiet": {
			Category: "music", Spatial: false, Background: false, Title: "Quiet room", Help: "No music here.",
		}},
		Ambient: &authoring.AmbientPreset{
			CuePrefix: "bed/", Profile: "river", Family: "water", HowToHear: "Stand near it.", Gain: 0.3,
		},
		MixLabel: "PROJECT MIX", SavedNotice: "Saved.",
	}, descriptor)
	require.Equal(t, filepath.Join("testdata", "format", "library.json"), descriptor.LibraryPath(path))
}

func TestDescriptorVersionTwoFormatIsFrozen(t *testing.T) {
	path := filepath.Join("testdata", "format", "project2.json")
	descriptor, err := authoring.ReadProjectDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, authoring.ProjectDescriptorVersionTwo, descriptor.SchemaVersion)
	require.Equal(t, map[string]float64{"ambient": -3, "music": 2}, descriptor.CategoryGainsDB)
	require.InDelta(t, -3, descriptor.CategoryGainDB(authoring.CategoryAmbient), 0)
	require.InDelta(t, 0, descriptor.CategoryGainDB(authoring.CategorySFX), 0, "a category it does not name is 0 dB")
	one, err := authoring.ReadProjectDescriptor(filepath.Join("testdata", "format", "project.json"))
	require.NoError(t, err)
	one.SchemaVersion, one.CategoryGainsDB = descriptor.SchemaVersion, descriptor.CategoryGainsDB
	require.Equal(t, one, descriptor, "version 2 adds only the category gains")
}

func TestDescriptorOneAndTwoAreTheOnlyDescriptorVersions(t *testing.T) {
	for _, change := range []struct{ fixture, from, to string }{
		{"project.json", `"schemaVersion": 1`, `"schemaVersion": 3`},
		{"project2.json", `"schemaVersion": 2`, `"schemaVersion": 1`},
	} {
		data := bytes.Replace(readFormatFixture(t, change.fixture), []byte(change.from), []byte(change.to), 1)
		_, err := authoring.ParseProjectDescriptor(data)
		require.ErrorIs(t, err, authoring.ErrLibrary, change.fixture+" as "+change.to)
	}
}

func copyDescriptor(t *testing.T, name string) (string, []byte) {
	t.Helper()
	data := readFormatFixture(t, name)
	path := filepath.Join(t.TempDir(), "project.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	return path, data
}

func TestCategoryGainMovesADescriptorToVersionTwo(t *testing.T) {
	path, data := copyDescriptor(t, "project.json")
	written, err := authoring.SetDescriptorCategoryGain(path, data, authoring.CategorySFX, -4.5)
	require.NoError(t, err)
	descriptor, err := authoring.ReadProjectDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, authoring.ProjectDescriptorVersionTwo, descriptor.SchemaVersion)
	require.Equal(t, map[string]float64{"sfx": -4.5}, descriptor.CategoryGainsDB)
	require.Equal(t, string(data), strings.Replace(
		strings.Replace(string(written), `"schemaVersion": 2`, `"schemaVersion": 1`, 1),
		",\n  \"categoryGainsDb\": {\n    \"sfx\": -4.5\n  }", "", 1), "every other byte is kept")

	cleared, err := authoring.SetDescriptorCategoryGain(path, written, authoring.CategorySFX, 0)
	require.NoError(t, err)
	require.NotContains(t, string(cleared), "categoryGainsDb", "0 dB removes the entry")
	require.Contains(t, string(cleared), `"schemaVersion": 2`, "a descriptor never moves back a version")

	_, err = authoring.SetDescriptorCategoryGain(path, data, authoring.CategorySFX, 1)
	require.ErrorIs(t, err, authoring.ErrProjectChangedOnDisk, "a stale writer is refused")
	_, err = authoring.SetDescriptorCategoryGain(path, cleared, "voices", 1)
	require.ErrorIs(t, err, authoring.ErrLibrary, "only known categories")
	_, err = authoring.SetDescriptorCategoryGain(path, cleared, authoring.CategoryMusic, 13)
	require.ErrorIs(t, err, authoring.ErrLibrary, "within a calibration's range")
}

func TestCalibrationTargetKeepsItsMethod(t *testing.T) {
	path, data := copyDescriptor(t, "project.json")
	written, err := authoring.SetDescriptorCalibrationTarget(path, data, -18)
	require.NoError(t, err)
	descriptor, err := authoring.ReadProjectDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, authoring.CalibrationOptions{Method: authoring.ActiveRMS, TargetDB: -18}, *descriptor.Calibration)
	require.Equal(t, authoring.ProjectDescriptorVersionOne, descriptor.SchemaVersion, "the target is version 1's")
	require.Equal(t, string(data), strings.Replace(string(written), `"TargetDB": -18}`, `"TargetDB": -20}`, 1),
		"every other byte is kept")
	_, err = authoring.SetDescriptorCalibrationTarget(path, written, -3)
	require.ErrorIs(t, err, authoring.ErrLibrary, "targets stay in [-60,-6] dB")

	bare := filepath.Join(t.TempDir(), "project.json")
	require.NoError(t, authoring.CreateProjectDescriptor(bare, "library.json"))
	created, err := os.ReadFile(bare)
	require.NoError(t, err)
	_, err = authoring.SetDescriptorCalibrationTarget(bare, created, -24)
	require.NoError(t, err)
	descriptor, err = authoring.ReadProjectDescriptor(bare)
	require.NoError(t, err)
	require.Equal(t, authoring.CalibrationOptions{Method: authoring.ActiveRMS, TargetDB: -24}, *descriptor.Calibration)
}

func TestCommercialUseIsAnOrdinaryEdit(t *testing.T) {
	cleared := true
	path := copyFixture(t, "library4.json")
	editor := openFixture(t, path, 0)
	provenance := editor.Recording("river_loop").Provenance
	provenance.CommercialUse = &cleared
	require.NoError(t, editor.SetRecordingProvenance("river_loop", provenance))
	require.NoError(t, editor.SaveChanged())
	require.True(t, openFixture(t, path, 0).Recording("river_loop").Provenance.CommercialUseAllowed())

	early := openFixture(t, copyFixture(t, "library.json"), 0)
	provenance = early.Recording("river_loop").Provenance
	provenance.CommercialUse = &cleared
	require.ErrorIs(t, early.SetRecordingProvenance("river_loop", provenance), authoring.ErrLibrary,
		"schema 3 cannot record it")
}
