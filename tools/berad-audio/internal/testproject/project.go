// Package testproject supplies generated media and explicit neutral project data
// to tool boundary regressions. It never reads a consumer game's installation.
package testproject

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func Write(t testing.TB) (string, string, []byte) {
	t.Helper()
	root := t.TempDir()
	assets := filepath.Join(root, "media")
	if err := os.Mkdir(assets, 0o700); err != nil {
		t.Fatal(err)
	}
	file := "signal.wav"
	data := make([]byte, 44+441*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], 44100)
	binary.LittleEndian.PutUint32(data[28:], 44100*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	for offset := 44; offset < len(data); offset += 2 {
		binary.LittleEndian.PutUint16(data[offset:], 4096)
	}
	if err := os.WriteFile(filepath.Join(assets, file), data, 0o600); err != nil {
		t.Fatal(err)
	}
	provenance := authoring.RecordingProvenance{OriginalFilename: file, SourceType: "generated", SourceURL: "unknown", SourceCreator: "test generator", License: "MIT", ImportedFrom: "generated", Processing: []authoring.ProcessingStep{}}
	document := authoring.LibraryDocument{SchemaVersion: 3, Recordings: map[string]authoring.Recording{"signal": {File: &file, Provenance: provenance}},
		Cues:     map[string]authoring.Cue{"panel/accept": {Name: "Accept", RecordingID: "signal", VolumeModifier: 1, HowToHear: "Confirm panel"}},
		Taxonomy: authoring.Taxonomy{RecordingFamilies: map[string][]string{"signal": {"signal"}}, CueProfiles: map[string][]string{"panel": {"panel/accept"}}, Profiles: map[string]authoring.CueClassification{"panel": {TargetAcousticFamily: "signal", Phase: "accept", Outcome: "confirmed", MixRole: "interface", Emitters: []string{"panel"}, Contexts: []string{"menu"}}}}}
	manifest, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "library.json")
	if err := os.WriteFile(path, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, assets, manifest
}
