package authoring

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

// Publication completes a flushed temporary file before making it visible. The
// caller records namespace/durability facts in its receipt before a later error.
func publishTemporaryFile(root, pattern string, data []byte, mode os.FileMode, publish func(string) error) (err error) {
	temporary, err := os.CreateTemp(root, pattern)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	path := temporary.Name()
	defer func() {
		if cleanup := os.Remove(path); cleanup != nil && !errors.Is(cleanup, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary file after publication attempt: %w", cleanup))
		}
	}()
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if failure := errors.Join(err, temporary.Close()); failure != nil {
		return fmt.Errorf("prepare publication: %w", failure)
	}

	return publish(path)
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	//nolint:gosec // Explicit validated publication directory; this operation flushes its namespace.
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	if failure := errors.Join(directory.Sync(), directory.Close()); failure != nil {
		return fmt.Errorf("flush publication directory: %w", failure)
	}

	return nil
}
