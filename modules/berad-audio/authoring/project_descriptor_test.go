package authoring_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func writeDescriptor(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "project.json")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))

	return path
}

func TestProjectDescriptorRejectsWhatEditorsCannotTrust(t *testing.T) {
	for name, text := range map[string]string{
		"no library":          `{"schemaVersion": 1}`,
		"duplicate key":       `{"schemaVersion": 1, "library": "a.json", "library": "b.json"}`,
		"unknown field":       `{"schemaVersion": 1, "library": "library.json", "volume": 1}`,
		"trailing data":       `{"schemaVersion": 1, "library": "library.json"} {}`,
		"calibration target":  `{"schemaVersion": 1, "library": "library.json", "calibration": {"Method": "active_rms", "TargetDB": -3}}`,
		"calibration method":  `{"schemaVersion": 1, "library": "library.json", "calibrationFamilies": {"x": {"Method": "peak", "TargetDB": -20}}}`,
		"slider count":        `{"schemaVersion": 1, "library": "library.json", "sliderGains": [0, 1]}`,
		"recording category":  `{"schemaVersion": 1, "library": "library.json", "recordingCategories": {"water": "voice"}}`,
		"spatial background":  `{"schemaVersion": 1, "library": "library.json", "targetPolicies": {"music": {"Category": "ambient", "Spatial": true, "Background": true}}}`,
		"cue category":        `{"schemaVersion": 1, "library": "library.json", "targets": {"CueCategories": {"door/open": "voice"}}}`,
		"schema-3 group only": `{"schemaVersion": 1, "library": "library.json", "targets": {"Groups": {"weather": {}}}}`,
	} {
		_, err := authoring.ReadProjectDescriptor(writeDescriptor(t, text))
		require.ErrorIs(t, err, authoring.ErrLibrary, name)
	}
	_, err := authoring.ReadProjectDescriptor(writeDescriptor(t,
		`{"schemaVersion": 1, "library": "library.json", "note": "`+strings.Repeat("x", 1<<20)+`"}`))
	require.ErrorIs(t, err, authoring.ErrLibrary, "over 1 MiB")
	_, err = authoring.ReadProjectDescriptor(filepath.Join(t.TempDir(), "missing.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestProjectDescriptorLibraryPath(t *testing.T) {
	path := writeDescriptor(t, `{"schemaVersion": 1, "library": "audio/library.json"}`)
	descriptor, err := authoring.ReadProjectDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(path), "audio", "library.json"), descriptor.LibraryPath(path))
	absolute := filepath.Join(t.TempDir(), "elsewhere.json")
	descriptor.Library = absolute
	require.Equal(t, absolute, descriptor.LibraryPath(path))
}

func TestCueCategoryPrefersTheConsumerThenAmbientRouting(t *testing.T) {
	targets := authoring.AuthoringTargets{
		Tiles: nil, Groups: nil, Families: nil,
		CueCategories: map[string]string{"theme/explore": authoring.CategoryMusic, "bed/crowd": authoring.CategorySFX},
	}
	routing := authoring.Routing{
		Ambient: map[string]authoring.AmbientProfile{
			"bed/river": {RadiusTiles: 8, FadeInMs: 800, FadeOutMs: 1200},
			"bed/crowd": {RadiusTiles: 5, FadeInMs: 800, FadeOutMs: 1200},
		},
		Tiles: nil, Surfaces: nil, DefaultSurface: "", Emitters: nil, Groups: nil,
	}
	require.Equal(t, authoring.CategoryMusic, authoring.CueCategory(targets, routing, "theme/explore"))
	require.Equal(t, authoring.CategoryAmbient, authoring.CueCategory(targets, routing, "bed/river"))
	require.Equal(t, authoring.CategorySFX, authoring.CueCategory(targets, routing, "bed/crowd"),
		"a named category wins over ambient routing")
	require.Equal(t, authoring.CategorySFX, authoring.CueCategory(targets, routing, "door/open"))
}
