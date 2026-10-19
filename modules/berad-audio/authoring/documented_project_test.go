package authoring_test

import (
	"bytes"
	"os"
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
	example, _, found = bytes.Cut(example, []byte("\n```"))
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
}
