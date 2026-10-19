package editorui

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

// AmbientPreset supplies consumer-authored defaults for new persistent cues.
// Nil disables creation; no profile/family/prefix is silently invented.
type AmbientPreset struct {
	CuePrefix, Profile, Family, HowToHear string
	Gain                                  float64
}

// TargetPolicy describes assignment eligibility and consumer-specific help.
// Spatial/Background restrict an ambient profile's radius without interpreting
// target names. With neither flag set, either ambient kind is allowed.
type TargetPolicy struct {
	Category            string
	Spatial, Background bool
	Title, Help         string
}

func (m *model) groupLabel(group string) string {
	if label := m.targets.Groups[group].Label; label != "" {
		return label
	}
	return group
}

// Options contains presentation/project policy, never game state or saved player
// settings. Callbacks must be deterministic and safe on the editor event loop.
type Options struct {
	SliderGain        func(int) float64
	RecordingCategory func(string, audio.Recording, string) string
	TargetPolicy      func(string, string) TargetPolicy
	Ambient           *AmbientPreset
	MixLabel          string
	SavedNotice       string
}

func (m *model) targetPolicy(group, target string) TargetPolicy {
	policy := TargetPolicy{Category: m.targets.Groups[group].Category}
	if group == "" || group == routingEmitter {
		policy.Category = "ambient"
	}
	if m.options.TargetPolicy != nil {
		policy = m.options.TargetPolicy(group, target)
	}
	if policy.Category == "" {
		policy.Category = "sfx"
	}
	if policy.Title == "" {
		policy.Title = "CHOOSE " + strings.ToUpper(policy.Category) + " SOUND"
	}
	return policy
}

func normalizedOptions(options Options) Options {
	if options.SliderGain == nil {
		options.SliderGain = func(level int) float64 { return float64(level) / 10 }
	}
	if options.MixLabel == "" {
		options.MixLabel = "PROJECT MIX"
	}
	if options.SavedNotice == "" {
		options.SavedNotice = "Saved. Existing playback snapshots remain unchanged until reloaded."
	}
	if options.Ambient != nil {
		value := *options.Ambient
		options.Ambient = &value
	}
	return options
}

func validateOptions(doc *audio.LibraryEditor, options Options) error {
	for level := 0; level <= 10; level++ {
		value := options.SliderGain(level)
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("%w: slider mapping must return finite values in [0,1]", audio.ErrLibrary)
		}
	}
	if preset := options.Ambient; preset != nil {
		_, exists := doc.Document().Taxonomy.Profiles[preset.Profile]
		if !exists || preset.Family == "" || strings.TrimSpace(preset.HowToHear) == "" || math.IsNaN(preset.Gain) || math.IsInf(preset.Gain, 0) || preset.Gain < 0 || preset.Gain > 1 {
			return fmt.Errorf("%w: ambient preset requires a known profile, family, description and gain in [0,1]", audio.ErrLibrary)
		}
	}
	for group, descriptor := range doc.Targets().Groups {
		if descriptor.Category != "" && descriptor.Category != "music" && descriptor.Category != "sfx" && descriptor.Category != "ambient" {
			return fmt.Errorf("%w: unknown target category", audio.ErrLibrary)
		}
		if options.TargetPolicy != nil {
			for target := range descriptor.Targets {
				policy := options.TargetPolicy(group, target)
				if policy.Spatial && policy.Background || policy.Category != "music" && policy.Category != "sfx" && policy.Category != "ambient" {
					return fmt.Errorf("%w: invalid target policy for %s/%s", audio.ErrLibrary, group, target)
				}
			}
		}
	}
	return nil
}

// ValidateOptions checks project presentation policy without starting the UI or
// opening a native device. Run performs the same check before terminal setup.
func ValidateOptions(doc *audio.LibraryEditor, options Options) error {
	if doc == nil {
		return fmt.Errorf("%w: editor document required", audio.ErrLibrary)
	}
	return validateOptions(doc, normalizedOptions(options))
}

