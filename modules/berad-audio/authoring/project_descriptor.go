package authoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

// Cue categories, as descriptors and CueCategory name them.
const (
	CategoryMusic   = catalogMusic
	CategorySFX     = catalogSFX
	CategoryAmbient = routingAmbientKey
)

// Project descriptor versions read. Version 2 adds categoryGainsDb; a
// version-1 descriptor stays version 1 until something writes a category
// gain. ProjectDescriptorVersion is the newest, which new descriptors use.
const (
	ProjectDescriptorVersionOne = 1
	ProjectDescriptorVersionTwo = 2
	ProjectDescriptorVersion    = ProjectDescriptorVersionTwo
)

const (
	maxDescriptorBytes     = 1 << 20
	descriptorSliderValues = 11
)

// ProjectDescriptor is version 1 or 2 of an audio project descriptor: the
// file an audio editor opens to find a library and the consumer policy for
// editing it. It configures editors; it does not replace or extend the
// manifest, and playback reads only the manifest. Library is relative to the
// descriptor's directory unless absolute; AssetRoot and ImportDirectory stay
// library-relative, as EditorOptions documents. Targets, TargetPolicies and
// Ambient keep their Go field names on the wire, as version 1 published them.
// Absent mappings mean neutral defaults, never cue-name heuristics.
//
// CategoryGainsDB, from version 2, is an editor's adjustment for each cue
// category (music, sfx, ambient) in dB, applied by the editor to the gain it
// writes for every cue in that category; playback sees only those cue gains.
// A missing category is 0 dB.
type ProjectDescriptor struct {
	SchemaVersion       int                           `json:"schemaVersion"`
	Library             string                        `json:"library"`
	AssetRoot           string                        `json:"assetRoot,omitempty"`
	ImportDirectory     string                        `json:"importDirectory,omitempty"`
	Targets             AuthoringTargets              `json:"targets,omitempty"`
	Calibration         *CalibrationOptions           `json:"calibration,omitempty"`
	CalibrationFamilies map[string]CalibrationOptions `json:"calibrationFamilies,omitempty"`
	SliderGains         []float64                     `json:"sliderGains,omitempty"`
	RecordingFamilies   map[string]string             `json:"recordingCategories,omitempty"`
	TargetPolicies      map[string]TargetPolicy       `json:"targetPolicies,omitempty"`
	Ambient             *AmbientPreset                `json:"ambientPreset,omitempty"`
	MixLabel            string                        `json:"mixLabel,omitempty"`
	SavedNotice         string                        `json:"savedNotice,omitempty"`
	CategoryGainsDB     map[string]float64            `json:"categoryGainsDb,omitempty"`
}

// CategoryGainDB is the descriptor's adjustment for a cue category, in dB; 0
// when it names none.
func (descriptor ProjectDescriptor) CategoryGainDB(category string) float64 {
	return descriptor.CategoryGainsDB[category]
}

// TargetPolicy describes assignment eligibility and consumer help for a
// routing group, or for one "group|target". Spatial and Background restrict an
// ambient profile's radius without interpreting target names; with neither
// set, either ambient kind is allowed.
type TargetPolicy struct {
	Category            string
	Spatial, Background bool
	Title, Help         string
}

// AmbientPreset supplies the defaults an editor gives a new ambient cue.
type AmbientPreset struct {
	CuePrefix, Profile, Family, HowToHear string
	Gain                                  float64
}

