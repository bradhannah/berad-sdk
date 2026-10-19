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

const recordingID = "signal"
const profileID = "panel"

// Write creates a neutral schema-3 project with generated PCM in a test-owned directory.
func Write(tb testing.TB) (string, string, []byte) {
	tb.Helper()
	const directoryMode = 0o700
	const fileMode = 0o600
	root := tb.TempDir()
	assets := filepath.Join(root, "media")
	if err := os.Mkdir(assets, directoryMode); err != nil {
		tb.Fatal(err)
	}
	file := "signal.wav"
	if err := os.WriteFile(filepath.Join(assets, file), signalWAV(), fileMode); err != nil {
		tb.Fatal(err)
	}
	manifest, err := json.MarshalIndent(neutralDocument(file), "", "  ")
	if err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(root, "library.json")
	if err := os.WriteFile(path, manifest, fileMode); err != nil {
		tb.Fatal(err)
	}

	return path, assets, manifest
}

//nolint:mnd // Fixed RIFF/WAVE header offsets and PCM format words are the synthetic wire fixture.
func signalWAV() []byte {
	const payloadBytes = 441 * 4
	const headerBytes = 44
	data := make([]byte, headerBytes+payloadBytes)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], headerBytes+payloadBytes-8)
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], 44100)
	binary.LittleEndian.PutUint32(data[28:], 44100*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], payloadBytes)
	for offset := 44; offset < len(data); offset += 2 {
		binary.LittleEndian.PutUint16(data[offset:], 4096)
	}

	return data
}

func neutralDocument(file string) authoring.LibraryDocument {
	const schemaVersion = 3
	var recording authoring.Recording
	recording.File = &file
	recording.Provenance = authoring.RecordingProvenance{
		OriginalFilename: file, SourceType: "generated", SourceURL: "unknown", SourceCreator: "test generator",
		License: "MIT", ImportedFrom: "generated", Processing: []authoring.ProcessingStep{},
	}
	var document authoring.LibraryDocument
	document.SchemaVersion = schemaVersion
	document.Recordings = map[string]authoring.Recording{recordingID: recording}
	document.Cues = map[string]authoring.Cue{"panel/accept": {
		Name: "Accept", RecordingID: recordingID, VolumeModifier: 1, HowToHear: "Confirm panel",
		RecordingIDs: nil, Selection: "", SearchTerms: nil,
	}}
	document.Taxonomy = authoring.Taxonomy{
		RecordingFamilies: map[string][]string{recordingID: {recordingID}},
		CueProfiles:       map[string][]string{profileID: {"panel/accept"}}, ProfileSearchTerms: nil,
		Profiles: map[string]authoring.CueClassification{profileID: {
			TargetAcousticFamily: recordingID, Phase: "accept", Outcome: "confirmed", MixRole: "interface",
			Emitters: []string{profileID}, Contexts: []string{"menu"},
		}},
	}

	return document
}
