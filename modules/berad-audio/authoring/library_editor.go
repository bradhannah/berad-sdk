package authoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

// Recording is the production recording schema, shared with the editor.
type Recording = libraryRecording

// Cue is the production cue schema, shared with the editor.
type Cue = libraryCue

const editorHistoryLimit = 100

// ErrDocumentStale rejects a detached edit based on an older document revision.
var ErrDocumentStale = errors.New("authoring document revision changed")

// LibraryEditor owns a disk-backed working document. Its caller serializes edits
// and saves; detached file decoding can run in background commands.
type LibraryEditor struct {
	assetRoot           string
	validator           LibraryValidator
	importDirectory     string
	importPathAllowed   func(string) bool
	targets             AuthoringTargets
	path                string
	files               fs.FS
	library             *audioLibrary
	baseline            *audioLibrary
	saved               []byte
	undo                []*audioLibrary
	redo                []*audioLibrary
	revision            uint64
	persistence         libraryPersistence
	publication         PublicationOutcome
	pendingDirectories  []string
	manifestSyncPending bool
	retainedImports     []string
	decode              audio.Decoder
	calibrationPolicy   func(string) CalibrationOptions
	importDestination   func(string, Recording, string, string) (string, error)
}

// EditorOptions binds disk authoring to an explicit asset root and consumer
// validation policy. An empty root uses the manifest directory.
type EditorOptions struct {
	AssetRoot       string
	Validate        LibraryValidator
	ImportDirectory string
	Targets         AuthoringTargets
	Decode          audio.Decoder
	Calibration     func(string) CalibrationOptions
	// Optional consumer path policy; nil accepts any valid relative asset path.
	AcceptExistingPath func(string) bool
	// ImportDestination receives detached metadata and the source fingerprint.
	// It must return a valid relative destination; no files have been published yet.
	ImportDestination func(string, Recording, string, string) (string, error)
}

// DefaultEditorOptions supplies generic active-RMS calibration and relative
// imported-file placement. Consumers override policy explicitly; no game family,
// target registry or embedded asset root is inferred.
func DefaultEditorOptions(decode audio.Decoder) EditorOptions {
	var options EditorOptions
	options.Decode = decode
	options.Calibration = func(string) CalibrationOptions {
		return DefaultCalibrationOptions()
	}

	return options
}

// OpenLibraryProject opens the shared authoring engine for an independent project.
// Relative asset roots resolve against the manifest, never the process directory.
func OpenLibraryProject(path string, options EditorOptions) (*LibraryEditor, error) {
	if options.Decode == nil || options.Calibration == nil {
		return nil, fmt.Errorf("%w: decoder and calibration policy are required", ErrLibrary)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve audio library: %w", err)
	}
	data, err := os.ReadFile(absolute) //nolint:gosec // Explicit developer-selected authoring file.
	if err != nil {
		return nil, fmt.Errorf("open audio library: %w", err)
	}
	root, err := resolveAssetRoot(absolute, options.AssetRoot)
	if err != nil {
		return nil, err
	}
	if options.ImportDirectory != "" &&
		(!fs.ValidPath(options.ImportDirectory) || strings.Contains(options.ImportDirectory, "\\")) {
		return nil, fmt.Errorf("%w: invalid import directory", ErrAudioLibrary)
	}
	files := os.DirFS(root)
	library, err := parseLibraryDocument(data, options.Validate)
	if err != nil {
		return nil, err
	}
	if err := validateAuthoringTargets(options.Targets); err != nil {
		return nil, err
	}

	editor := newLibraryEditor(absolute, root, options)
	editor.files, editor.library = files, library
	editor.baseline, editor.saved = library, data
	editor.beginPublication(absolute, false)
	editor.publication.State = PublicationUnchanged

	return editor, nil
}

func validateAuthoringTargets(targets AuthoringTargets) error {
	for _, category := range targets.CueCategories {
		if category != catalogMusic && category != catalogSFX && category != routingAmbientKey {
			return fmt.Errorf("%w: unknown authoring cue category %s", ErrAudioLibrary, category)
		}
	}
	for group := range targets.Groups {
		if !schemaThreeAssignmentGroup(group) {
			return fmt.Errorf("%w: unknown schema-3 target group %q", ErrLibrary, group)
		}
	}

	return nil
}

