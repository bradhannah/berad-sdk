package authoring_test

// The format guard keeps schema-3 libraries and version-1 project descriptors
// exactly as every editor and game reading them expects. testdata/format holds
// one of each using every field. A change that renames, retypes, adds or
// drops an encoded field fails here; it needs a new schema or descriptor
// version with a step-by-step upgrade, not an edit to these files.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
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

func TestSchemaThreeFormatIsFrozen(t *testing.T) {
	data := readFormatFixture(t, "library.json")
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

func TestSchemaThreeIsTheOnlyLibraryVersion(t *testing.T) {
	data := bytes.Replace(readFormatFixture(t, "library.json"), []byte(`"schemaVersion": 3`),
		[]byte(`"schemaVersion": 4`), 1)
	_, err := authoring.ParseDocument(data, nil)
	require.ErrorIs(t, err, authoring.ErrLibrary)
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

func TestDescriptorOneIsTheOnlyDescriptorVersion(t *testing.T) {
	data := bytes.Replace(readFormatFixture(t, "project.json"), []byte(`"schemaVersion": 1`),
		[]byte(`"schemaVersion": 2`), 1)
	path := filepath.Join(t.TempDir(), "project.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	_, err := authoring.ReadProjectDescriptor(path)
	require.ErrorIs(t, err, authoring.ErrLibrary)
}
