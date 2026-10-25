package authoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// ErrProjectChangedOnDisk reports that a descriptor no longer holds the bytes
// its writer last read. Read it again before changing it.
var ErrProjectChangedOnDisk = fmt.Errorf("%w: project descriptor changed on disk; read it again", ErrLibrary)

const (
	cueCategoriesKey = "CueCategories"
	categoryGainsKey = "categoryGainsDb"
)

// CreateProjectDescriptor writes a version-1 descriptor that names library,
// relative to the descriptor's directory unless absolute, and nothing else:
// the oldest version that holds it, so every editor reading version 1 can
// open it until something needs version 2. The file appears complete or not
// at all, and an existing file is never replaced.
func CreateProjectDescriptor(path, library string) error {
	if strings.TrimSpace(library) == "" {
		return fmt.Errorf("%w: a project descriptor names its library", ErrLibrary)
	}
	data := fmt.Sprintf("{\n  \"schemaVersion\": %d,\n  \"library\": %s\n}\n",
		ProjectDescriptorVersionOne, encodeAudioValue(library))

	return createFile(path, ".audio-project-*", []byte(data))
}

// SetDescriptorCueCategory records one cue's category in the descriptor at
// path, in targets.CueCategories, and returns the bytes it wrote. An empty
// category removes the cue's entry, so CueCategory falls back to its default.
// expected holds the descriptor's bytes as last read; a file that differs from
// them now is ErrProjectChangedOnDisk. Every byte outside the categories is
// kept, and the file is replaced complete or not at all.
func SetDescriptorCueCategory(path string, expected []byte, cueID, category string) ([]byte, error) {
	if category != "" && !validCueCategory(category) {
		return nil, fmt.Errorf("%w: unknown cue category %s", ErrLibrary, category)
	}
	descriptor, err := ParseProjectDescriptor(expected)
	if err != nil {
		return nil, err
	}
	categories := maps.Clone(descriptor.Targets.CueCategories)
	if categories == nil {
		categories = make(map[string]string)
	}
	if category == "" {
		delete(categories, cueID)
	} else {
		categories[cueID] = category
	}

	return writeDescriptor(path, expected, withCueCategories(expected, categories))
}

// SetDescriptorCategoryGain records the adjustment for a cue category, in
// dB, in the descriptor at path, in categoryGainsDb, and returns the bytes it
// wrote. Zero removes the category's entry. A version-1 descriptor becomes
// version 2 when it gains its first category gain, and never goes back.
// expected and the file are checked and replaced as SetDescriptorCueCategory
// does; every other byte is kept.
func SetDescriptorCategoryGain(path string, expected []byte, category string, gainDB float64) ([]byte, error) {
	if !validCueCategory(category) || !finiteRange(gainDB, calibrationMinGain, calibrationMaxGain) {
		return nil, fmt.Errorf("%w: a category gain needs a known category and [-48,12] dB", ErrLibrary)
	}
	descriptor, err := ParseProjectDescriptor(expected)
	if err != nil {
		return nil, err
	}
	gains := maps.Clone(descriptor.CategoryGainsDB)
	if gains == nil {
		gains = make(map[string]float64)
	}
	if gainDB == 0 {
		delete(gains, category)
	} else {
		gains[category] = gainDB
	}
	data := expected
	if len(gains) == 0 {
		data = deleteObjectEntry(data, categoryGainsKey)
	} else {
		data = setObjectEntry(data, categoryGainsKey, func(prefix string) []byte { return indentedJSON(gains, prefix) })
		if descriptor.SchemaVersion < ProjectDescriptorVersionTwo {
			data = setObjectEntry(data, "schemaVersion", func(string) []byte {
				return encodeAudioValue(ProjectDescriptorVersionTwo)
			})
		}
	}

	return writeDescriptor(path, expected, data)
}

// SetDescriptorCalibrationTarget sets the loudness, in dB, that measuring a
// recording aims at when its acoustic family names no policy of its own: the
// descriptor's calibration target. Its method stays, or is active RMS when the
// descriptor names none. expected and the file are checked and replaced as
// SetDescriptorCueCategory does; every other byte is kept.
func SetDescriptorCalibrationTarget(path string, expected []byte, targetDB float64) ([]byte, error) {
	if !finiteRange(targetDB, calibrationMinTarget, calibrationMaxTarget) {
		return nil, fmt.Errorf("%w: a calibration target is in [-60,-6] dB", ErrLibrary)
	}
	descriptor, err := ParseProjectDescriptor(expected)
	if err != nil {
		return nil, err
	}
	policy := DefaultCalibrationOptions()
	if descriptor.Calibration != nil {
		policy.Method = descriptor.Calibration.Method
	}
	policy.TargetDB = targetDB
	data := setObjectEntry(expected, "calibration", func(string) []byte {
		return []byte(fmt.Sprintf(`{"Method": %s, "TargetDB": %s}`,
			encodeAudioValue(string(policy.Method)), encodeAudioValue(policy.TargetDB)))
	})

	return writeDescriptor(path, expected, data)
}