func resolveAssetRoot(manifest, root string) (string, error) {
	if root == "" {
		root = filepath.Dir(manifest)
	} else if !filepath.IsAbs(root) {
		root = filepath.Join(filepath.Dir(manifest), root)
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("open audio asset root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: asset root is not a directory", ErrAudioLibrary)
	}

	return root, nil
}

func newLibraryEditor(absolute, root string, options EditorOptions) *LibraryEditor {
	editor := new(LibraryEditor)
	editor.assetRoot, editor.validator, editor.importDirectory = root, options.Validate, options.ImportDirectory
	editor.decode, editor.calibrationPolicy = options.Decode, options.Calibration
	editor.importPathAllowed = validAssetPath
	if options.AcceptExistingPath != nil {
		editor.importPathAllowed = options.AcceptExistingPath
	}
	editor.importDestination = options.ImportDestination
	editor.targets = cloneAudioValue(options.Targets)
	editor.path = absolute
	editor.persistence = defaultLibraryPersistence()

	return editor
}

// Path returns the absolute manifest filename.
func (e *LibraryEditor) Path() string { return e.path }

// RuntimeLibrary freezes the current validated authoring state for isolated
// playback. Later edits, Save, Undo and external file changes cannot retarget it.
func (e *LibraryEditor) RuntimeLibrary(maxEncodedBytes int64) (*audio.Library, error) {
	definition, err := PlaybackDefinition(e.Document())
	if err != nil {
		return nil, err
	}

	library, err := audio.NewLibrary(definition, e.files, maxEncodedBytes)
	if err != nil {
		return nil, fmt.Errorf("freeze authored library: %w", err)
	}

	return library, nil
}

// Document returns a detached complete metadata view.
func (e *LibraryEditor) Document() LibraryDocument { return *cloneAudioValue(e.library) }

// ApplyDocument commits a detached composite edit as one history entry. A source
// snapshot is not reloaded and a stale caller revision cannot replace newer work.
func (e *LibraryEditor) ApplyDocument(expectedRevision uint64, document LibraryDocument) error {
	if e.revision != expectedRevision {
		return ErrDocumentStale
	}

	return e.replace(&document)
}

// LoadRecording applies this project's decoder to a selected source or import.
// Paths are explicit; callers normally obtain assigned paths from RecordingPath.
func (e *LibraryEditor) LoadRecording(ctx context.Context, path string) (*RecordingAudio, error) {
	return LoadRecordingContext(ctx, path, e.decode)
}

// Revision identifies authored state, including Undo/Redo. Detached work must
// compare its starting revision before replacing the current working document.
func (e *LibraryEditor) Revision() uint64 { return e.revision }

// RecordingIDs returns sorted stable recording identities for browsing.
func (e *LibraryEditor) RecordingIDs() []string { return sortedKeys(e.library.Recordings) }

// CueIDs returns sorted stable production cue identities for browsing.
func (e *LibraryEditor) CueIDs() []string { return sortedKeys(e.library.Cues) }

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

// Recording returns a detached authoring record.
func (e *LibraryEditor) Recording(id string) Recording {
	return cloneAudioValue(e.library.Recordings[id])
}

// Cue returns a copy of one authored cue.
func (e *LibraryEditor) Cue(id string) Cue { return cloneAudioValue(e.library.Cues[id]) }

