package authoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type jsonEdit struct {
	start int64
	end   int64
	data  []byte
}

// Save preserves untouched entries verbatim and replaces only edited records.
// An external change is a conflict even if the local document is not dirty.
func (e *LibraryEditor) Save() error {
	e.beginPublication(e.path, false)
	unlock, err := e.persistence.lock(e.path)
	if err != nil {
		return err
	}
	return errors.Join(e.saveLocked(), unlock())
}

func (e *LibraryEditor) saveLocked() error {
	if err := e.checkExternalChange(); err != nil {
		return err
	}
	if err := e.flushPendingDirectories(); err != nil {
		if e.manifestSyncPending {
			e.publication.State = PublicationSyncPending
		}

		return err
	}
	if err := e.Validate(); err != nil {
		return err
	}
	if !e.Dirty() {
		e.publication.State = PublicationUnchanged

		return nil
	}
	data := e.formattedChanges()
	info, err := os.Stat(e.path)
	if err != nil {
		return fmt.Errorf("stat audio manifest: %w", err)
	}
	err = e.persistence.publish(filepath.Dir(e.path), ".audio-library-*", data, info.Mode().Perm(),
		func(temporary string) error {
			if err := e.checkExternalChange(); err != nil {
				return err
			}
			if err := e.persistence.replace(temporary, e.path); err != nil {
				return fmt.Errorf("publish audio manifest: %w", err)
			}
			e.saved, e.baseline, e.manifestSyncPending = data, e.library, true
			e.requireDirectorySync(filepath.Dir(e.path))
			e.publication.State = PublicationSyncPending

			return nil
		})
	if err != nil {
		if e.manifestSyncPending {
			return fmt.Errorf("%w: %w", ErrAudioPublicationSync, err)
		}

		return fmt.Errorf("save audio library: %w", err)
	}
	if err := e.flushPendingDirectories(); err != nil {
		return err
	}
	e.publication.State = PublicationComplete

	return nil
}

// Validate checks schema, file references, and authored windows against decoded
// source lengths without opening an audio device or modifying any files.
func (e *LibraryEditor) Validate() error {
	if _, err := parseLibraryDocument(encodeAudioValue(e.library), e.validator); err != nil {
		return err
	}

	return e.validateWindows()
}

func (e *LibraryEditor) checkExternalChange() error {
	current, err := os.ReadFile(e.path)
	if err != nil {
		return fmt.Errorf("check audio library before save: %w", err)
	}
	if !bytes.Equal(current, e.saved) {
		return fmt.Errorf("%w: library changed on disk; reopen it before saving", ErrAudioLibrary)
	}

	return nil
}

func (e *LibraryEditor) validateWindows() error {
	for id, recording := range e.library.Recordings {
		if recording.File == nil {
			continue
		}
		if err := e.validateWindow(recording); err != nil {
			return fmt.Errorf("validate %s selection: %w", id, err)
		}
	}

	return nil
}

func (e *LibraryEditor) validateWindow(recording Recording) error {
	if !validAssetPath(*recording.File) {
		return fmt.Errorf("%w: invalid recording path", ErrAudioLibrary)
	}
	data, _, err := readRecordingSource(context.Background(), filepath.Join(e.assetRoot, filepath.FromSlash(*recording.File)))
	if err != nil {
		return fmt.Errorf("read selection source: %w", err)
	}
	stream, err := decodeRecording(data, *recording.File, e.decode, recording.PlaybackWindow, false)
	if err != nil {
		return err
	}
	return stream.Close()
}

// objectSpans maps exact JSON value ranges. Input was validated on open/edit.
func objectSpans(data []byte) map[string]jsonEdit {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if _, err := decoder.Token(); err != nil {
		panic(err)
	}
	spans := make(map[string]jsonEdit)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			panic(err)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			panic(err)
		}
		name, ok := key.(string)
		if !ok {
			panic("validated JSON object key is not a string")
		}
		end := decoder.InputOffset()
		spans[name] = jsonEdit{start: end - int64(len(raw)), end: end, data: raw}
	}

	return spans
}