// writeDescriptor checks the new bytes parse, then replaces the descriptor at
// path with them, complete or not at all, only while it still holds expected.
func writeDescriptor(path string, expected, data []byte) ([]byte, error) {
	if _, err := ParseProjectDescriptor(data); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("write audio project: %w", err)
	}
	directory := filepath.Dir(path)
	err = publishTemporaryFile(directory, ".audio-project-*", data, info.Mode().Perm(), func(temporary string) error {
		current, err := os.ReadFile(path) //nolint:gosec // The caller's own descriptor, read just before replacing it.
		if err != nil {
			return fmt.Errorf("check audio project: %w", err)
		}
		if !bytes.Equal(current, expected) {
			return ErrProjectChangedOnDisk
		}

		return os.Rename(temporary, path)
	})
	if err != nil {
		return nil, fmt.Errorf("write audio project: %w", err)
	}

	return data, syncDirectory(directory)
}

// withCueCategories sets or removes targets.CueCategories, indenting what it
// writes like the surrounding entries.
func withCueCategories(data []byte, categories map[string]string) []byte {
	targets, found := objectValue(data, "targets")
	if !found {
		if len(categories) == 0 {
			return data
		}

		return setObjectEntry(data, "targets", func(prefix string) []byte {
			return []byte("{\n" + prefix + "  " + string(encodeAudioValue(cueCategoriesKey)) + ": " +
				string(indentedJSON(categories, prefix+"  ")) + "\n" + prefix + "}")
		})
	}
	if len(categories) == 0 {
		targets = deleteObjectEntry(targets, cueCategoriesKey)
	} else {
		targets = setObjectEntry(targets, cueCategoriesKey, func(prefix string) []byte {
			return indentedJSON(categories, prefix)
		})
	}

	return setObjectEntry(data, "targets", func(string) []byte { return targets })
}

func objectValue(object []byte, key string) ([]byte, bool) {
	for _, entry := range orderedEntries(object) {
		if entry.key == key {
			return entry.value, true
		}
	}

	return nil, false
}

// setObjectEntry replaces one entry's value or appends the entry, keeping every
// other byte. value receives the indentation of the entry's line.
func setObjectEntry(object []byte, key string, value func(prefix string) []byte) []byte {
	entries := orderedEntries(object)
	for index, entry := range entries {
		if entry.key == key {
			return splice(object, entry.valueStart, entry.end, value(linePrefix(object, entries, index)))
		}
	}
	text := string(encodeAudioValue(key)) + ": "
	if len(entries) == 0 {
		return []byte("{" + text + string(value("")) + "}")
	}
	last := len(entries) - 1
	lead := entryLead(object, entries, last)
	insert := "," + lead + text + string(value(linePrefix(object, entries, last)))

	return splice(object, entries[last].end, entries[last].end, []byte(insert))
}

// deleteObjectEntry removes one entry and its separator, keeping every other byte.
func deleteObjectEntry(object []byte, key string) []byte {
	entries := orderedEntries(object)
	for index, entry := range entries {
		if entry.key != key {
			continue
		}
		switch {
		case len(entries) == 1:
			return []byte("{}")
		case index == 0:
			return splice(object, entry.start, entries[1].start, nil)
		default:
			return splice(object, entries[index-1].end, entry.end, nil)
		}
	}

	return object
}

func linePrefix(object []byte, entries []objectEntry, index int) string {
	lead := entryLead(object, entries, index)

	return lead[strings.LastIndexAny(lead, "\r\n")+1:]
}

func indentedJSON(value any, prefix string) []byte {
	data, err := json.MarshalIndent(value, prefix, "  ")
	if err != nil {
		panic(err) // Category maps hold strings only.
	}

	return data
}

func splice(data []byte, start, end int64, insert []byte) []byte {
	return append(append(bytes.Clone(data[:start]), insert...), data[end:]...)
}