// Usages lists every production cue affected by editing a shared recording.
func (e *LibraryEditor) Usages(recordingID string) []string {
	var ids []string
	for id, cue := range e.library.Cues {
		if slices.Contains(cue.Recordings(), recordingID) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	return ids
}

// RecordingPath returns a source file path, or an explicit missing-slot error.
func (e *LibraryEditor) RecordingPath(id string) (string, error) {
	recording, exists := e.library.Recordings[id]
	if !exists || recording.File == nil {
		return "", fmt.Errorf("%w: %s has no audio file", ErrAudioLibrary, id)
	}

	return filepath.Join(e.assetRoot, filepath.FromSlash(*recording.File)), nil
}

// SetRecording validates a replacement before changing the document or history.
func (e *LibraryEditor) SetRecording(recordingID string, recording Recording) error {
	if recording.File != nil && recording.PlaybackWindow != nil {
		if err := e.validateWindow(recording); err != nil {
			return err
		}
	}
	next := cloneAudioValue(e.library)
	next.Recordings[recordingID] = recording

	return e.replace(next)
}

// SetRecordingLicense changes provenance only, including for an empty or missing
// audio file. Metadata authoring does not decode, trim or recalibrate the source.
func (e *LibraryEditor) SetRecordingLicense(recordingID, license string) error {
	if _, exists := e.library.Recordings[recordingID]; !exists {
		return fmt.Errorf("%w: unknown recording %s", ErrAudioLibrary, recordingID)
	}
	next := cloneAudioValue(e.library)
	recording := next.Recordings[recordingID]
	recording.Provenance.License = strings.TrimSpace(license)
	next.Recordings[recordingID] = recording

	return e.replace(next)
}

// AddRecording creates an unreviewed, intentionally empty slot for a later import.
func (e *LibraryEditor) AddRecording(recordingID string) error {
	if _, exists := e.library.Recordings[recordingID]; exists {
		return fmt.Errorf("%w: recording %s already exists", ErrAudioLibrary, recordingID)
	}

	next := cloneAudioValue(e.library)
	next.Recordings[recordingID] = Recording{File: nil, CalibrationGainDB: nil, PlaybackWindow: nil,
		CalibrationPolicy: "", CalibrationTargetDB: nil,
		Provenance: unknownRecordingProvenance(), Note: "", SearchTerms: nil}
	const family = "unreviewed"
	next.Taxonomy.RecordingFamilies[family] = append(next.Taxonomy.RecordingFamilies[family], recordingID)
	slices.Sort(next.Taxonomy.RecordingFamilies[family])

	return e.replace(next)
}

// Clone creates a detached working copy for a blocking save/import command.
func (e *LibraryEditor) Clone() *LibraryEditor {
	cloned := *e
	cloned.library = cloneAudioValue(e.library)
	cloned.saved = bytes.Clone(e.saved)
	cloned.undo = slices.Clone(e.undo)
	cloned.redo = slices.Clone(e.redo)
	cloned.pendingDirectories = slices.Clone(e.pendingDirectories)
	cloned.retainedImports = slices.Clone(e.retainedImports)

	return &cloned
}

// SetCue validates an assignment or gain before changing the working document.
func (e *LibraryEditor) SetCue(id string, cue Cue) error {
	next := cloneAudioValue(e.library)
	next.Cues[id] = cue

	return e.replace(next)
}

// RecordingFamily describes the actual acoustic content independently of cue uses.
func (e *LibraryEditor) RecordingFamily(id string) string {
	return e.library.Taxonomy.recordingFamily(id)
}

// CueProfile names the reusable semantic classification assigned to a moment.
func (e *LibraryEditor) CueProfile(id string) string { return e.library.Taxonomy.cueProfile(id) }

// CueClassification returns a detached description of the intended moment.
func (e *LibraryEditor) CueClassification(id string) CueClassification {
	return cloneAudioValue(e.library.Taxonomy.Profiles[e.CueProfile(id)])
}

// ClassificationProfiles lists the valid authored profile names.
func (e *LibraryEditor) ClassificationProfiles() []string {
	return sortedKeys(e.library.Taxonomy.Profiles)
}

// SetRecordingFamily changes one recording's acoustic description atomically.
func (e *LibraryEditor) SetRecordingFamily(recordingID, family string) error {
	if _, exists := e.library.Recordings[recordingID]; !exists || strings.TrimSpace(family) == "" {
		return fmt.Errorf("%w: unknown recording or acoustic family", ErrAudioLibrary)
	}
	if e.RecordingFamily(recordingID) == family {
		return nil
	}
	next := cloneAudioValue(e.library)
	moveTaxonomyID(next.Taxonomy.RecordingFamilies, recordingID, family)
	recording := next.Recordings[recordingID]
	if recording.File != nil && recording.CalibrationMode() == CalibrationAuto {
		path, err := e.RecordingPath(recordingID)
		if err != nil {
			return err
		}
		clip, err := LoadRecording(path, e.decode)
		if err != nil {
			return err
		}
		options := e.calibrationOptions(family, recording.CalibrationTargetDB)
		suggestion, err := clip.SuggestCalibration(recording.PlaybackWindow, options)
		if err != nil {
			return fmt.Errorf("calibrate changed recording family: %w", err)
		}
		recording.CalibrationGainDB = &suggestion.GainDB
		next.Recordings[recordingID] = recording
	}

	return e.replace(next)
}

// SetCueProfile changes one moment's semantic intent atomically.
func (e *LibraryEditor) SetCueProfile(cueID, profile string) error {
	if _, exists := e.library.Cues[cueID]; !exists {
		return fmt.Errorf("%w: unknown cue %s", ErrAudioLibrary, cueID)
	}
	if _, exists := e.library.Taxonomy.Profiles[profile]; !exists {
		return fmt.Errorf("%w: unknown classification profile %s", ErrAudioLibrary, profile)
	}
	if e.CueProfile(cueID) == profile {
		return nil
	}
	next := cloneAudioValue(e.library)
	moveTaxonomyID(next.Taxonomy.CueProfiles, cueID, profile)

	return e.replace(next)
}

func moveTaxonomyID(groups map[string][]string, identity, target string) {
	removeTaxonomyID(groups, identity)
	groups[target] = append(groups[target], identity)
	slices.Sort(groups[target])
}

// removeTaxonomyID drops an identity from its group, and the group once empty.
func removeTaxonomyID(groups map[string][]string, identity string) {
	for group, ids := range groups {
		groups[group] = slices.DeleteFunc(ids, func(candidate string) bool { return candidate == identity })
		if len(groups[group]) == 0 {
			delete(groups, group)
		}
	}
}

func (e *LibraryEditor) replace(next *audioLibrary) error {
	encoded, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("invalid audio edit: %w", err)
	}
	if _, err := parseLibraryDocument(encoded, e.validator); err != nil {
		return err
	}
	if bytes.Equal(encoded, encodeAudioValue(e.library)) {
		return nil
	}
	e.undo = append(e.undo, e.library)
	if len(e.undo) > editorHistoryLimit {
		e.undo = e.undo[1:]
	}
	e.library, e.redo = cloneAudioValue(next), nil
	e.revision++

	return nil
}

