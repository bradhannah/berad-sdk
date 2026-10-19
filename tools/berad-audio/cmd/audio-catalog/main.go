// Command audio-catalog exports an explicitly selected manifest and asset root.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/catalog"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("audio-catalog", flag.ContinueOnError)
	manifest := flags.String("library", "", "Explicit schema-3 library manifest")
	assetRoot := flags.String("asset-root", ".", "Asset root, relative to the selected manifest")
	inventory := flags.String("output", "", "Inventory JSON destination (required)")
	guide := flags.String("guide", "", "Markdown guide destination (required)")
	measure := flags.Bool("measure", false, "Measure decoded sample peak/RMS; not listening acceptance")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *manifest == "" || *inventory == "" || *guide == "" || flags.NArg() != 0 {
		return fmt.Errorf("%w: require -library, -output, -guide and no positional arguments", catalog.ErrAudioCatalog)
	}
	path, err := filepath.Abs(*manifest)
	if err != nil {
		return err
	}
	root := *assetRoot
	if !filepath.IsAbs(root) {
		root = filepath.Join(filepath.Dir(path), root)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read supplied manifest: %w", err)
	}
	value, err := catalog.Build(data, os.DirFS(root), catalog.Options{Decode: ebitengine.Decode, Measure: *measure})
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	// Derived reports never overwrite the manifest or any input recording, including
	// hard-link/symlink aliases. Validate both destinations before writing either.
	protected := []string{path}
	for _, asset := range value.Assets {
		if asset.Path != "" {
			protected = append(protected, filepath.Join(root, filepath.FromSlash(asset.Path)))
		}
	}
	if err := distinctOutputs([]string{*inventory, *guide}, protected); err != nil {
		return err
	}
	if err := os.WriteFile(*inventory, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	text := value.GenerationGuide(catalog.ReportOptions{ManifestLabel: path, AssetRootLabel: root})
	if err := os.WriteFile(*guide, []byte(text), 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	_, err = fmt.Fprintf(output, "Exported %d recordings and %d cues from %s\n", len(value.Assets), len(value.Moments), path)
	return err
}

func distinctOutputs(outputs, protected []string) error {
	paths := append([]string(nil), protected...)
	for _, output := range outputs {
		absolute, err := filepath.Abs(output)
		if err != nil {
			return err
		}
		info, statErr := os.Stat(absolute)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
		if err != nil {
			return err
		}
		resolved := filepath.Join(parent, filepath.Base(absolute))
		for _, prior := range paths {
			other, err := filepath.Abs(prior)
			if err != nil {
				return err
			}
			otherInfo, _ := os.Stat(other)
			if absolute == other || resolved == other || info != nil && otherInfo != nil && os.SameFile(info, otherInfo) {
				return fmt.Errorf("%w: output aliases an input or another report", catalog.ErrAudioCatalog)
			}
		}
		paths = append(paths, resolved)
	}
	return nil
}
