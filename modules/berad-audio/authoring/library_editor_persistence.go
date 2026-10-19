package authoring

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/gofrs/flock"
)

// PublicationState describes file publication separately from authored changes.
type PublicationState string

// File publication states do not describe whether the working document is dirty.
const (
	PublicationNotWritten  PublicationState = "not_published"
	PublicationUnchanged   PublicationState = "unchanged"
	PublicationComplete    PublicationState = "published"
	PublicationSyncPending PublicationState = "published_sync_pending"
)

// PublicationOutcome is a detached receipt for the latest Save/import operation.
// Complete means the supported flush operations succeeded, not a guarantee
// against every filesystem/device failure. Windows directory flush is unsupported.
type PublicationOutcome struct {
	Sequence               uint64
	State                  PublicationState
	Path                   string
	Import                 bool
	Created                bool
	Assigned               bool
	DirectorySyncSupported bool
}

// ImportReceipt describes a source file retained independently of Undo.
type ImportReceipt struct {
	Path       string
	Referenced bool
}

// ErrAudioPublicationSync preserves a published-but-unconfirmed outcome for retry.
var ErrAudioPublicationSync = errors.New("audio file state changed but durability is unconfirmed; retry Save")

// ErrAudioLibraryBusy reports a cooperating publisher holding the stable lock.
var ErrAudioLibraryBusy = errors.New("another audio editor is publishing this library; retry")

// These narrow publication boundaries support fault injection without replacing
// decoding, validation, or the filesystem with a second implementation.
type libraryPersistence struct {
	publish                func(string, string, []byte, os.FileMode, func(string) error) error
	replace                func(string, string) error
	link                   func(string, string) error
	syncDirectory          func(string) error
	syncFile               func(string) error
	lock                   func(string) (func() error, error)
	directorySyncSupported bool
}

func defaultLibraryPersistence() libraryPersistence {
	return libraryPersistence{publish: publishTemporaryFile, replace: os.Rename, link: os.Link,
		syncDirectory: syncDirectory, syncFile: syncImportedFile, lock: lockAudioLibrary,
		directorySyncSupported: runtime.GOOS != "windows"}
}

func lockAudioLibrary(manifest string) (func() error, error) {
	root, err := filepath.EvalSymlinks(filepath.Dir(manifest))
	if err != nil {
		return nil, fmt.Errorf("resolve audio library lock: %w", err)
	}
	identity := sha256.Sum256([]byte(filepath.Join(root, filepath.Base(manifest))))
	// Keep coordination files outside the embedded assets directory. The
	// stable lock inode must not be removed while editor processes are active.
	lock := flock.New(filepath.Join(filepath.Dir(root), fmt.Sprintf(".audio-library-%x.lock", identity)))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock audio library: %w", err)
	}
	if !locked {
		return nil, ErrAudioLibraryBusy
	}

	return func() error {
		if err := lock.Unlock(); err != nil {
			return fmt.Errorf("release audio library lock: %w", err)
		}
		return nil
	}, nil
}

func syncImportedFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // Validated content-qualified import destination.
	if err != nil {
		return fmt.Errorf("open import for flush: %w", err)
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return fmt.Errorf("flush imported file: %w", err)
	}

	return nil
}

// LastPublication returns publication facts even when an operation returns an error.
func (e *LibraryEditor) LastPublication() PublicationOutcome { return e.publication }

// NeedsSync reports unresolved durability work, independently of Dirty.
func (e *LibraryEditor) NeedsSync() bool { return len(e.pendingDirectories) != 0 }

// RetainedImports returns copied files from this session and their current use.
// Undo changes references only; it never deletes these files.
func (e *LibraryEditor) RetainedImports() []ImportReceipt {
	if len(e.retainedImports) == 0 {
		return nil
	}
	referenced := make(map[string]bool)
	for _, recording := range e.library.Recordings {
		if recording.File != nil {
			referenced[filepath.Join(e.assetRoot, filepath.FromSlash(*recording.File))] = true
		}
	}
	files := make([]ImportReceipt, 0, len(e.retainedImports))
	for _, path := range e.retainedImports {
		files = append(files, ImportReceipt{Path: path, Referenced: referenced[path]})
	}

	return files
}

func (e *LibraryEditor) beginPublication(path string, importing bool) {
	e.publication = PublicationOutcome{Sequence: e.publication.Sequence + 1, State: PublicationNotWritten, Path: path,
		Import: importing, Created: false, Assigned: false, DirectorySyncSupported: e.persistence.directorySyncSupported}
}

// MergePublication retains I/O facts from a completed detached operation while
// preserving newer authored state and its history. Rejecting a stale document
// must not discard knowledge of a manifest replacement or retained import.
func (e *LibraryEditor) MergePublication(completed *LibraryEditor) error {
	if completed == nil || completed.path != e.path || completed.assetRoot != e.assetRoot {
		return fmt.Errorf("%w: publication belongs to another library", ErrAudioLibrary)
	}
	if completed.publication.Sequence <= e.publication.Sequence {
		return nil
	}
	e.saved = bytes.Clone(completed.saved)
	e.baseline = cloneAudioValue(completed.baseline)
	e.publication = completed.publication
	e.pendingDirectories = slices.Clone(completed.pendingDirectories)
	e.manifestSyncPending = completed.manifestSyncPending
	for _, path := range completed.retainedImports {
		if !slices.Contains(e.retainedImports, path) {
			e.retainedImports = append(e.retainedImports, path)
		}
	}

	return nil
}

func (e *LibraryEditor) requireDirectorySync(path string) {
	if !slices.Contains(e.pendingDirectories, path) {
		e.pendingDirectories = append(e.pendingDirectories, path)
	}
}

func (e *LibraryEditor) flushPendingDirectories() error {
	var failures []error
	remaining := make([]string, 0, len(e.pendingDirectories))
	for _, path := range e.pendingDirectories {
		if err := e.persistence.syncDirectory(path); err != nil {
			remaining = append(remaining, path)
			failures = append(failures, fmt.Errorf("flush directory %s: %w", path, err))
		} else if path == filepath.Dir(e.path) {
			e.manifestSyncPending = false
		}
	}
	e.pendingDirectories = remaining
	if len(failures) != 0 {
		return fmt.Errorf("%w: %w", ErrAudioPublicationSync, errors.Join(failures...))
	}

	return nil
}
