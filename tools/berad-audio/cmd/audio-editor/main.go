// Command audio-editor edits an explicitly selected audio project in a terminal.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	editor "github.com/bradhannah/berad-sdk/tools/berad-audio/editor"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/project"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, editor.Run); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer, launch func(*authoring.LibraryEditor, editor.Options) error) error {
	flags := flag.NewFlagSet("audio-editor", flag.ContinueOnError)
	manifest := flags.String("library", "", "Explicit schema-3 audio manifest (neutral defaults)")
	descriptor := flags.String("project", "", "Explicit version-1 tool project descriptor")
	check := flags.Bool("check", false, "Validate without opening audio, terminal UI, or writing files")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return fmt.Errorf("parse editor options: %w", err)
	}
	if flags.NArg() != 0 || (*manifest == "") == (*descriptor == "") {
		return fmt.Errorf("%w: choose exactly one -library or -project", authoring.ErrLibrary)
	}
	var doc *authoring.LibraryEditor
	var options editor.Options
	var err error
	if *descriptor != "" {
		doc, options, err = project.Open(*descriptor)
	} else {
		doc, options, err = project.OpenLibrary(*manifest)
	}
	if err != nil {
		return fmt.Errorf("open editor project: %w", err)
	}
	if *check {
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("validate editor project: %w", err)
		}
		_, err = fmt.Fprintf(output, "Valid library: %d recordings, %d cues\n", len(doc.RecordingIDs()), len(doc.CueIDs()))
		if err != nil {
			return fmt.Errorf("report editor validation: %w", err)
		}

		return nil
	}

	return launch(doc, options)
}