func (e *LibraryEditor) formattedChanges() []byte {
	roots := objectSpans(e.saved)
	edits := entryEdits(roots["recordings"], e.library.Recordings)
	edits = append(edits, entryEdits(roots["cues"], e.library.Cues)...)
	if !bytes.Equal(encodeAudioValue(e.library.Routing), encodeAudioValue(e.baseline.Routing)) {
		edits = append(edits, routingEntryEdits(roots["routing"], e.library.Routing)...)
	}
	if !bytes.Equal(encodeAudioValue(e.library.Taxonomy), encodeAudioValue(e.baseline.Taxonomy)) {
		edits = append(edits, jsonEdit{start: roots["taxonomy"].start, end: roots["taxonomy"].end,
			data: encodeAudioValue(e.library.Taxonomy)})
	}
	slices.SortFunc(edits, func(a, b jsonEdit) int {
		if a.start > b.start {
			return -1
		}

		return 1
	})
	result := bytes.Clone(e.saved)
	for _, edit := range edits {
		result = append(append(bytes.Clone(result[:edit.start]), edit.data...), result[edit.end:]...)
	}

	return result
}

func entryEdits[T any](root jsonEdit, values map[string]T) []jsonEdit {
	spans := objectSpans(root.data)
	for key := range spans {
		if _, exists := values[key]; !exists {
			// Deleting an override changes this small object only. Other routing
			// sections, cues and recording entries retain their original bytes.
			return []jsonEdit{{start: root.start, end: root.end, data: encodeAudioValue(values)}}
		}
	}
	var edits []jsonEdit
	var additions []string
	for _, id := range sortedKeys(values) {
		encoded := encodeAudioValue(values[id])
		span, exists := spans[id]
		if !exists {
			additions = append(additions, string(encodeAudioValue(id))+": "+string(encoded))

			continue
		}
		var original T
		if err := json.Unmarshal(span.data, &original); err != nil {
			panic(err)
		}
		if bytes.Equal(encoded, encodeAudioValue(original)) {
			continue
		}
		edits = append(edits, jsonEdit{start: root.start + span.start, end: root.start + span.end, data: encoded})
	}
	if len(additions) > 0 {
		prefix := ""
		if len(spans) > 0 {
			prefix = ","
		}
		position := root.end - 1
		data := []byte(prefix + "\n    " + strings.Join(additions, ",\n    ") + "\n  ")
		edits = append(edits, jsonEdit{start: position, end: position, data: data})
	}

	return edits
}

func routingEntryEdits(root jsonEdit, routing Routing) []jsonEdit {
	fields := objectSpans(root.data)
	encoded := encodeAudioValue(routing)
	sections := []string{"ambient", "tiles", "surfaces", "emitters", "defaultSurface"}
	for group := range routing.Groups {
		sections = append(sections, group)
	}
	if len(fields) != len(sections) {
		return []jsonEdit{{start: root.start, end: root.end, data: encoded}}
	}
	for _, name := range sections {
		field, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(field.data), []byte("null")) {
			return []jsonEdit{{start: root.start, end: root.end, data: encoded}}
		}
		field.start, field.end = field.start+root.start, field.end+root.start
		fields[name] = field
	}
	edits := entryEdits(fields["ambient"], routing.Ambient)
	edits = append(edits, entryEdits(fields["tiles"], routing.Tiles)...)
	edits = append(edits, entryEdits(fields["surfaces"], routing.Surfaces)...)
	edits = append(edits, entryEdits(fields["emitters"], routing.Emitters)...)
	for group, assignments := range routing.Groups {
		edits = append(edits, entryEdits(fields[group], assignments)...)
	}
	field := fields["defaultSurface"]
	if value := encodeAudioValue(routing.DefaultSurface); !bytes.Equal(field.data, value) {
		edits = append(edits, jsonEdit{start: field.start, end: field.end, data: value})
	}
	return edits
}
