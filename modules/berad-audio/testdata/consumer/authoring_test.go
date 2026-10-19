package consumer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

type pcmSource struct{ *bytes.Reader }

func (source pcmSource) Length() int64 { return source.Size() }

func TestIndependentSourceAnalysisAndFrozenAudition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.raw")
	encoded := bytes.Repeat([]byte{0x10, 0x10, 0x20, 0x20}, 441)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	decoder := func(data []byte, filename string, rate int) (audio.PCMStream, error) {
		if filename != path || rate != authoring.EditorSampleRate {
			return nil, errors.New("unexpected caller decoder input")
		}
		return pcmSource{bytes.NewReader(data)}, nil
	}
	source, err := authoring.LoadRecording(path, decoder)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, len(encoded)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := source.VerifySource(context.Background()); !errors.Is(err, authoring.ErrRecordingSourceChanged) {
		t.Fatal("source change was not observable", err)
	}
	stream, err := source.Stream(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(stream)
	if closeErr := stream.Close(); closeErr != nil || err != nil || !bytes.Equal(actual, encoded) {
		t.Fatalf("source replacement retargeted frozen audition: read=%v close=%v", err, closeErr)
	}
}

func TestIndependentAuthoringProjectApplyUndoSaveReopen(t *testing.T) {
	root := t.TempDir()
	file := "panel.raw"
	encoded := bytes.Repeat([]byte{0x10, 0x10, 0x20, 0x20}, 441)
	if err := os.WriteFile(filepath.Join(root, file), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	document := authoring.LibraryDocument{SchemaVersion: 3,
		Recordings: map[string]authoring.Recording{"click": {File: &file,
			Provenance: authoring.RecordingProvenance{OriginalFilename: file, SourceType: "generated", SourceURL: "unknown",
				SourceCreator: "consumer fixture", License: "MIT", ImportedFrom: "generated", Processing: []authoring.ProcessingStep{}}}},
		Cues: map[string]authoring.Cue{"ui/accept": {Name: "Accept", RecordingID: "click", VolumeModifier: 1, HowToHear: "Confirm selection"}},
		Taxonomy: authoring.Taxonomy{RecordingFamilies: map[string][]string{"signal": {"click"}},
			CueProfiles: map[string][]string{"interface": {"ui/accept"}}, Profiles: map[string]authoring.CueClassification{
				"interface": {TargetAcousticFamily: "signal", Phase: "press", Outcome: "accepted", MixRole: "interface",
					Emitters: []string{"panel"}, Contexts: []string{"menu"}}}},
		Routing: authoring.Routing{Groups: map[string]map[string]*string{"music": {"quiet": nil}}},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "project.json")
	if err := os.WriteFile(manifest, data, 0o600); err != nil {
		t.Fatal(err)
	}
	options := authoring.EditorOptions{
		Decode: func(data []byte, _ string, _ int) (audio.PCMStream, error) {
			return pcmSource{bytes.NewReader(data)}, nil
		},
		Calibration: func(string) authoring.CalibrationOptions { return authoring.DefaultCalibrationOptions() },
	}
	editor, err := authoring.OpenLibraryProject(manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	clip, err := editor.LoadRecording(context.Background(), filepath.Join(root, file))
	if err != nil {
		t.Fatal(err)
	}
	draft, err := editor.BeginRecordingRegion("click", clip)
	if err != nil {
		t.Fatal(err)
	}
	window := &authoring.PlaybackWindow{StartMs: 1, EndMs: 9}
	preview, err := draft.Preview(window)
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.ApplyRecordingRegion(context.Background(), draft, window); err != nil {
		t.Fatal(err)
	}
	if !editor.Undo() || !editor.Redo() {
		t.Fatal("region did not commit as one history edit")
	}
	if err := editor.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := authoring.OpenLibraryProject(manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	if *reopened.Recording("click").PlaybackWindow != *preview.Window || reopened.Dirty() {
		t.Fatal("saved region changed")
	}
	if _, exists := reopened.Routing().Groups["music"]["quiet"]; !exists {
		t.Fatal("explicit silence lost")
	}
	if _, err := reopened.RuntimeLibrary(int64(len(encoded))); err != nil {
		t.Fatal(err)
	}
}
