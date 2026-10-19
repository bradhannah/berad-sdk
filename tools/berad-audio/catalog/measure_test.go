package catalog

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

// Retain the consumer's signed-sample/silence regression beside its mechanism.
func TestPCMMeasurementsUseSignedSamplesAndNeverInventFiniteSilenceDB(t *testing.T) {
	var buffer bytes.Buffer
	if err := binary.Write(&buffer, binary.LittleEndian, []int16{-16384, 16384, -16384, 16384}); err != nil {
		t.Fatal(err)
	}
	peak, rms, err := measureCatalogPCM(&buffer)
	if err != nil || peak != 0.5 || rms != 0.5 {
		t.Fatal(peak, rms, err)
	}
	if level := amplitudeDB(peak); level == nil || math.Abs(*level-(-6.0206)) > 0.0001 {
		t.Fatal(level)
	}
	if amplitudeDB(0) != nil {
		t.Fatal("silence must not invent a finite dB value")
	}
	if _, _, err := measureCatalogPCM(bytes.NewReader([]byte{1})); !errors.Is(err, ErrAudioCatalog) {
		t.Fatal(err)
	}
}
