package authoring

import (
	"fmt"
	"io/fs"
	"math"
	"path"
	"strings"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

func validAssetPath(name string) bool {
	return fs.ValidPath(name) && !strings.Contains(name, "\\") && path.Clean(name) == name
}

// PlaybackDefinition projects only playback properties; authored routing,
// taxonomy and provenance are not inferred runtime policy.
func PlaybackDefinition(document LibraryDocument) (audio.Definition, error) {
	definition := audio.Definition{Recordings: make(map[audio.RecordingID]audio.Recording), Cues: make(map[audio.CueID]audio.Cue)}
	for id, source := range document.Recordings {
		var recording audio.Recording
		if source.File != nil {
			recording.File = *source.File
		}
		if source.CalibrationGainDB != nil {
			recording.CalibrationDB = *source.CalibrationGainDB
		}
		if window := source.PlaybackWindow; window != nil {
			if window.StartMs < 0 || window.EndMs <= window.StartMs || window.EndMs > math.MaxInt64/int64(time.Millisecond) {
				return audio.Definition{}, fmt.Errorf("%w: recording %s window exceeds duration range", ErrLibrary, id)
			}
			recording.Window = &audio.Window{Start: time.Duration(window.StartMs) * time.Millisecond, End: time.Duration(window.EndMs) * time.Millisecond}
		}
		definition.Recordings[audio.RecordingID(id)] = recording
	}
	for id, source := range document.Cues {
		cue := audio.Cue{Gain: source.VolumeModifier}
		for _, recording := range source.Recordings() {
			cue.Recordings = append(cue.Recordings, audio.RecordingID(recording))
		}
		definition.Cues[audio.CueID(id)] = cue
	}
	return definition, nil
}
