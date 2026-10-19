// Package pcm supplies shared seekable window/loop mechanics for playback and authoring.
package pcm

import (
	"errors"
	"fmt"
	"io"
)

// Options selects byte bounds in stereo signed 16-bit PCM. Error identities are
// supplied by each owning API so sharing mechanics does not change its contract.
type Options struct {
	Start, End                  int64
	Loop                        bool
	InvalidData, InvalidRequest error
}

// Section borrows its source. The owner serializes Read/Seek and closes the
// original decoder after the native read-retirement barrier, where applicable.
type Section struct {
	source                io.ReadSeeker
	start, size, position int64
	options               Options
}

// Select validates bounds and positions the source before returning a view.
func Select(source io.ReadSeeker, length int64, options Options) (*Section, error) {
	const frameBytes = 4
	if length <= 0 || length%frameBytes != 0 || options.Start < 0 || options.End <= options.Start ||
		options.End > length || options.Start%frameBytes != 0 || options.End%frameBytes != 0 {
		return nil, fmt.Errorf("%w: positive whole-frame PCM window required", options.InvalidData)
	}
	selected := &Section{source: source, start: options.Start,
		size: options.End - options.Start, options: options, position: 0}
	if _, err := selected.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	return selected, nil
}

// Length is the finite selected region length, including for a looping view.
func (source *Section) Length() int64 { return source.size }

func (source *Section) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if source.position == source.size {
		if !source.options.Loop {
			return 0, io.EOF
		}
		if _, err := source.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
	}
	buffer = buffer[:min(int64(len(buffer)), source.size-source.position)]
	count, err := source.source.Read(buffer)
	if count < 0 || count > len(buffer) {
		return 0, fmt.Errorf("%w: invalid decoded byte count", source.options.InvalidData)
	}
	source.position += int64(count)
	if errors.Is(err, io.EOF) {
		if source.position < source.size {
			return count, io.ErrUnexpectedEOF
		}
		if source.options.Loop {
			return count, nil
		}
	}

	return count, err //nolint:wrapcheck // io.Reader requires an unwrapped EOF sentinel at the selected boundary.
}

// Seek validates the region-relative position before moving the borrowed decoder.
func (source *Section) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		if offset > source.size-source.position || offset < -source.position {
			return 0, fmt.Errorf("%w: seek outside recording", source.options.InvalidRequest)
		}
		offset += source.position
	case io.SeekEnd:
		if offset > 0 || offset < -source.size {
			return 0, fmt.Errorf("%w: seek outside recording", source.options.InvalidRequest)
		}
		offset += source.size
	default:
		return 0, fmt.Errorf("%w: invalid seek origin", source.options.InvalidRequest)
	}
	if offset < 0 || offset > source.size {
		return 0, fmt.Errorf("%w: seek outside recording", source.options.InvalidRequest)
	}
	if _, err := source.source.Seek(source.start+offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek decoded recording: %w", err)
	}
	source.position = offset

	return offset, nil
}
