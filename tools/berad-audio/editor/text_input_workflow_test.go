package editorui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestNoteCanApplyWithOrdinaryTabAndEnter(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, 'N')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("  first line")})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second line  ")})
	require.Contains(t, ansi.Strip(m.View()), "Apply")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyTab})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Empty(t, m.field)
	require.Equal(t, "  first line\nsecond line  ", m.doc.Recording(m.recordingID()).Note)
}

func TestTypedControlKeyNamesRemainLiteralText(t *testing.T) {
	for _, name := range []string{"enter", "esc", "tab", "ctrl+s", "ctrl+x", "ctrl+c"} {
		t.Run(name, func(t *testing.T) {
			m := fixture(t)
			dispatchRune(m, 'N')
			dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlU})
			dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)})
			require.Equal(t, name, m.input)
			require.Equal(t, fieldNote, m.field)
			require.False(t, m.confirmQuit)
			require.False(t, m.doc.Dirty())
		})
	}
}

func TestInlineTextEditsAtCaretWithoutChangingNumericValue(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, 's')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("100")})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, "100", m.input, "typing mode uses arrows for the caret, not numeric stepping")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	require.Equal(t, "120", m.input)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, int64(120), m.window().StartMs)
}

func TestCancelInvalidFieldClearsItsErrorWithoutAuthoring(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, 'g')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("invalid")})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.statusIsError)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Empty(t, m.field)
	require.False(t, m.statusIsError)
	require.False(t, m.doc.Dirty())
}

func TestFormPasteUsesTextAndEscapeRestoresFieldDraft(t *testing.T) {
	m := fixture(t)
	finish(m, m.openNewAmbientForm())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Quiet cave"), Paste: true})
	require.Equal(t, "Quiet cave", m.form.fields[newNameField].value)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyLeft})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyDelete})
	require.Equal(t, "Quiet cav", m.form.fields[newNameField].value)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Empty(t, m.form.fields[newNameField].value)
	require.Empty(t, m.form.fields[newIDField].value)
	require.False(t, m.form.editing)
	require.False(t, m.doc.Dirty())
}

func TestRoutingSearchPasteIsAReversibleDraft(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("216")
	finish(m, m.openRoutingAssignment("cue"))
	dispatchRune(m, '/')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Fountain"), Paste: true})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, "Fountain", m.routing.menu.search)
	dispatchRune(m, '/')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Fire")})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, "Fountain", m.routing.menu.search)
}

func TestAssignmentChooserDoesNotDispatchHiddenEditingShortcuts(t *testing.T) {
	for _, key := range []rune{'g', 's', 'e', 'D', 'N', 'C', 'A', 'u', '1', 'a', '+'} {
		t.Run(string(key), func(t *testing.T) {
			m := fixture(t)
			selectHealMoment(t, m)
			dispatchRune(m, 'f')
			revision := m.doc.Revision()
			dispatchRune(m, key)
			require.True(t, m.assigning)
			require.Empty(t, m.field)
			require.Nil(t, m.form)
			require.Nil(t, m.duplicate)
			require.Equal(t, revision, m.doc.Revision())
			require.Contains(t, ansi.Strip(m.View()), "CHOOSE RECORDING")
		})
	}
}

func TestStopAllWorksDuringTextEntryWithoutChangingDraft(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, 'p')
	dispatchRune(m, 'N')
	before := m.input
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlX})
	require.False(t, m.preview.Active())
	require.Equal(t, fieldNote, m.field)
	require.Equal(t, before, m.input)
}

func TestFootstepSequenceHasReachableApplyAndCancelActions(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("68")
	before := m.doc.Routing().Surfaces["stone"]
	dispatchRune(m, 'S')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlDown})
	require.NotEqual(t, before, m.form.sequence)
	require.Contains(t, ansi.Strip(m.View()), "Apply changes")
	for range len(m.form.sequence) - m.form.cursor {
		dispatchKey(m, tea.KeyMsg{Type: tea.KeyTab})
	}
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.form)
	require.NotEqual(t, before, m.doc.Routing().Surfaces["stone"])
	dispatchRune(m, 'u')
	require.Equal(t, before, m.doc.Routing().Surfaces["stone"])
}

func TestNestedRecordingPickerRestoresParentSelectionAndSearch(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("216")
	finish(m, m.openRoutingAssignment("cue"))
	parent := m.routing.menu
	parent.search = "Fountain"
	parent.cursor = 0
	finish(m, m.loadSelection())
	dispatchRune(m, 'r')
	require.Same(t, parent, m.routing.menu.parent)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Same(t, parent, m.routing.menu)
	require.Equal(t, "Fountain", parent.search)
	require.Zero(t, parent.cursor)
}

