package authoring

import (
	"fmt"
	"os"
	"path/filepath"
)

const emptyLibrary = `{
  "schemaVersion": 3,
  "recordings": {},
  "cues": {},
  "taxonomy": {
    "recordingFamilies": {},
    "cueProfiles": {},
    "profiles": {}
  },
  "routing": {}
}
`

const createdFileMode = 0o644

// CreateLibrary writes an empty library, with no recordings, cues,
// classifications or routing, at path. The file appears complete or not at
// all, and an existing file is never replaced.
func CreateLibrary(path string) error {
	return createFile(path, ".audio-library-*", []byte(emptyLibrary))
}

func createFile(path, pattern string, data []byte) error {
	directory := filepath.Dir(path)
	err := publishTemporaryFile(directory, pattern, data, createdFileMode, func(temporary string) error {
		return os.Link(temporary, path)
	})
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}

	return syncDirectory(directory)
}
