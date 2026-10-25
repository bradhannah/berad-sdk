package authoring

import (
	"fmt"
	"slices"
	"strings"
)

// UnclassifiedProfile is the classification profile CreateCue files a new cue
// under when none is named. CreateCue adds it, with every field unknown, when
// the library does not have it yet.
const UnclassifiedProfile = "unclassified"

// CreateCue adds a new cue in one edit. Its recordings must already exist; an
// empty profile files it under UnclassifiedProfile. Cue IDs are references, so
// an existing ID is refused rather than replaced.
func (e *LibraryEditor) CreateCue(cueID string, cue Cue, profile string) error {
	if _, exists := e.library.Cues[cueID]; exists {
		return fmt.Errorf("%w: cue %s already exists", ErrAudioLibrary, cueID)
	}
	next := cloneAudioValue(e.library)
	if profile == "" {
		profile = UnclassifiedProfile
		if _, exists := next.Taxonomy.Profiles[profile]; !exists {
			next.Taxonomy.Profiles[profile] = unknownClassification()
		}
	}
	if _, exists := next.Taxonomy.Profiles[profile]; !exists {
		return fmt.Errorf("%w: unknown classification profile %s", ErrAudioLibrary, profile)
	}
	next.Cues[cueID] = cue
	next.Taxonomy.CueProfiles[profile] = append(next.Taxonomy.CueProfiles[profile], cueID)
	slices.Sort(next.Taxonomy.CueProfiles[profile])

	return e.replace(next)
}

func unknownClassification() CueClassification {
	return CueClassification{TargetAcousticFamily: provenanceUnknown, Phase: provenanceUnknown,
		Outcome: provenanceUnknown, MixRole: provenanceUnknown,
		Emitters: []string{provenanceUnknown}, Contexts: []string{provenanceUnknown}}
}

// DeleteCue removes a cue, its classification and its ambient settings in one
// edit. It refuses while other routing names the cue (see RoutingUses), and it
// keeps the cue's recordings, which no cue may play afterwards.
func (e *LibraryEditor) DeleteCue(cueID string) error {
	if _, exists := e.library.Cues[cueID]; !exists {
		return fmt.Errorf("%w: unknown cue %s", ErrAudioLibrary, cueID)
	}
	if uses := e.RoutingUses(cueID); len(uses) > 0 {
		return fmt.Errorf("%w: cue %s is still routed by %s", ErrAudioLibrary, cueID, strings.Join(uses, ", "))
	}
	next := cloneAudioValue(e.library)
	delete(next.Cues, cueID)
	removeTaxonomyID(next.Taxonomy.CueProfiles, cueID)
	delete(next.Routing.Ambient, cueID)

	return e.replace(next)
}

// RoutingUses describes the routing, other than the cue's own ambient settings,
// that names a cue: tiles playing it, surfaces stepping through it, emitters
// and assignment groups. Each use reads like "tile grass" or "music overworld";
// the list is sorted.
func (e *LibraryEditor) RoutingUses(cueID string) []string {
	routing := e.library.Routing
	var uses []string
	for key, tile := range routing.Tiles {
		if tile.AmbientCue == cueID {
			uses = append(uses, "tile "+key)
		}
	}
	for surface, cues := range routing.Surfaces {
		if slices.Contains(cues, cueID) {
			uses = append(uses, "surface "+surface)
		}
	}
	for role, cue := range routing.Emitters {
		if cue == cueID {
			uses = append(uses, "emitter "+role)
		}
	}
	for group, assignments := range routing.Groups {
		for target, cue := range assignments {
			if cue != nil && *cue == cueID {
				uses = append(uses, group+" "+target)
			}
		}
	}
	slices.Sort(uses)

	return uses
}

// DeleteRecording removes a recording that no cue plays, with its acoustic
// family membership, in one edit. Its file stays on disk, as after Undo of an
// import.
func (e *LibraryEditor) DeleteRecording(recordingID string) error {
	if _, exists := e.library.Recordings[recordingID]; !exists {
		return fmt.Errorf("%w: unknown recording %s", ErrAudioLibrary, recordingID)
	}
	if uses := e.Usages(recordingID); len(uses) > 0 {
		return fmt.Errorf("%w: recording %s is played by %s", ErrAudioLibrary, recordingID, strings.Join(uses, ", "))
	}
	next := cloneAudioValue(e.library)
	delete(next.Recordings, recordingID)
	removeTaxonomyID(next.Taxonomy.RecordingFamilies, recordingID)

	return e.replace(next)
}

// SetRecordingProvenance replaces a recording's provenance in one edit without
// decoding its source. Unknown facts are spelled "unknown" and Processing is an
// empty list, not nil, as the schema requires.
func (e *LibraryEditor) SetRecordingProvenance(recordingID string, provenance RecordingProvenance) error {
	if _, exists := e.library.Recordings[recordingID]; !exists {
		return fmt.Errorf("%w: unknown recording %s", ErrAudioLibrary, recordingID)
	}
	next := cloneAudioValue(e.library)
	recording := next.Recordings[recordingID]
	recording.Provenance = provenance
	next.Recordings[recordingID] = recording

	return e.replace(next)
}
