package pcm

import (
	"fmt"
	"io"
)

// Read fills a PCM request while preserving source errors even with a full final
// buffer. Broken byte counts and repeated empty reads fail rather than hanging.
func Read(source io.Reader, buffer []byte, invalidData error) (int, error) {
	count, empty := 0, 0
	for count < len(buffer) {
		read, err := source.Read(buffer[count:])
		if read < 0 || read > len(buffer)-count {
			return count, fmt.Errorf("%w: invalid read count", invalidData)
		}
		count += read
		if err != nil {
			return count, err
		}
		if read == 0 {
			empty++
			const maxEmptyReads = 100
			if empty == maxEmptyReads {
				return count, io.ErrNoProgress
			}
		} else {
			empty = 0
		}
	}
	return count, nil
}