func (m *model) sliderGain(level int) float64 {
	gain := m.options.SliderGain(level)
	if math.IsNaN(gain) || math.IsInf(gain, 0) || gain < 0 || gain > 1 {
		panic("invalid project slider gain")
	}
	return gain
}

func supportedRecording(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav", ".mp3", ".ogg":
		return true
	default:
		return false
	}
}

type routingUsage struct{ Workspace, Target, Label string }

func (m *model) routingUsages(cueID string) []routingUsage {
	var usages []routingUsage
	routing := m.doc.Routing()
	for _, tile := range m.targets.Tiles {
		assignment := routing.Tiles[tile.ID]
		if assignment.AmbientCue == cueID {
			usages = append(usages, routingUsage{"tiles", tile.ID, tile.Name + " · ambience"})
		}
		surface := assignment.Surface
		if surface == "" {
			surface = routing.DefaultSurface
		}
		if slices.Contains(routing.Surfaces[surface], cueID) {
			usages = append(usages, routingUsage{"tiles", tile.ID, tile.Name + " · footsteps: " + surface})
		}
	}
	for group, descriptor := range m.targets.Groups {
		for target, label := range descriptor.Targets {
			if cue, _ := m.doc.ResolveAssignment(group, target); cue == cueID {
				usages = append(usages, routingUsage{group, target, label + " · " + descriptor.Label})
			}
		}
	}
	for emitter, cue := range routing.Emitters {
		if cue == cueID {
			usages = append(usages, routingUsage{"emitter", emitter, emitter + " · active source"})
		}
	}
	slices.SortFunc(usages, func(a, b routingUsage) int { return strings.Compare(a.Label, b.Label) })
	return usages
}

func (m *model) addAmbient(id, name string, profile audio.AmbientProfile, recordingID string) error {
	preset := m.options.Ambient
	if preset == nil {
		return fmt.Errorf("%w: this project has no ambient creation preset", audio.ErrLibrary)
	}
	if id == "" || strings.Trim(id, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
		return fmt.Errorf("%w: invalid sound identifier", audio.ErrLibrary)
	}
	next, revision := m.doc.Document(), m.doc.Revision()
	if next.Routing.Ambient == nil {
		return fmt.Errorf("%w: project has no ambient authoring section", audio.ErrLibrary)
	}
	if _, exists := next.Taxonomy.Profiles[preset.Profile]; !exists {
		return fmt.Errorf("%w: missing creation profile", audio.ErrLibrary)
	}
	cueID := preset.CuePrefix + id
	if _, exists := next.Cues[cueID]; exists {
		return fmt.Errorf("%w: cue already exists", audio.ErrLibrary)
	}
	if recordingID == "" {
		recordingID = id + "_01"
		if _, exists := next.Recordings[recordingID]; exists {
			return fmt.Errorf("%w: recording already exists", audio.ErrLibrary)
		}
		next.Recordings[recordingID] = audio.Recording{Provenance: audio.RecordingProvenance{
			OriginalFilename: "unknown", SourceType: "unknown", SourceURL: "unknown", SourceCreator: "unknown",
			License: "unknown", ImportedFrom: "unknown", Processing: []audio.ProcessingStep{}}}
		next.Taxonomy.RecordingFamilies[preset.Family] = append(next.Taxonomy.RecordingFamilies[preset.Family], recordingID)
	}
	next.Cues[cueID] = audio.Cue{Name: name, RecordingID: recordingID, VolumeModifier: preset.Gain, HowToHear: preset.HowToHear}
	next.Routing.Ambient[cueID] = profile
	next.Taxonomy.CueProfiles[preset.Profile] = append(next.Taxonomy.CueProfiles[preset.Profile], cueID)
	return m.doc.ApplyDocument(revision, next)
}
