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

const (
	categorySFX        = "sfx"
	categoryAmbient    = "ambient"
	maxDescriptorBytes = 1 << 20
	sliderPositions    = 11
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
	if err != nil {
		return nil, editor.Options{}, fmt.Errorf("open standalone library: %w", err)
	}

	var options editor.Options

	return doc, options, nil
}

// Open reads one descriptor. It does not change files or open a native device.
func Open(path string) (*authoring.LibraryEditor, editor.Options, error) {
	config, err := readConfig(path)
	if err != nil {
		return nil, editor.Options{}, err
	}
	if err := config.validate(); err != nil {
		return nil, editor.Options{}, err
	}
	library := config.Library
	if !filepath.IsAbs(library) {
		library = filepath.Join(filepath.Dir(path), library)
	}
	options := authoring.DefaultEditorOptions(ebitengine.Decode)
	options.AssetRoot, options.ImportDirectory, options.Targets = config.AssetRoot, config.ImportDirectory, config.Targets
	options.Calibration = config.calibration
	view, err := config.viewOptions()
	if err != nil {
		return nil, view, err
	}
	doc, err := authoring.OpenLibraryProject(library, options)
	if err != nil {
		return nil, view, fmt.Errorf("open project library: %w", err)
	}
	if err := editor.ValidateOptions(doc, view); err != nil {
		return nil, view, fmt.Errorf("validate project view: %w", err)
	}

	return doc, view, nil
}

func readConfig(path string) (Config, error) {
	file, err := os.Open(path) //nolint:gosec // Explicit developer-selected project descriptor, not a sandboxed path.
	if err != nil {
		return Config{}, fmt.Errorf("read audio project: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxDescriptorBytes+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return Config{}, err
	}
	if len(data) > maxDescriptorBytes {
		return Config{}, fmt.Errorf("%w: project descriptor exceeds 1 MiB", authoring.ErrLibrary)
	}
	if err := uniqueValue(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return Config{}, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	//nolint:musttag // Nested policy structs retain their published Go field names in version-1 descriptors.
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode audio project: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%w: trailing project data", authoring.ErrLibrary)
	}

	return config, nil
}

func (config Config) validate() error {
	if config.SchemaVersion != 1 || config.Library == "" {
		return fmt.Errorf("%w: project requires schemaVersion 1 and library", authoring.ErrLibrary)
	}
	if config.Calibration != nil && !validCalibration(*config.Calibration) {
		return fmt.Errorf("%w: invalid calibration policy", authoring.ErrLibrary)
	}
	for _, value := range config.CalibrationFamilies {
		if !validCalibration(value) {
			return fmt.Errorf("%w: invalid family calibration policy", authoring.ErrLibrary)
		}
	}
	for _, value := range config.RecordingFamilies {
		if !validCategory(value) {
			return fmt.Errorf("%w: invalid recording category", authoring.ErrLibrary)
		}
	}
	for _, value := range config.TargetPolicies {
		if !validCategory(value.Category) || value.Spatial && value.Background {
			return fmt.Errorf("%w: invalid target policy", authoring.ErrLibrary)
		}
	}

	return nil
}

func validCategory(value string) bool {
	return value == categorySFX || value == "music" || value == categoryAmbient
}

func validCalibration(value authoring.CalibrationOptions) bool {
	return (value.Method == authoring.ActiveRMS || value.Method == authoring.GatedLoudness) &&
		!math.IsNaN(value.TargetDB) && !math.IsInf(value.TargetDB, 0) && value.TargetDB >= -60 && value.TargetDB <= -6
}

func (config Config) calibration(family string) authoring.CalibrationOptions {
	if value, exists := config.CalibrationFamilies[family]; exists {
		return value
	}
	if config.Calibration != nil {
		return *config.Calibration
	}

	return authoring.DefaultCalibrationOptions()
}

func (config Config) viewOptions() (editor.Options, error) {
	var view editor.Options
	view.Ambient, view.MixLabel, view.SavedNotice = config.Ambient, config.MixLabel, config.SavedNotice
	if len(config.SliderGains) > 0 {
		if len(config.SliderGains) != sliderPositions {
			return view, fmt.Errorf("%w: sliderGains requires 11 values", authoring.ErrLibrary)
		}
		view.SliderGain = func(level int) float64 { return config.SliderGains[level] }
	}
	view.RecordingCategory = func(_ string, _ authoring.Recording, family string) string {
		return config.RecordingFamilies[family]
	}
	view.TargetPolicy = config.targetPolicy

	return view, nil
}

func (config Config) targetPolicy(group, target string) editor.TargetPolicy {
	if value, exists := config.TargetPolicies[group+"|"+target]; exists {
		return value
	}
	if value, exists := config.TargetPolicies[group]; exists {
		return value
	}
	category := config.Targets.Groups[group].Category
	if group == "" || group == "emitter" {
		category = categoryAmbient
	}
	if category == "" {
		category = categorySFX
	}

	return editor.TargetPolicy{Category: category, Spatial: false, Background: false, Title: "", Help: ""}
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
				return fmt.Errorf("read project key: %w", err)
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
	if err != nil {
		return fmt.Errorf("read project delimiter: %w", err)
	}

	return nil
}
