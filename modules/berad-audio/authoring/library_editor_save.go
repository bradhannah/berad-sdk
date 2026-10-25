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

// ErrLibraryChangedOnDisk reports that the manifest's bytes differ from those
// this editor last read or wrote: another writer replaced it. Reopen the
// library to continue; this editor's unsaved edits are not merged.
var ErrLibraryChangedOnDisk = fmt.Errorf("%w: library changed on disk; reopen it before saving", ErrAudioLibrary)

// Save preserves untouched entries verbatim and replaces only edited records.
// An external change is a conflict even if the local document is not dirty.
func (e *LibraryEditor) Save() error {
	return e.save(e.Validate)
}

// SaveChanged is Save for an editor that saves after every edit. It decodes
// only the recordings whose file or excerpt changed since the last save, so a
// missing or broken source elsewhere in the library does not block saving.
func (e *LibraryEditor) SaveChanged() error {
	return e.save(e.validateChanged)
}

// CheckDisk returns ErrLibraryChangedOnDisk when the manifest no longer holds
// the bytes this editor last read or wrote. It changes nothing.
func (e *LibraryEditor) CheckDisk() error {
	return e.checkExternalChange()
}

func (e *LibraryEditor) save(validate func() error) error {
	e.beginPublication(e.path, false)
	unlock, err := e.persistence.lock(e.path)
	if err != nil {
		return err
	}

	return errors.Join(e.saveLocked(validate), unlock())
}

func (e *LibraryEditor) saveLocked(validate func() error) error {
	if err := e.checkExternalChange(); err != nil {
		return err
	}
	if err := e.flushPendingDirectories(); err != nil {
		if e.manifestSyncPending {
			e.publication.State = PublicationSyncPending
		}

		return err
	}
	if err := validate(); err != nil {
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
		return ErrLibraryChangedOnDisk
	}

	return nil
}

