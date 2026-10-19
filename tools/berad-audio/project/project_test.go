package project_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/tools/berad-audio/internal/testproject"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/project"
)

func TestProjectRelativePathsAndInvalidConfig(t *testing.T) {
	manifest, _, _ := testproject.Write(t)
	path := filepath.Join(filepath.Dir(manifest), "project.json")
	config := project.Config{SchemaVersion: 1, Library: "library.json", AssetRoot: "media"}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, _, err := project.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	source, err := doc.RecordingPath("signal")
	if err != nil || source != filepath.Join(filepath.Dir(manifest), "media", "signal.wav") {
		t.Fatal(source, err)
	}
	for _, invalid := range []string{`{"schemaVersion":1,"schemaVersion":1,"library":"library.json"}`, `{"schemaVersion":2,"library":"library.json"}`, `{"schemaVersion":1,"library":"library.json","typo":1}`, string(data) + " {}"} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := project.Open(path); err == nil {
			t.Fatal("invalid configuration accepted", invalid)
		}
	}
}
