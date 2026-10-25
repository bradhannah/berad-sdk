package authoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/stretchr/testify/require"
)

func TestCreateProjectDescriptorNamesItsLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	require.Error(t, CreateProjectDescriptor(path, " "))
	require.NoError(t, CreateProjectDescriptor(path, "library.json"))
	require.Error(t, CreateProjectDescriptor(path, "other.json"), "an existing descriptor is never replaced")
	descriptor, err := ReadProjectDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(path), "library.json"), descriptor.LibraryPath(path))
}

func TestSetDescriptorCueCategoryKeepsEverythingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	original := "{\n  \"schemaVersion\": 1,\n  \"library\": \"library.json\",\n" +
		"  \"targets\": {\n    \"Families\": [\"signal\"]\n  },\n  \"mixLabel\": \"Test mix\"\n}\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

	first, err := SetDescriptorCueCategory(path, []byte(original), "theme/title", CategoryMusic)
	require.NoError(t, err)
	require.Equal(t, "{\n  \"schemaVersion\": 1,\n  \"library\": \"library.json\",\n"+
		"  \"targets\": {\n    \"Families\": [\"signal\"],\n    \"CueCategories\": {\n      \"theme/title\": \"music\"\n    }\n  },\n"+
		"  \"mixLabel\": \"Test mix\"\n}\n", string(first))
	second, err := SetDescriptorCueCategory(path, first, "bed/river", CategoryAmbient)
	require.NoError(t, err)
	descriptor, err := ReadProjectDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"theme/title": "music", "bed/river": "ambient"}, descriptor.Targets.CueCategories)
	require.Equal(t, []string{"signal"}, descriptor.Targets.Families)

	third, err := SetDescriptorCueCategory(path, second, "theme/title", "")
	require.NoError(t, err)
	require.NotContains(t, string(third), "theme/title")
	cleared, err := SetDescriptorCueCategory(path, third, "bed/river", "")
	require.NoError(t, err)
	require.Equal(t, original, string(cleared), "removing the last category leaves the original bytes")

	_, err = SetDescriptorCueCategory(path, cleared, "x", "loud")
	require.ErrorIs(t, err, ErrLibrary)
	_, err = SetDescriptorCueCategory(path, first, "x", CategorySFX)
	require.ErrorIs(t, err, ErrProjectChangedOnDisk, "bytes other than the expected ones are never replaced")
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, cleared, unchanged)
}

func TestSetDescriptorCueCategoryAddsTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	require.NoError(t, CreateProjectDescriptor(path, "library.json"))
	created, err := os.ReadFile(path)
	require.NoError(t, err)
	written, err := SetDescriptorCueCategory(path, created, "sample/tune", CategoryMusic)
	require.NoError(t, err)
	require.Equal(t, "{\n  \"schemaVersion\": 1,\n  \"library\": \"library.json\",\n"+
		"  \"targets\": {\n    \"CueCategories\": {\n      \"sample/tune\": \"music\"\n    }\n  }\n}\n", string(written))
	descriptor, err := ParseProjectDescriptor(written)
	require.NoError(t, err)
	require.Equal(t, CategoryMusic, CueCategory(descriptor.Targets, Routing{}, "sample/tune"))
}

func TestDescriptorEditorOptionsCalibrateByFamily(t *testing.T) {
	music := CalibrationOptions{Method: GatedLoudness, TargetDB: -23}
	fallback := CalibrationOptions{Method: ActiveRMS, TargetDB: -18}
	descriptor := ProjectDescriptor{SchemaVersion: 1, Library: "library.json", AssetRoot: "sounds",
		ImportDirectory: "imported", Calibration: &fallback,
		CalibrationFamilies: map[string]CalibrationOptions{"music": music}}
	options := descriptor.EditorOptions(ebitengine.Decode)
	require.Equal(t, "sounds", options.AssetRoot)
	require.Equal(t, "imported", options.ImportDirectory)
	require.Equal(t, music, options.Calibration("music"))
	require.Equal(t, fallback, options.Calibration("signal"))
	require.NotNil(t, options.Decode)
	descriptor.Calibration = nil
	require.Equal(t, DefaultCalibrationOptions(), descriptor.EditorOptions(ebitengine.Decode).Calibration("signal"))
}