func (e *LibraryEditor) validateChanged() error {
	if _, err := parseLibraryDocument(encodeAudioValue(e.library), e.validator); err != nil {
		return err
	}
	for recordingID, recording := range e.library.Recordings {
		if recording.File == nil {
			continue
		}
		saved, exists := e.baseline.Recordings[recordingID]
		if exists && bytes.Equal(encodeAudioValue(saved.File), encodeAudioValue(recording.File)) &&
			bytes.Equal(encodeAudioValue(saved.PlaybackWindow), encodeAudioValue(recording.PlaybackWindow)) {
			continue
		}
		if err := e.validateWindow(recording); err != nil {
			return fmt.Errorf("validate %s selection: %w", recordingID, err)
		}
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
	path := filepath.Join(e.assetRoot, filepath.FromSlash(*recording.File))
	data, _, err := readRecordingSource(context.Background(), path)
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
	edits := entryEdits(e.saved, roots["recordings"], e.library.Recordings)
	if e.library.SchemaVersion != e.baseline.SchemaVersion {
		version := roots["schemaVersion"]
		edits = append(edits, jsonEdit{start: version.start, end: version.end,
			data: encodeAudioValue(e.library.SchemaVersion)})
	}
	edits = append(edits, entryEdits(e.saved, roots["cues"], e.library.Cues)...)
	if !bytes.Equal(encodeAudioValue(e.library.Routing), encodeAudioValue(e.baseline.Routing)) {
		edits = append(edits, routingEntryEdits(e.saved, roots["routing"], e.library.Routing)...)
	}
	if !bytes.Equal(encodeAudioValue(e.library.Taxonomy), encodeAudioValue(e.baseline.Taxonomy)) {
		edits = append(edits, taxonomyEntryEdits(e.saved, roots["taxonomy"], e.library.Taxonomy)...)
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

// entryEdits changes an object of the saved document entry by entry; root
// holds its place in document.
func entryEdits[T any](document []byte, root jsonEdit, values map[string]T) []jsonEdit {
	spans := objectSpans(root.data)
	for key := range spans {
		if _, exists := values[key]; !exists {
			// Deleting an entry rewrites this object only, and its other entries
			// keep their original bytes, as do the other sections.
			return []jsonEdit{rebuiltObject(lineIndent(document, root.start), root, values)}
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
		edits = append(edits, additionEdit(lineIndent(document, root.start), root, additions))
	}

	return edits
}

// additionEdit appends entries after an object's last one, on lines indented
// like it, or fills an empty object whose line is indented by indent.
func additionEdit(indent string, root jsonEdit, additions []string) jsonEdit {
	entries := orderedEntries(root.data)
	if len(entries) == 0 {
		position := root.end - 1
		lead := "\n" + indent + jsonIndent
		data := []byte(lead + strings.Join(additions, ","+lead) + "\n" + indent)

		return jsonEdit{start: position, end: position, data: data}
	}
	lead := entryLead(root.data, entries, len(entries)-1)
	position := root.start + entries[len(entries)-1].end
	data := []byte("," + lead + strings.Join(additions, ","+lead))

	return jsonEdit{start: position, end: position, data: data}
}

// taxonomyEntryEdits changes the taxonomy's sections entry by entry, unless
// its set of sections changed.
func taxonomyEntryEdits(document []byte, root jsonEdit, taxonomy Taxonomy) []jsonEdit {
	fields := objectSpans(root.data)
	names := []string{"recordingFamilies", "cueProfiles", "profiles"}
	if len(taxonomy.ProfileSearchTerms) > 0 {
		names = append(names, "profileSearchTerms")
	}
	if len(fields) != len(names) {
		return []jsonEdit{{start: root.start, end: root.end, data: encodeAudioValue(taxonomy)}}
	}
	for _, name := range names {
		field, exists := fields[name]
		if !exists {
			return []jsonEdit{{start: root.start, end: root.end, data: encodeAudioValue(taxonomy)}}
		}
		field.start, field.end = field.start+root.start, field.end+root.start
		fields[name] = field
	}
	edits := entryEdits(document, fields["recordingFamilies"], taxonomy.RecordingFamilies)
	edits = append(edits, entryEdits(document, fields["cueProfiles"], taxonomy.CueProfiles)...)
	edits = append(edits, entryEdits(document, fields["profiles"], taxonomy.Profiles)...)
	if len(taxonomy.ProfileSearchTerms) > 0 {
		edits = append(edits, entryEdits(document, fields["profileSearchTerms"], taxonomy.ProfileSearchTerms)...)
	}

	return edits
}

// routingSection is one of routing's sections: its name, its entries as
// each would be saved, and how to change the section the file holds.
type routingSection struct {
	name    string
	entries map[string]json.RawMessage
	edit    func(field jsonEdit) []jsonEdit
}

// sectionOf describes a routing section of document.
func sectionOf[T any](document []byte, name string, values map[string]T) routingSection {
	entries := make(map[string]json.RawMessage, len(values))
	for id, value := range values {
		entries[id] = encodeAudioValue(value)
	}

	return routingSection{name: name, entries: entries, edit: func(field jsonEdit) []jsonEdit {
		return entryEdits(document, field, values)
	}}
}

// routingEntryEdits changes routing's sections entry by entry. A section the
// file lacks is added after the others, laid out like them, when it has
// entries or a default surface. A section the file holds as null, or one
// routing no longer has, rewrites routing whole.
func routingEntryEdits(document []byte, root jsonEdit, routing Routing) []jsonEdit {
	fields := objectSpans(root.data)
	const fixedSections = 4
	sections := make([]routingSection, 0, fixedSections+len(routing.Groups))
	sections = append(sections,
		sectionOf(document, routingAmbientKey, routing.Ambient), sectionOf(document, routingTilesKey, routing.Tiles),
		sectionOf(document, routingSurfacesKey, routing.Surfaces),
		sectionOf(document, routingEmittersKey, routing.Emitters))
	for _, group := range sortedKeys(routing.Groups) {
		sections = append(sections, sectionOf(document, group, routing.Groups[group]))
	}
	known := map[string]bool{routingDefaultSurfaceKey: true}
	for _, section := range sections {
		known[section.name] = true
	}
	for name, field := range fields {
		if !known[name] || bytes.Equal(bytes.TrimSpace(field.data), []byte("null")) {
			return []jsonEdit{{start: root.start, end: root.end, data: encodeAudioValue(routing)}}
		}
	}
	indent := lineIndent(document, root.start)
	var edits []jsonEdit
	var additions []string
	for _, section := range sections {
		field, exists := fields[section.name]
		if !exists {
			if len(section.entries) > 0 {
				additions = append(additions, string(encodeAudioValue(section.name))+": "+
					indentedObject(indent+jsonIndent, section.entries))
			}

			continue
		}
		field.start, field.end = field.start+root.start, field.end+root.start
		edits = append(edits, section.edit(field)...)
	}
	value := encodeAudioValue(routing.DefaultSurface)
	if field, exists := fields[routingDefaultSurfaceKey]; exists && !bytes.Equal(field.data, value) {
		edits = append(edits, jsonEdit{start: field.start + root.start, end: field.end + root.start, data: value})
	} else if !exists && routing.DefaultSurface != "" {
		additions = append(additions, string(encodeAudioValue(routingDefaultSurfaceKey))+": "+string(value))
	}
	if len(additions) > 0 {
		edits = append(edits, additionEdit(indent, root, additions))
	}

	return edits
}
