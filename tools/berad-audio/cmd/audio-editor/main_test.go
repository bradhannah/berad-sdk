package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	editor "github.com/bradhannah/berad-sdk/tools/berad-audio/editor"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/internal/testproject"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/project"
)

func TestCheckDoesNotLaunchUIOrRewriteManifest(t *testing.T) {
	manifest, _, before := testproject.Write(t)
	descriptor := filepath.Join(filepath.Dir(manifest), "project.json")
	data, err := json.Marshal(project.Config{SchemaVersion: 1, Library: "library.json", AssetRoot: "media"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(descriptor, data, 0o600); err != nil {
		t.Fatal(err)
	}
	launch := func(*authoring.LibraryEditor, editor.Options) error { t.Fatal("check opened the editor"); return nil }
	var output bytes.Buffer
	if err := run([]string{"-project", descriptor, "-check"}, &output, launch); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("check changed source", err)
	}
	if !bytes.Contains(output.Bytes(), []byte("1 recordings, 1 cues")) {
		t.Fatal(output.String())
	}
	for _, args := range [][]string{nil, {"-library", manifest, "-project", descriptor}, {"-project", descriptor, "extra"}} {
		if err := run(args, &output, launch); err == nil {
			t.Fatal("invalid arguments accepted", args)
		}
	}
}
