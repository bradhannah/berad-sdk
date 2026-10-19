package authoring

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/stretchr/testify/require"
)

// These historic test identities are data only: no production manifest, game
// enum, media or target registry participates in the transferred regressions.
func editorFixture(t *testing.T) (*LibraryEditor, []byte) {
	t.Helper()
	root := t.TempDir()
	file := "signal.wav"
	wav := editorTestWAV()
	require.NoError(t, os.WriteFile(filepath.Join(root, file), wav, 0o600))
	provenance := unknownRecordingProvenance()
	provenance.OriginalFilename, provenance.SourceType = file, "generated"
	provenance.SourceCreator, provenance.License = "test generator", "MIT"
	document := LibraryDocument{SchemaVersion: 3,
		Recordings: map[string]Recording{
			"healing_01": {File: &file, Provenance: provenance},
			"healing_02": {Provenance: unknownRecordingProvenance()},
		},
		Cues: map[string]Cue{"sfx/SoundHeal": {Name: "Test signal", RecordingID: "healing_01", VolumeModifier: 1, HowToHear: "Test preview"}},
		Taxonomy: Taxonomy{RecordingFamilies: map[string][]string{"signal": {"healing_01", "healing_02"}},
			CueProfiles: map[string][]string{"panel": {"sfx/SoundHeal"}},
			Profiles: map[string]CueClassification{"panel": {TargetAcousticFamily: "signal", Phase: "press", Outcome: "accepted",
				MixRole: "interface", Emitters: []string{"panel"}, Contexts: []string{"menu"}}}},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	require.NoError(t, err)
	manifest := filepath.Join(root, "library.json")
	require.NoError(t, os.WriteFile(manifest, data, 0o600))
	editor, err := OpenLibraryEditor(manifest)
	require.NoError(t, err)
	return editor, wav
}

// OpenLibraryEditor is test-only shorthand, never an embedded/default project.
func OpenLibraryEditor(path string) (*LibraryEditor, error) {
	return OpenLibraryProject(path, EditorOptions{Decode: ebitengine.Decode,
		Calibration: func(string) CalibrationOptions { return DefaultCalibrationOptions() }})
}

func editorTestWAV() []byte {
	pcm := make([]byte, EditorSampleRate/10*decodedPCMBytesPerFrame)
	for frame := EditorSampleRate / 50; frame < EditorSampleRate*3/50; frame++ {
		binary.LittleEndian.PutUint16(pcm[frame*4+2:], 16000)
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+len(pcm)))
	wav.WriteString("WAVEfmt ")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(1))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(2))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(EditorSampleRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(EditorSampleRate*4))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(4))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(16))
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(len(pcm)))
	wav.Write(pcm)
	return wav.Bytes()
}
