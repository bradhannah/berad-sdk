package authoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrRecordingSourceChanged means the loaded bytes are no longer the file being
// authored. Keep the draft and require an explicit source reload/review.
var ErrRecordingSourceChanged = errors.New("recording source changed; reload and review the region")

// Fingerprint identifies the immutable encoded bytes used for analysis/audition.
func (clip *RecordingAudio) Fingerprint() string { return hex.EncodeToString(clip.fingerprint[:]) }

// CheckSourceStamp is a cheap freshness probe, not a content proof. Apply always
// hashes the file, including same-size replacements with preserved timestamps.
func (clip *RecordingAudio) CheckSourceStamp() error {
	info, err := os.Stat(clip.path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRecordingSourceChanged, err)
	}
	if clip.fileInfo == nil || !os.SameFile(clip.fileInfo, info) ||
		info.Size() != clip.fileInfo.Size() || !info.ModTime().Equal(clip.fileInfo.ModTime()) {
		return ErrRecordingSourceChanged
	}

	return nil
}

// VerifySource performs a bounded content check at the authoring boundary.
// Callers run this outside latency-sensitive input dispatch. An external writer
// can still change the file afterward; this is not a filesystem write lock.
func (clip *RecordingAudio) VerifySource(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify recording canceled: %w", err)
	}
	info, err := os.Stat(clip.path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRecordingSourceChanged, err)
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(clip.data)) {
		return ErrRecordingSourceChanged
	}
	file, err := os.Open(clip.path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRecordingSourceChanged, err)
	}
	verifyErr := clip.verifyBytes(ctx, file)
	if err := errors.Join(verifyErr, file.Close()); err != nil {
		return fmt.Errorf("verify recording source: %w", err)
	}

	return nil
}

func (clip *RecordingAudio) verifyBytes(ctx context.Context, source io.Reader) error {
	const blockBytes = 64 << 10
	buffer := make([]byte, blockBytes)
	digest := sha256.New()
	read := 0
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("verify recording canceled: %w", err)
		}
		count, err := source.Read(buffer)
		if cancelErr := ctx.Err(); cancelErr != nil {
			return fmt.Errorf("verify recording canceled: %w", cancelErr)
		}
		read += count
		if read > len(clip.data) {
			return ErrRecordingSourceChanged
		}
		if _, writeErr := digest.Write(buffer[:count]); writeErr != nil {
			return fmt.Errorf("hash recording: %w", writeErr)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read recording for verification: %w", err)
		}
	}
	if read != len(clip.data) || !bytes.Equal(digest.Sum(nil), clip.fingerprint[:]) {
		return ErrRecordingSourceChanged
	}

	return nil
}
