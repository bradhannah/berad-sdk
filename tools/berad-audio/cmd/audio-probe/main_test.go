package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckIsDeviceFreeAndDoesNotWriteSource(t *testing.T) {
	data := make([]byte, 44+441*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], 44100)
	binary.LittleEndian.PutUint32(data[28:], 44100*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	path := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{"-file", path, "-check"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "10 ms") {
		t.Fatal(output.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("check changed source", err)
	}
	for _, arguments := range [][]string{nil, {"-file", path, "-duration", "0s"}, {"-file", path, "-check", "unexpected"}} {
		if err := run(context.Background(), arguments, &output); err == nil {
			t.Fatal("invalid arguments accepted", arguments)
		}
	}
}
