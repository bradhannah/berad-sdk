package authoring

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportRecording validates and copies a file into the library before assigning
// it. Existing repository files are referenced in place. Outside files receive
// content-qualified names, so replacing a source never overwrites shared audio.
// Imported files remain on disk after Undo; Git exposes them for review.
func (e *LibraryEditor) ImportRecording(recordingID, path string) error {
	e.beginPublication(path, true)
	clip, err := LoadRecording(path, e.decode)
	if err != nil {
		return err
	}

	return e.ImportRecordingSource(recordingID, clip)
}

// ImportRecordingSource imports exactly the auditioned snapshot, rejecting a
// changed source rather than silently choosing new bytes at the same pathname.
func (e *LibraryEditor) ImportRecordingSource(recordingID string, clip *RecordingAudio) error {
	if clip == nil {
		return fmt.Errorf("%w: select a source recording", ErrAudioLibrary)
	}
	e.beginPublication(clip.path, true)
	unlock, err := e.persistence.lock(e.path)
	if err != nil {
		return err
	}
	operation := func() error {
		if err := e.checkExternalChange(); err != nil {
			return err
		}
		if err := e.flushPendingDirectories(); err != nil {
			return err
		}

		return e.importSourceLocked(recordingID, clip)
	}

	return errors.Join(operation(), unlock())
}

func (e *LibraryEditor) importSourceLocked(recordingID string, clip *RecordingAudio) error {
	previous, exists := e.library.Recordings[recordingID]
	if !exists {
		return fmt.Errorf("%w: select or create a recording first", ErrAudioLibrary)
	}
	relative, err := filepath.Rel(e.assetRoot, clip.path)
	if err != nil {
		return fmt.Errorf("resolve relative recording path: %w", err)
	}
	relative = filepath.ToSlash(relative)
	copySource := !e.importPathAllowed(relative)
	if copySource {
		relative, err = e.chooseImportDestination(recordingID, previous, clip)
		if err != nil {
			return err
		}
	}
	if !validAssetPath(relative) {
		return fmt.Errorf("%w: invalid import destination", ErrLibrary)
	}
	e.publication.Path = filepath.Join(e.assetRoot, filepath.FromSlash(relative))
	if err := clip.VerifySource(context.Background()); err != nil {
		return err
	}
	if previous.File != nil && *previous.File == relative {
		e.publication.State, e.publication.Assigned = PublicationUnchanged, true

		return nil
	}
	replacement, err := e.importReplacement(recordingID, previous, relative, clip)
	if err != nil {
		return err
	}
	next := cloneAudioValue(e.library)
	next.Recordings[recordingID] = replacement
	if _, err := parseLibraryDocument(encodeAudioValue(next), e.validator); err != nil {
		return err // All authored validation precedes any copied-file publication.
	}
	if copySource {
		if err := e.publishImport(relative, clip); err != nil {
			return err
		}
	} else {
		e.publication.State = PublicationUnchanged
	}
	if err := e.replace(next); err != nil {
		return err
	}
	e.publication.Assigned = true

	return nil
}

func (e *LibraryEditor) chooseImportDestination(
	recordingID string, previous Recording, clip *RecordingAudio,
) (string, error) {
	if e.importDestination != nil {
		path, err := e.importDestination(recordingID, cloneAudioValue(previous), clip.Path(), clip.Fingerprint())
		if err != nil {
			return "", fmt.Errorf("choose import destination: %w", err)
		}

		return path, nil
	}
	relative := importRelativePath(recordingID, clip)
	if e.importDirectory != "" {
		relative = e.importDirectory + "/" + filepath.Base(relative)
	}

	return relative, nil
}

func (e *LibraryEditor) importReplacement(
	recordingID string, previous Recording, relative string, clip *RecordingAudio,
) (Recording, error) {
	provenance := unknownRecordingProvenance()
	provenance.OriginalFilename = filepath.Base(clip.path)
	for _, known := range e.library.Recordings {
		if known.File != nil && *known.File == relative {
			provenance = cloneAudioValue(known.Provenance)

			break
		}
	}

	replacement := previous
	replacement.File, replacement.PlaybackWindow = &relative, nil
	replacement.Provenance, replacement.Note = provenance, ""
	// Manual overrides belong to the recording identity and survive replacement.
	// Auto-managed replacements are measured before publishing the single edit.
	if replacement.CalibrationMode() == CalibrationAuto {
		options := e.calibrationOptions(e.RecordingFamily(recordingID), replacement.CalibrationTargetDB)
		suggestion, err := clip.SuggestCalibration(nil, options)
		if err != nil {
			return Recording{}, err
		}
		replacement.CalibrationGainDB = &suggestion.GainDB
	}

	return replacement, nil
}

func importRelativePath(recordingID string, clip *RecordingAudio) string {
	directory := "imported"
	hash := sha256.Sum256(clip.data)
	name := fmt.Sprintf("%s_%x%s", recordingID, hash[:6], strings.ToLower(filepath.Ext(clip.path)))

	return directory + "/" + name
}

func (e *LibraryEditor) publishImport(relative string, clip *RecordingAudio) error {
	path := filepath.Join(e.assetRoot, filepath.FromSlash(relative))
	const directoryMode = 0o750
	const recordingMode = 0o644
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		return fmt.Errorf("create recording directory: %w", err)
	}
	// Flush every namespace level potentially created by MkdirAll, up to the
	// already-existing asset root containing this manifest.
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		e.requireDirectorySync(directory)
		if directory == e.assetRoot {
			break
		}
	}
	if err := e.flushPendingDirectories(); err != nil {
		return err
	}
	err := e.persistence.publish(filepath.Dir(path), ".audio-import-*", clip.data, recordingMode,
		func(temporary string) error {
			if err := e.persistence.link(temporary, path); err != nil {
				if !os.IsExist(err) {
					return fmt.Errorf("publish imported recording: %w", err)
				}
				if _, err := matchImportedFile(path, relative, clip.fingerprint); err != nil {
					return err
				}
				if err := e.persistence.syncFile(path); err != nil {
					return err
				}
			} else {
				e.publication.Created = true
				e.retainedImports = append(e.retainedImports, path)
			}
			e.publication.State = PublicationSyncPending
			e.requireDirectorySync(filepath.Dir(path))

			return nil
		})
	if err != nil {
		return fmt.Errorf("import recording: %w", err)
	}
	if err := e.flushPendingDirectories(); err != nil {
		return err
	}
	e.publication.State = PublicationComplete

	return nil
}

func matchImportedFile(path, relative string, expected [sha256.Size]byte) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Validated library import destination.
	if err != nil {
		return "", fmt.Errorf("read existing import: %w", err)
	}
	if sha256.Sum256(data) != expected {
		return "", fmt.Errorf("%w: import destination already has different contents", ErrAudioLibrary)
	}

	return relative, nil
}
