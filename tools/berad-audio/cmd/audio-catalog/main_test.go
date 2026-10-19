package main

import (
	"bytes"
	"os"
	"path/filepath"
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