// Undo restores the preceding document, without deleting imported source files.
func (e *LibraryEditor) Undo() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.redo = append(e.redo, e.library)
	e.library = e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.revision++

	return true
}

// Redo reapplies a document edit undone in this session.
func (e *LibraryEditor) Redo() bool {
	if len(e.redo) == 0 {
		return false
	}
	e.undo = append(e.undo, e.library)
	e.library = e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.revision++

	return true
}

// Dirty compares authored values with the most recent saved document.
func (e *LibraryEditor) Dirty() bool {
	return !bytes.Equal(encodeAudioValue(e.baseline), encodeAudioValue(e.library))
}

// RecordingChanged reports edits to this recording since the last Save.
func (e *LibraryEditor) RecordingChanged(id string) bool {
	return e.baseline.Taxonomy.recordingFamily(id) != e.RecordingFamily(id) ||
		!bytes.Equal(encodeAudioValue(e.baseline.Recordings[id]), encodeAudioValue(e.library.Recordings[id]))
}

// CueChanged reports authored cue edits, independently of its recording's state.
func (e *LibraryEditor) CueChanged(id string) bool {
	return e.baseline.Taxonomy.cueProfile(id) != e.CueProfile(id) ||
		!bytes.Equal(encodeAudioValue(e.baseline.Cues[id]), encodeAudioValue(e.library.Cues[id]))
}

func encodeAudioValue(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err) // Internal snapshots contain only validated JSON schema values.
	}

	return data
}

//nolint:ireturn // The helper preserves the concrete input schema type while detaching JSON-backed pointers.
func cloneAudioValue[T any](value T) T {
	var clone T
	if err := json.Unmarshal(encodeAudioValue(value), &clone); err != nil {
		panic(err)
	}

	return clone
}
