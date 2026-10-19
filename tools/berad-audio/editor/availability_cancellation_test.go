package editorui

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestCancelledVariantAnalysisRetainsAvailabilityAndPlayback(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			m := fixture(t)
			separateVariant(t, m)
			dispatchRune(m, 'p')
			before := m.clip
			beforeStatus := m.status
			status := m.availability["healing_02"]
			command := m.previewNextClip()
			message := command().(loadedMsg)
			message.clip, message.err = nil, fmt.Errorf("analyze source: %w", failure)
			m.Update(message)
			require.Equal(t, status, m.availability["healing_02"])
			require.Same(t, before, m.clip)
			require.True(t, m.preview.Playing())
			require.Equal(t, "healing_01", m.recordingID())
			if failure == context.Canceled {
				require.Equal(t, beforeStatus, m.status, "superseded work stays quiet")
			} else {
				require.Contains(t, m.status, failure.Error())
			}
		})
	}
}

func TestStaleCancelledAnalysisCannotPoisonFreshSelection(t *testing.T) {
	m := fixture(t)
	command := m.loadSelection()
	message := command().(loadedMsg)
	message.clip, message.err = nil, fmt.Errorf("analyze source: %w", context.DeadlineExceeded)
	finish(m, m.loadSelection())
	current, status := m.clip, m.status
	m.Update(message)
	require.Same(t, current, m.clip)
	require.Equal(t, status, m.status)
	require.Equal(t, audio.RecordingReady, m.availability[m.recordingID()].State)
	require.False(t, m.loading)
}
