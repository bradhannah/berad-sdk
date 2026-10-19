package authoring

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// readRecordingBytes bounds both total input and work between cancellation
// checks. A synchronous Read already in progress must return before cancellation
// can be observed; no unowned worker is left reading after this function returns.
func readRecordingBytes(ctx context.Context, source io.Reader) ([]byte, error) {
	const blockBytes = 64 << 10
	buffer := make([]byte, blockBytes)
	limited := io.LimitReader(source, maxEditorFileBytes+1)
	var data []byte
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("read recording canceled: %w", err)
		}
		count, err := limited.Read(buffer)
		if cancelErr := ctx.Err(); cancelErr != nil {
			return nil, fmt.Errorf("read recording canceled: %w", cancelErr)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read recording bytes: %w", err)
		}
		data = append(data, buffer[:count]...)
		if errors.Is(err, io.EOF) {
			return data, nil
		}
		if count > 0 {
			emptyReads = 0
		} else {
			emptyReads++
			const maxEmptyReads = 100
			if emptyReads >= maxEmptyReads {
				return nil, fmt.Errorf("read recording bytes: %w", io.ErrNoProgress)
			}
		}
	}
}
