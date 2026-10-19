// Package project loads explicit standalone tool configuration. It does not
// discover a game checkout, embedded catalog, player settings or game rules.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	editor "github.com/bradhannah/berad-sdk/tools/berad-audio/editor"
)

// Config version 1 is a tool-project descriptor, not a replacement audio manifest.
// Library is descriptor-relative. AssetRoot remains library-relative as documented
// by authoring. Absent mappings use neutral defaults, never cue-name heuristics.
type Config struct {
	SchemaVersion       int                                     `json:"schemaVersion"`
	Library             string                                  `json:"library"`
	AssetRoot           string                                  `json:"assetRoot,omitempty"`
	ImportDirectory     string                                  `json:"importDirectory,omitempty"`
	Targets             authoring.AuthoringTargets              `json:"targets,omitempty"`
	Calibration         *authoring.CalibrationOptions           `json:"calibration,omitempty"`
	CalibrationFamilies map[string]authoring.CalibrationOptions `json:"calibrationFamilies,omitempty"`
	SliderGains         []float64                               `json:"sliderGains,omitempty"`
	RecordingFamilies   map[string]string                       `json:"recordingCategories,omitempty"`
	TargetPolicies      map[string]editor.TargetPolicy          `json:"targetPolicies,omitempty"`
	Ambient             *editor.AmbientPreset                   `json:"ambientPreset,omitempty"`
	MixLabel            string                                  `json:"mixLabel,omitempty"`
	SavedNotice         string                                  `json:"savedNotice,omitempty"`
}

// OpenLibrary uses neutral defaults for an explicit manifest, without target
// discovery. Integrations needing policy supply Config or use authoring directly.
func OpenLibrary(path string) (*authoring.LibraryEditor, editor.Options, error) {
	doc, err := authoring.OpenLibraryProject(path, authoring.DefaultEditorOptions(ebitengine.Decode))
	return doc, editor.Options{}, err
}

// Open reads one descriptor. It does not change files or open a native device.
func Open(path string) (*authoring.LibraryEditor, editor.Options, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, editor.Options{}, fmt.Errorf("read audio project: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, editor.Options{}, err
	}
	if len(data) > 1<<20 {
		return nil, editor.Options{}, fmt.Errorf("%w: project descriptor exceeds 1 MiB", authoring.ErrLibrary)
	}
	if err := uniqueValue(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return nil, editor.Options{}, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, editor.Options{}, fmt.Errorf("decode audio project: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, editor.Options{}, fmt.Errorf("%w: trailing project data", authoring.ErrLibrary)
	}
	if config.SchemaVersion != 1 || config.Library == "" {
		return nil, editor.Options{}, fmt.Errorf("%w: project requires schemaVersion 1 and library", authoring.ErrLibrary)
	}
	validCalibration := func(value authoring.CalibrationOptions) bool {
		return (value.Method == authoring.ActiveRMS || value.Method == authoring.GatedLoudness) && !math.IsNaN(value.TargetDB) && !math.IsInf(value.TargetDB, 0) && value.TargetDB >= -60 && value.TargetDB <= -6
	}
	if config.Calibration != nil && !validCalibration(*config.Calibration) {
		return nil, editor.Options{}, fmt.Errorf("%w: invalid calibration policy", authoring.ErrLibrary)
	}
	for _, value := range config.CalibrationFamilies {
		if !validCalibration(value) {
			return nil, editor.Options{}, fmt.Errorf("%w: invalid family calibration policy", authoring.ErrLibrary)
		}
	}
	for _, value := range config.RecordingFamilies {
		if value != "sfx" && value != "music" && value != "ambient" {
			return nil, editor.Options{}, fmt.Errorf("%w: invalid recording category", authoring.ErrLibrary)
		}
	}
	for _, value := range config.TargetPolicies {
		if value.Category != "sfx" && value.Category != "music" && value.Category != "ambient" || value.Spatial && value.Background {
			return nil, editor.Options{}, fmt.Errorf("%w: invalid target policy", authoring.ErrLibrary)
		}
	}
	library := config.Library
	if !filepath.IsAbs(library) {
		library = filepath.Join(filepath.Dir(path), library)
	}
	options := authoring.DefaultEditorOptions(ebitengine.Decode)
	options.AssetRoot, options.ImportDirectory, options.Targets = config.AssetRoot, config.ImportDirectory, config.Targets
	options.Calibration = func(family string) authoring.CalibrationOptions {
		if value, exists := config.CalibrationFamilies[family]; exists {
			return value
		}
		if config.Calibration != nil {
			return *config.Calibration
		}
		return authoring.DefaultCalibrationOptions()
	}
	view := editor.Options{Ambient: config.Ambient, MixLabel: config.MixLabel, SavedNotice: config.SavedNotice}
	if len(config.SliderGains) > 0 {
		if len(config.SliderGains) != 11 {
			return nil, view, fmt.Errorf("%w: sliderGains requires 11 values", authoring.ErrLibrary)
		}
		view.SliderGain = func(level int) float64 { return config.SliderGains[level] }
	}
	view.RecordingCategory = func(_ string, _ authoring.Recording, family string) string { return config.RecordingFamilies[family] }
	view.TargetPolicy = func(group, target string) editor.TargetPolicy {
		if value, exists := config.TargetPolicies[group+"|"+target]; exists {
			return value
		}
		if value, exists := config.TargetPolicies[group]; exists {
			return value
		}
		category := config.Targets.Groups[group].Category
		if group == "" || group == "emitter" {
			category = "ambient"
		}
		if category == "" {
			category = "sfx"
		}
		return editor.TargetPolicy{Category: category}
	}
	doc, err := authoring.OpenLibraryProject(library, options)
	if err != nil {
		return nil, view, err
	}
	if err := editor.ValidateOptions(doc, view); err != nil {
		return nil, view, err
	}
	return doc, view, nil
}

func uniqueValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read project JSON: %w", err)
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("%w: duplicate project key", authoring.ErrLibrary)
			}
			seen[name] = true
		}
		if err := uniqueValue(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