func TestRoutingWorkspacesRetainIndependentFiltersAndScopes(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceLocations))
	m.routing.scope, m.routing.filter = 2, 1
	m.refreshKeys()
	finish(m, m.selectWorkspace(workspaceTiles))
	require.Zero(t, m.routing.filter)
	m.routing.filter = 3
	m.refreshKeys()
	finish(m, m.selectWorkspace(workspaceLocations))
	require.Equal(t, 2, m.routing.scope)
	require.Equal(t, 1, m.routing.filter)
	finish(m, m.selectWorkspace(workspaceTiles))
	require.Equal(t, 3, m.routing.filter)
}

func TestRoutingRefreshUsesGlobalNonEditingF5(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	generation := m.scanGeneration
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyF5})
	require.Greater(t, m.scanGeneration, generation)
	dispatchRune(m, '/')
	generation = m.scanGeneration
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyF5})
	require.Equal(t, generation, m.scanGeneration, "refresh cannot invalidate the owner of an open text draft")
}

func TestInputOwnerDrivesForegroundAndHints(t *testing.T) {
	m := fixture(t)
	finish(m, m.openNewAmbientForm())
	require.Equal(t, ownerForm, m.inputOwner())
	dispatchRune(m, 'v')
	require.Equal(t, ownerVolumes, m.inputOwner())
	dialog, visible := m.dialogContent(80)
	require.True(t, visible)
	require.Contains(t, dialog.title, "PREVIEW VOLUME")
	require.Contains(t, ansi.Strip(strings.Join(m.footerLines(), "\n")), "Game Mix/Reference")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, ownerForm, m.inputOwner())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	dispatchRune(m, 'v')
	require.Equal(t, "v", m.form.fields[0].value, "typing cannot open a hidden volume owner")
	require.Equal(t, ownerForm, m.inputOwner())
	require.Contains(t, ansi.Strip(strings.Join(m.footerLines(), "\n")), "Caret")
}

type terminalWorkflowProbe struct {
	editor *model
	pasted bool
}

func (probe *terminalWorkflowProbe) Init() tea.Cmd { return nil }
func (probe *terminalWorkflowProbe) View() string  { return "" }
func (probe *terminalWorkflowProbe) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		if key.Type == tea.KeyCtrlQ {
			return probe, tea.Quit
		}
		probe.pasted = probe.pasted || key.Paste
	}
	_, command := probe.editor.Update(message)
	return probe, command
}

func runTerminalWorkflow(t *testing.T, editor *model, input string) *terminalWorkflowProbe {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	probe := &terminalWorkflowProbe{editor: editor}
	program := tea.NewProgram(probe, tea.WithContext(ctx), tea.WithInput(strings.NewReader(input)),
		tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	_, err := program.Run()
	require.NoError(t, err)
	return probe
}

func TestTerminalBytesPasteAndApplyMultilineNote(t *testing.T) {
	m := fixture(t)
	probe := runTerminalWorkflow(t, m, "N\x15\x1b[200~  α👩‍🔬\r\nsecond  \x1b[201~\t\r\x11")
	require.True(t, probe.pasted, "exercise the pinned terminal decoder, not a fabricated key name")
	require.Empty(t, m.field)
	require.Equal(t, "  α👩‍🔬\nsecond  ", m.doc.Recording(m.recordingID()).Note)
}

func TestTerminalBytesMoveCaretAndApplyNumericField(t *testing.T) {
	m := fixture(t)
	runTerminalWorkflow(t, m, "s\x15100\x1b[D\x7f2\r\x11")
	require.Empty(t, m.field)
	require.Equal(t, int64(120), m.window().StartMs)
}

func TestTerminalBytesApplyFootstepsWithoutModifiedEnter(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("68")
	before := m.doc.Routing().Surfaces["stone"]
	dispatchRune(m, 'S')
	runTerminalWorkflow(t, m, "\x1b[3~"+strings.Repeat("\t", len(before)-1)+"\r\x11")
	require.Nil(t, m.form)
	require.Equal(t, before[1:], m.doc.Routing().Surfaces["stone"])
}

func TestBusyQuitWaitsForPublicationAndDoesNotHideFailure(t *testing.T) {
	for _, failure := range []error{nil, errors.New("publication failed")} {
		m := fixture(t)
		operation := m.documentOperation(false, "saved", func(*audio.LibraryEditor) error { return failure })
		_, quit := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		require.Nil(t, quit)
		require.True(t, m.busy)
		require.True(t, m.quitAfterOperation)
		_, done := m.Update(operation())
		require.False(t, m.busy)
		require.False(t, m.quitAfterOperation)
		if failure == nil {
			require.NotNil(t, done)
			require.IsType(t, tea.QuitMsg{}, done())
		} else {
			require.Nil(t, done)
			require.True(t, m.statusIsError)
			require.Contains(t, m.status, failure.Error())
		}
	}
}
