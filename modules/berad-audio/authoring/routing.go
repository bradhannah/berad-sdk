package authoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

const catalogMusic = "music"
const catalogSFX = "sfx"
const routingAmbientKey = "ambient"
const routingTilesKey = "tiles"
const routingSurfacesKey = "surfaces"
const routingEmittersKey = "emitters"
const routingDefaultSurfaceKey = "defaultSurface"

// AmbientProfile retains schema-3 source-distance/fade units. The consumer owns
// source discovery, coordinates and the meaning of one distance unit.
type AmbientProfile struct {
	RadiusTiles float64 `json:"radiusTiles"`
	FadeInMs    int64   `json:"fadeInMs"`
	FadeOutMs   int64   `json:"fadeOutMs"`
}

// TileAudio is a consumer-keyed source assignment, using the existing wire names.
type TileAudio struct {
	AmbientCue  string   `json:"ambientCue,omitempty"`
	Surface     string   `json:"surface,omitempty"`
	RadiusTiles *float64 `json:"radiusTiles,omitempty"`
	Gain        *float64 `json:"gain,omitempty"`
}

// WithAmbient returns an updated assignment, clearing source overrides when unset.
func (assignment TileAudio) WithAmbient(cue string) TileAudio {
	assignment.AmbientCue = cue
	if cue == "" {
		assignment.RadiusTiles, assignment.Gain = nil, nil
	}

	return assignment
}

// Routing preserves schema-3 wire groups without interpreting their gameplay
// meaning. Labels, target identities and inheritance are supplied by the consumer.
// New wire sections require a schema revision, not a permissive version-3 write.
type Routing struct {
	Ambient        map[string]AmbientProfile
	Tiles          map[string]TileAudio
	Surfaces       map[string][]string
	DefaultSurface string
	Emitters       map[string]string
	Groups         map[string]map[string]*string
}

// MarshalJSON preserves schema-3 section names and validates assignment groups.
func (routing Routing) MarshalJSON() ([]byte, error) {
	fields := map[string]any{routingAmbientKey: routing.Ambient, routingTilesKey: routing.Tiles,
		routingSurfacesKey: routing.Surfaces, routingDefaultSurfaceKey: routing.DefaultSurface,
		routingEmittersKey: routing.Emitters}
	for group, assignments := range routing.Groups {
		if !schemaThreeAssignmentGroup(group) {
			return nil, fmt.Errorf("%w: unknown schema-3 assignment group %q", ErrLibrary, group)
		}
		fields[group] = assignments
	}

	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode routing: %w", err)
	}

	return encoded, nil
}

// UnmarshalJSON rejects unknown wire sections before replacing the routing value.
func (routing *Routing) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode routing: %w", err)
	}
	var next Routing
	next.Groups = make(map[string]map[string]*string)
	known := map[string]any{routingAmbientKey: &next.Ambient, routingTilesKey: &next.Tiles,
		routingSurfacesKey: &next.Surfaces, routingDefaultSurfaceKey: &next.DefaultSurface,
		routingEmittersKey: &next.Emitters}
	for name, value := range fields {
		if target, exists := known[name]; exists {
			decoder := json.NewDecoder(bytes.NewReader(value))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(target); err != nil {
				return fmt.Errorf("routing %s: %w", name, err)
			}
		} else {
			if !schemaThreeAssignmentGroup(name) {
				return fmt.Errorf("%w: unknown schema-3 routing section %q", ErrLibrary, name)
			}
			var assignments map[string]*string
			if err := json.Unmarshal(value, &assignments); err != nil {
				return fmt.Errorf("routing group %s: %w", name, err)
			}
			next.Groups[name] = assignments
		}
	}
	*routing = next

	return nil
}

func schemaThreeAssignmentGroup(name string) bool {
	return name == "music" || name == "dungeon" || name == "naval"
}

// TileAmbientSettings resolves profile defaults with explicit source overrides.
func (routing Routing) TileAmbientSettings(id string) (AmbientProfile, float64) {
	assignment := routing.Tiles[id]
	profile, gain := routing.Ambient[assignment.AmbientCue], 1.0
	if assignment.RadiusTiles != nil {
		profile.RadiusTiles = *assignment.RadiusTiles
	}
	if assignment.Gain != nil {
		gain = *assignment.Gain
	}

	return profile, gain
}

// TileTarget supplies a consumer-defined source label and optional frame members.
type TileTarget struct {
	ID, Name string
	Frames   []int
}

// AssignmentGroup supplies labels and optional inheritance without game names.
type AssignmentGroup struct {
	Label         string
	Targets       map[string]string
	DefaultTarget string
	Category      string
}

// AuthoringTargets supplies consumer-owned identities and category policy.
//
//nolint:revive // Preserve the published authoring.AuthoringTargets API name for existing consumers.
type AuthoringTargets struct {
	Tiles         []TileTarget
	Groups        map[string]AssignmentGroup
	Families      []string
	CueCategories map[string]string
}

// Targets returns detached consumer target metadata.
func (e *LibraryEditor) Targets() AuthoringTargets { return cloneAudioValue(e.targets) }

// AcousticFamilies returns supplied families or sorted authored taxonomy families.
func (e *LibraryEditor) AcousticFamilies() []string {
	if len(e.targets.Families) > 0 {
		return slices.Clone(e.targets.Families)
	}

	return sortedKeys(e.library.Taxonomy.RecordingFamilies)
}

// Routing returns a detached routing snapshot.
func (e *LibraryEditor) Routing() Routing { return cloneAudioValue(e.library.Routing) }

// SetRouting validates and commits routing as one history entry.
func (e *LibraryEditor) SetRouting(routing Routing) error {
	next := cloneAudioValue(e.library)
	next.Routing = routing

	return e.replace(next)
}

// RoutingChanged compares routing with the last saved baseline.
func (e *LibraryEditor) RoutingChanged() bool {
	return !bytes.Equal(encodeAudioValue(e.library.Routing), encodeAudioValue(e.baseline.Routing))
}

// ResolveAssignment reports the effective cue and whether the target has an explicit entry.
func (e *LibraryEditor) ResolveAssignment(group, target string) (string, bool) {
	assignments := e.library.Routing.Groups[group]
	value, explicit := assignments[target]
	if !explicit && e.targets.Groups[group].DefaultTarget != "" {
		value = assignments[e.targets.Groups[group].DefaultTarget]
	}
	if value == nil {
		return "", explicit
	}

	return *value, explicit
}
