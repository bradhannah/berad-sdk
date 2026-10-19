package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradhannah/berad-sdk/tools/berad-audio/internal/testproject"
)

func TestExportUsesSelectedProjectAndProtectsSourceAliases(t *testing.T) {
	manifest, assets, before := testproject.Write(t)
	root := filepath.Dir(manifest)
	inventory := filepath.Join(root, "inventory.json")
	guide := filepath.Join(root, "guide.md")
	var output bytes.Buffer
	args := []string{"-library", manifest, "-asset-root", "media", "-output", inventory, "-guide", guide, "-measure"}
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("export altered source", err)
	}
	alias := filepath.Join(root, "alias.json")
	if err := os.Link(manifest, alias); err != nil {
		t.Fatal(err)
	}
	if err := distinctOutputs([]string{alias, guide}, []string{manifest}); err == nil {
		t.Fatal("hard-link manifest alias accepted")
	}
	if err := distinctOutputs([]string{filepath.Join(assets, "signal.wav"), guide}, []string{filepath.Join(assets, "signal.wav")}); err == nil {
		t.Fatal("audio input overwrite accepted")
	}
	if err := distinctOutputs([]string{inventory, inventory}, []string{manifest}); err == nil {
		t.Fatal("overlapping output accepted")
	}
}

func TestProtectedInputInspectionFailureIsNotTreatedAsAbsence(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "report.json")
	err := distinctOutputs([]string{output}, []string{filepath.Join(root, "invalid\x00input")})
	if err == nil || !strings.Contains(err.Error(), "inspect protected input") {
		t.Fatalf("expected observable protected-input stat failure, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("validation created output: %v", err)
	}
}