// ReadProjectDescriptor reads and validates one descriptor of at most 1 MiB.
// It rejects other versions, duplicate keys, unknown fields, trailing data,
// unknown categories and invalid calibration targets. It opens neither the
// library nor a device and changes no files.
func ReadProjectDescriptor(path string) (ProjectDescriptor, error) {
	file, err := os.Open(path) //nolint:gosec // An explicit developer-selected descriptor, not a sandboxed path.
	if err != nil {
		return ProjectDescriptor{}, fmt.Errorf("read audio project: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxDescriptorBytes+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return ProjectDescriptor{}, fmt.Errorf("read audio project: %w", err)
	}

	return ParseProjectDescriptor(data)
}

// ParseProjectDescriptor validates descriptor bytes as ReadProjectDescriptor
// does, for a caller that reads the file itself.
func ParseProjectDescriptor(data []byte) (ProjectDescriptor, error) {
	if len(data) > maxDescriptorBytes {
		return ProjectDescriptor{}, fmt.Errorf("%w: project descriptor exceeds 1 MiB", ErrLibrary)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSONValue(decoder); err != nil {
		return ProjectDescriptor{}, fmt.Errorf("%w: project descriptor: %w", ErrLibrary, err)
	}
	var descriptor ProjectDescriptor
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	//nolint:musttag // Nested targets and policies keep the Go field names version 1 published.
	if err := decoder.Decode(&descriptor); err != nil {
		return ProjectDescriptor{}, fmt.Errorf("%w: decode audio project: %w", ErrLibrary, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ProjectDescriptor{}, fmt.Errorf("%w: trailing project data", ErrLibrary)
	}
	if err := descriptor.validate(); err != nil {
		return ProjectDescriptor{}, err
	}

	return descriptor, nil
}

// EditorOptions turns the descriptor's library policy into options for
// OpenLibraryProject: its asset root, import directory and targets, and its
// calibration, by acoustic family where CalibrationFamilies names one.
func (descriptor ProjectDescriptor) EditorOptions(decode audio.Decoder) EditorOptions {
	options := DefaultEditorOptions(decode)
	options.AssetRoot, options.ImportDirectory = descriptor.AssetRoot, descriptor.ImportDirectory
	options.Targets = cloneAudioValue(descriptor.Targets)
	families, fallback := cloneAudioValue(descriptor.CalibrationFamilies), DefaultCalibrationOptions()
	if descriptor.Calibration != nil {
		fallback = *descriptor.Calibration
	}
	options.Calibration = func(family string) CalibrationOptions {
		if value, exists := families[family]; exists {
			return value
		}

		return fallback
	}

	return options
}

// LibraryPath resolves the descriptor's library against the directory of the
// descriptor read from descriptorPath.
func (descriptor ProjectDescriptor) LibraryPath(descriptorPath string) string {
	if filepath.IsAbs(descriptor.Library) {
		return descriptor.Library
	}

	return filepath.Join(filepath.Dir(descriptorPath), descriptor.Library)
}

func (descriptor ProjectDescriptor) validate() error {
	if err := descriptor.validateVersion(); err != nil {
		return err
	}
	if descriptor.Calibration != nil && !validDescriptorCalibration(*descriptor.Calibration) {
		return fmt.Errorf("%w: invalid calibration policy", ErrLibrary)
	}
	for _, value := range descriptor.CalibrationFamilies {
		if !validDescriptorCalibration(value) {
			return fmt.Errorf("%w: invalid family calibration policy", ErrLibrary)
		}
	}
	if len(descriptor.SliderGains) > 0 && len(descriptor.SliderGains) != descriptorSliderValues {
		return fmt.Errorf("%w: sliderGains requires 11 values", ErrLibrary)
	}
	for _, value := range descriptor.RecordingFamilies {
		if !validCueCategory(value) {
			return fmt.Errorf("%w: invalid recording category", ErrLibrary)
		}
	}
	for _, value := range descriptor.TargetPolicies {
		if !validCueCategory(value.Category) || value.Spatial && value.Background {
			return fmt.Errorf("%w: invalid target policy", ErrLibrary)
		}
	}

	return validateAuthoringTargets(descriptor.Targets)
}

// validateVersion accepts versions 1 and 2 naming a library, and category
// gains only from version 2, each for a known category and finite in a
// calibration's [-48,+12] dB range.
func (descriptor ProjectDescriptor) validateVersion() error {
	version := descriptor.SchemaVersion
	if version < ProjectDescriptorVersionOne || version > ProjectDescriptorVersionTwo || descriptor.Library == "" {
		return fmt.Errorf("%w: project requires schemaVersion 1 or 2 and library", ErrLibrary)
	}
	if descriptor.CategoryGainsDB == nil {
		return nil
	}
	if descriptor.SchemaVersion < ProjectDescriptorVersionTwo {
		return fmt.Errorf("%w: categoryGainsDb needs project descriptor version 2", ErrLibrary)
	}
	for category, gain := range descriptor.CategoryGainsDB {
		if !validCueCategory(category) || !finiteRange(gain, calibrationMinGain, calibrationMaxGain) {
			return fmt.Errorf("%w: category gain for %q must be a known category in [-48,12] dB", ErrLibrary, category)
		}
	}

	return nil
}

func validCueCategory(value string) bool {
	return value == CategoryMusic || value == CategorySFX || value == CategoryAmbient
}

func validDescriptorCalibration(value CalibrationOptions) bool {
	return (value.Method == ActiveRMS || value.Method == GatedLoudness) &&
		!math.IsNaN(value.TargetDB) && !math.IsInf(value.TargetDB, 0) &&
		value.TargetDB >= calibrationMinTarget && value.TargetDB <= calibrationMaxTarget
}

// CueCategory returns a cue's category: the one its consumer targets name,
// else ambient when routing gives it an ambient profile, else SFX.
func CueCategory(targets AuthoringTargets, routing Routing, cueID string) string {
	if category, named := targets.CueCategories[cueID]; named {
		return category
	}
	if _, ambient := routing.Ambient[cueID]; ambient {
		return CategoryAmbient
	}

	return CategorySFX
}
