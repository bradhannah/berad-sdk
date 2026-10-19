package editorui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

type pickerEntry struct {
	name, path string
	directory  bool
}
type filePicker struct {
	directory string
	entries   []pickerEntry
	cursor    int
	start     int
}

func (m *model) openPicker() tea.Cmd {
	m.focus = focusBrowser
	m.picker = &filePicker{directory: filepath.Dir(m.doc.Path()), entries: nil, cursor: 0, start: 0}
	m.readDirectory(m.picker.directory)
	m.status = "Enter imports a file or opens a directory; p previews; / enters a path; Esc cancels."

	return nil
}

func (m *model) readDirectory(path string) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		m.report(fmt.Errorf("resolve picker directory: %w", err))

		return
	}
	path = absolute
	entries, err := os.ReadDir(path)
	if err != nil {
		m.report(fmt.Errorf("browse audio files: %w", err))

		return
	}
	rows := []pickerEntry{{name: "..", path: filepath.Dir(path), directory: true}}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() && !supportedRecording(entry.Name()) {
			continue
		}
		rows = append(rows, pickerEntry{
			name: entry.Name(), path: filepath.Join(path, entry.Name()), directory: entry.IsDir(),
		})
	}
	slices.SortStableFunc(rows[1:], func(left, right pickerEntry) int {
		if left.directory != right.directory {
			if left.directory {
				return -1
			}

			return 1
		}

		return strings.Compare(left.name, right.name)
	})
	m.invalidateSource(true)
	m.picker = &filePicker{directory: path, entries: rows, cursor: 0, start: 0}
}

func (m *model) pickerPath() string {
	if m.picker == nil || len(m.picker.entries) == 0 || m.picker.entries[m.picker.cursor].directory {
		return ""
	}

	return m.picker.entries[m.picker.cursor].path
}

func (m *model) pickerKey(key tea.KeyMsg) tea.Cmd {
	picker := m.picker
	previousPath := m.pickerPath()
	switch key.String() {
	case keyEscape:
		m.picker = nil

		return m.loadSelection()
	case "up":
		picker.cursor = max(0, picker.cursor-1)
	case keyDown:
		picker.cursor = min(len(picker.entries)-1, picker.cursor+1)
	case keyPageUp, keyShiftUp, keyPreviousPage:
		picker.start = listViewportStart(picker.start, picker.cursor, len(picker.entries), m.listHeight()) - m.listHeight()
		picker.cursor = max(0, picker.cursor-m.listHeight())
	case keyPageDown, keyShiftDown, keyNextPage:
		picker.start = listViewportStart(picker.start, picker.cursor, len(picker.entries), m.listHeight()) + m.listHeight()
		picker.cursor = max(0, min(len(picker.entries)-1, picker.cursor+m.listHeight()))
	case keyBackspace:
		m.readDirectory(filepath.Dir(picker.directory))

		return nil
	case "/":
		m.beginInput("path", picker.directory+string(filepath.Separator))
	case keyEnter:
		return m.openPickerEntry()
	case "p":
		return m.previewPickerEntry(false)
	case " ":
		return m.previewPickerEntry(true)
	}
	if m.pickerPath() != previousPath {
		m.invalidateSource(true)
		m.status = "File selection changed; p auditions this candidate, Enter imports it."
	}
	picker.start = listViewportStart(picker.start, picker.cursor, len(picker.entries), m.listHeight())

	return nil
}

func (m *model) openPickerEntry() tea.Cmd {
	if len(m.picker.entries) == 0 {
		return nil
	}
	entry := m.picker.entries[m.picker.cursor]
	if entry.directory {
		m.readDirectory(entry.path)

		return nil
	}

	return m.importFile(entry.path)
}

func (m *model) previewPickerEntry(toggle bool) tea.Cmd {
	if toggle && m.clip != nil && m.clip.Path() == m.pickerPath() &&
		m.preview.Active() && (m.preview.Playing() || m.previewPaused) {
		m.togglePreview()

		return nil
	}
	if len(m.picker.entries) == 0 || m.picker.entries[m.picker.cursor].directory {
		return nil
	}
	m.fullSource = true

	return m.loadPath(m.picker.entries[m.picker.cursor].path, true)
}

func (m *model) enterPickerPath(path string) tea.Cmd {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			m.report(err)

			return nil
		}
		path = filepath.Join(home, path[2:])
	}
	info, err := os.Stat(path)
	if err != nil {
		m.report(err)

		return nil
	}
	if info.IsDir() {
		m.readDirectory(path)

		return nil
	}

	return m.importFile(path)
}

func (m *model) importFile(path string) tea.Cmd {
	absolute, err := filepath.Abs(path)
	if err != nil {
		m.report(fmt.Errorf("resolve import source: %w", err))

		return nil
	}
	path = absolute
	recordingID := m.recordingID()
	var auditioned *audio.RecordingAudio
	if m.picker != nil && m.clip != nil && m.clip.Path() == path {
		auditioned = m.clip
	}
	m.busy, m.picker = true, nil
	if m.cancelLoad != nil {
		m.cancelLoad()
	}
	m.generation++
	m.clip, m.loading = nil, false
	m.status = "Validating and importing recording…"
	m.clearSoundscape()

	return m.documentOperation(true,
		"File assigned; trim reset, manual gain retained or AUTO recalculated. Ctrl+s saves; u undoes.",
		func(document *audio.LibraryEditor) error {
			if auditioned != nil {
				return document.ImportRecordingSource(recordingID, auditioned)
			}

			return document.ImportRecording(recordingID, path)
		})
}
