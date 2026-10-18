package authoring

import (
	"fmt"
	"math"
	"strings"
)

// validateRoutingStructure checks references and numerical integrity without
// supplying a game's targets, names, required defaults or map vocabulary.
func validateRoutingStructure(library *audioLibrary) error {
	for cueID, profile := range library.Routing.Ambient {
		cue, exists := library.Cues[cueID]
		if !exists {
			return fmt.Errorf("%w: ambient profile references unknown cue %s", ErrAudioLibrary, cueID)
		}
		if cue.RecordingID == "" {
			return fmt.Errorf("%w: ambient profile %s requires one recording, not a cycle", ErrAudioLibrary, cueID)
		}
		if math.IsNaN(profile.RadiusTiles) || math.IsInf(profile.RadiusTiles, 0) || profile.RadiusTiles < 0 ||
			profile.FadeInMs < 0 || profile.FadeOutMs < 0 {
			return fmt.Errorf("%w: invalid ambient profile %s", ErrAudioLibrary, cueID)
		}
	}
	var groups []map[string]*string
	for name, assignments := range library.Routing.Groups {
		if name == "" || strings.TrimSpace(name) != name {
			return fmt.Errorf("%w: invalid routing assignment group", ErrLibrary)
		}
		groups = append(groups, assignments)
	}
	if err := validateCoreAssignments(library, groups); err != nil {
		return err
	}
	for _, cues := range library.Routing.Surfaces {
		for _, cue := range cues {
			if _, exists := library.Cues[cue]; !exists {
				return fmt.Errorf("%w: unknown surface cue %s", ErrAudioLibrary, cue)
			}
		}
	}
	for role, cue := range library.Routing.Emitters {
		if _, exists := library.Routing.Ambient[cue]; !exists {
			return fmt.Errorf("%w: unknown emitter cue for %s", ErrAudioLibrary, role)
		}
	}

	return validateCoreTiles(library)
}

func validateCoreTiles(library *audioLibrary) error {
	if name := library.Routing.DefaultSurface; name != "" {
		if _, exists := library.Routing.Surfaces[name]; !exists {
			return fmt.Errorf("%w: unknown default surface %s", ErrAudioLibrary, name)
		}
	}
	for key, tile := range library.Routing.Tiles {
		if key == "" {
			return fmt.Errorf("%w: empty tile target", ErrAudioLibrary)
		}
		if tile.AmbientCue != "" {
			if _, exists := library.Routing.Ambient[tile.AmbientCue]; !exists {
				return fmt.Errorf("%w: unknown ambience for %s", ErrAudioLibrary, key)
			}
		}
		if tile.Surface != "" {
			if _, exists := library.Routing.Surfaces[tile.Surface]; !exists {
				return fmt.Errorf("%w: unknown surface for %s", ErrAudioLibrary, key)
			}
		}
		for _, value := range []*float64{tile.RadiusTiles, tile.Gain} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0) {
				return fmt.Errorf("%w: invalid source override for %s", ErrAudioLibrary, key)
			}
		}
	}

	return nil
}

func validateCoreAssignments(library *audioLibrary, groups []map[string]*string) error {
	for _, assignments := range groups {
		for target, cue := range assignments {
			if target == "" {
				return fmt.Errorf("%w: empty routing target", ErrAudioLibrary)
			}
			if cue != nil {
				if _, exists := library.Cues[*cue]; !exists {
					return fmt.Errorf("%w: unknown cue for %s", ErrAudioLibrary, target)
				}
			}
		}
	}

	return nil
}
