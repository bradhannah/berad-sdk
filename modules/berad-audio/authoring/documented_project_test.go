package authoring_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestDocumentedNeutralProjectIsACompleteSchemaThreeLibrary(t *testing.T) {
	guide, err := os.ReadFile("../docs/QUICKSTART.md")
	if err != nil {
		t.Fatal(err)
	}
	_, example, found := bytes.Cut(guide, []byte("```json\n"))
	if !found {
		t.Fatal("quickstart library example missing")
	}
	example, rest, found := bytes.Cut(example, []byte("\n```"))
	if !found {
		t.Fatal("quickstart library example unfinished")
	}
	document, err := authoring.ParseDocument(example, nil)
	if err != nil {
		t.Fatal("documented project is not usable:", err)
	}
	if document.Cues["panel/accept"].RecordingID != "panel_signal" || document.Recordings["panel_signal"].File != nil {
		t.Fatal("quickstart must begin with an explicit neutral silent slot")
	}
	_, descriptorExample, found := bytes.Cut(rest, []byte("```json\n"))
	if !found {
		t.Fatal("quickstart descriptor example missing")
	}
	descriptorExample, _, _ = bytes.Cut(descriptorExample, []byte("\n```"))
	path := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(path, descriptorExample, 0o600); err != nil {
		t.Fatal(err)
	}
	descriptor, err := authoring.ReadProjectDescriptor(path)
	if err != nil || descriptor.LibraryPath(path) != filepath.Join(filepath.Dir(path), "library.json") {
		t.Fatal("documented descriptor is not usable:", err)
	}
}
