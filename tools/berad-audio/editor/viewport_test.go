package editorui

import (
	"fmt"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestPageKeysMoveVisiblePageAndRetainSelectionRow(t *testing.T) {
	for _, keys := range [][2]tea.KeyType{
		{tea.KeyCtrlN, tea.KeyCtrlP}, {tea.KeyShiftDown, tea.KeyShiftUp}, {tea.KeyPgDown, tea.KeyPgUp},
	} {
		t.Run(fmt.Sprint(keys), func(t *testing.T) {
			m := fixture(t)
			m.cursor, m.browserStart = 2, 0
			height := m.listHeight()
			require.Greater(t, height, 2)
			m.key(tea.KeyMsg{Type: keys[0]})
			require.Equal(t, height+2, m.cursor)
			rows := m.browserLines(100)
			require.Contains(t, ansi.Strip(rows[browserHeadingRows]), m.keys[height], "first visible item must advance a whole page")
			require.Contains(t, ansi.Strip(rows[browserHeadingRows+2]), m.keys[height+2], "selection stays on the third visible row")
			m.key(tea.KeyMsg{Type: tea.KeyUp})
			require.Contains(t, ansi.Strip(m.browserLines(100)[browserHeadingRows]), m.keys[height], "ordinary Up retains the page")
			m.key(tea.KeyMsg{Type: keys[1]})
			require.Equal(t, 1, m.cursor)
			require.Contains(t, ansi.Strip(m.browserLines(100)[browserHeadingRows]), m.keys[0])
		})
	}
}

type pagingInputProbe struct {
	editor *model
	keys   []string
	rows   []string
}

func (p *pagingInputProbe) Init() tea.Cmd { return nil }
func (p *pagingInputProbe) View() string  { return "" }
func (p *pagingInputProbe) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		p.editor.key(key)
		p.keys = append(p.keys, key.String())
		p.rows = append(p.rows, ansi.Strip(p.editor.browserLines(100)[browserHeadingRows]))
		if len(p.keys) == 2 {
			return p, tea.Quit
		}
	}

	return p, nil
}

func TestControlPageKeysDecodeFromTerminalInput(t *testing.T) {
	m := fixture(t)
	m.cursor, m.browserStart = 0, 0
	probe := &pagingInputProbe{editor: m}
	program := tea.NewProgram(probe, tea.WithInput(strings.NewReader("\x0e\x10")),
		tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	_, err := program.Run()
	require.NoError(t, err)
	require.Equal(t, []string{"ctrl+n", "ctrl+p"}, probe.keys)
	require.Contains(t, probe.rows[0], m.keys[m.listHeight()])
	require.Contains(t, probe.rows[1], m.keys[0])
}

func TestPagingClampsAtEndsAndKeepsRenderedSelectionVisible(t *testing.T) {
	m := fixture(t)
	m.cursor, m.browserStart = 0, 0
	for range len(m.keys) {
		m.key(tea.KeyMsg{Type: tea.KeyShiftDown})
	}
	require.Equal(t, len(m.keys)-1, m.cursor)
	height := m.listHeight()
	require.Contains(t, ansi.Strip(m.browserLines(100)[browserHeadingRows+height-1]), m.keys[len(m.keys)-1])
	for range len(m.keys) {
		m.key(tea.KeyMsg{Type: tea.KeyShiftUp})
	}
	require.Zero(t, m.cursor)
	require.Contains(t, ansi.Strip(m.browserLines(100)[browserHeadingRows]), m.keys[0])
}

func TestFilePickerPagingMovesTheVisibleWindow(t *testing.T) {
	m := fixture(t)
	m.picker = &filePicker{directory: "/test"}
	height := m.listHeight()
	for index := range height * 3 {
		m.picker.entries = append(m.picker.entries, pickerEntry{name: fmt.Sprintf("clip_%03d.wav", index)})
	}
	m.pickerKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	require.Contains(t, ansi.Strip(m.fileLines(100)[browserHeadingRows]), fmt.Sprintf("clip_%03d.wav", height))
	m.pickerKey(tea.KeyMsg{Type: tea.KeyShiftUp})
	require.Contains(t, ansi.Strip(m.fileLines(100)[browserHeadingRows]), "clip_000.wav")
}
