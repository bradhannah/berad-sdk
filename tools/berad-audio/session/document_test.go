package session_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/session"
)

func document(t *testing.T) *authoring.LibraryEditor {
	t.Helper()
	value := authoring.LibraryDocument{SchemaVersion: 3, Recordings: map[string]authoring.Recording{"signal": {Provenance: authoring.RecordingProvenance{OriginalFilename: "unknown", SourceType: "unknown", SourceURL: "unknown", SourceCreator: "unknown", License: "unknown", ImportedFrom: "unknown", Processing: []authoring.ProcessingStep{}}}},
		Cues: map[string]authoring.Cue{}, Taxonomy: authoring.Taxonomy{RecordingFamilies: map[string][]string{"signal": {"signal"}}, CueProfiles: map[string][]string{}, Profiles: map[string]authoring.CueClassification{}}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := authoring.OpenLibraryProject(path, authoring.DefaultEditorOptions(ebitengine.Decode))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func note(t *testing.T, doc *authoring.LibraryEditor, value string) {
	t.Helper()
	record := doc.Recording("signal")
	record.Note = value
	if err := doc.SetRecording("signal", record); err != nil {
		t.Fatal(err)
	}
}

func TestStaleCompletionKeepsPublishedBaselineAndNewerDraft(t *testing.T) {
	doc := document(t)
	note(t, doc, "published")
	var operations session.Documents
	job, err := operations.Begin(doc)
	if err != nil {
		t.Fatal(err)
	}
	result := job.Run(func(working *authoring.LibraryEditor) error { return working.Save() })
	if result.Error() != nil {
		t.Fatal(result.Error())
	}
	note(t, doc, "newer draft")
	completion := operations.Complete(doc, result, nil)
	if completion.Err != nil || !completion.Stale || !completion.PublicationChanged || completion.Document != doc || operations.Busy() {
		t.Fatalf("wrong completion: %+v", completion)
	}
	if doc.Recording("signal").Note != "newer draft" || !doc.Dirty() {
		t.Fatal("newer edit lost")
	}
	if err := doc.Save(); err != nil {
		t.Fatal("actual published baseline was discarded", err)
	}
}

func TestApprovalFailureStillKeepsPublicationFacts(t *testing.T) {
	doc := document(t)
	note(t, doc, "published")
	var operations session.Documents
	job, err := operations.Begin(doc)
	if err != nil {
		t.Fatal(err)
	}
	result := job.Run(func(working *authoring.LibraryEditor) error { return working.Save() })
	failure := errors.New("selection changed")
	completion := operations.Complete(doc, result, failure)
	if !errors.Is(completion.Err, failure) || !completion.PublicationChanged || completion.Document != doc {
		t.Fatalf("publication facts lost: %+v", completion)
	}
	if doc.Dirty() {
		t.Fatal("published baseline should match the retained working values")
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestBusyForeignAndDuplicateResultsCannotRetargetJobs(t *testing.T) {
	doc := document(t)
	var operations, other session.Documents
	job, err := operations.Begin(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := operations.Begin(doc); !errors.Is(err, session.ErrBusy) {
		t.Fatal(err)
	}
	calls := 0
	result := job.Run(func(*authoring.LibraryEditor) error { calls++; return nil })
	duplicate := job.Run(func(*authoring.LibraryEditor) error { calls++; return nil })
	if !errors.Is(duplicate.Error(), session.ErrExecuted) || calls != 1 {
		t.Fatal("worker ran twice")
	}
	if !other.Complete(doc, result, nil).Ignored || !operations.Busy() {
		t.Fatal("foreign completion retired a job")
	}
	if !operations.Complete(doc, duplicate, nil).Ignored || !operations.Busy() {
		t.Fatal("duplicate execution retired a job")
	}
	completion := operations.Complete(doc, result, nil)
	if completion.Ignored || completion.Err != nil || operations.Busy() {
		t.Fatalf("valid completion failed: %+v", completion)
	}
	if !operations.Complete(completion.Document, result, nil).Ignored {
		t.Fatal("completion replay was accepted")
	}
}
