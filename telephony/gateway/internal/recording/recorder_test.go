package recording

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecorderWriteAndStop(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, "test-call")

	if err := r.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Start is idempotent.
	if err := r.Start(); err != nil {
		t.Fatalf("second Start() error: %v", err)
	}

	payload := []byte{0x80, 0x81, 0x82, 0x83}
	r.WritePacket(StreamRemote, 1000, time.Unix(0, 0), payload)
	r.WritePacket(StreamRemote, 1004, time.Unix(0, 4*int64(time.Millisecond)), payload)

	path := r.Stop()
	if path == "" {
		t.Fatal("Stop() returned empty path")
	}

	expectedPath := filepath.Join(dir, "test-call.wav")
	if path != expectedPath {
		t.Errorf("path = %q, want %q", path, expectedPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	// Verify WAV header markers.
	if string(data[0:4]) != "RIFF" {
		t.Error("missing RIFF header")
	}
	if string(data[8:12]) != "WAVE" {
		t.Error("missing WAVE format")
	}
	if string(data[12:16]) != "fmt " {
		t.Error("missing fmt sub-chunk")
	}
	if string(data[36:40]) != "data" {
		t.Error("missing data sub-chunk")
	}

	// Verify data size field at offset 40: 8 samples x 2 bytes = 16.
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	if dataSize != 16 {
		t.Errorf("data size = %d, want 16", dataSize)
	}

	// Verify RIFF size field at offset 4.
	riffSize := binary.LittleEndian.Uint32(data[4:8])
	if riffSize != 16+wavRIFFHeaderOverhead {
		t.Errorf("RIFF size = %d, want %d", riffSize, 16+wavRIFFHeaderOverhead)
	}

	// Verify PCM format tag at offset 20.
	formatTag := binary.LittleEndian.Uint16(data[20:22])
	if formatTag != wavFormatPCM {
		t.Errorf("format tag = %d, want %d (PCM)", formatTag, wavFormatPCM)
	}
}

func TestRecorderStopWithoutStart(t *testing.T) {
	r := NewRecorder(t.TempDir(), "no-start")
	if path := r.Stop(); path != "" {
		t.Errorf("Stop() without Start() returned %q, want empty", path)
	}
}

func TestRecorderWriteWithoutStart(t *testing.T) {
	r := NewRecorder(t.TempDir(), "no-start")
	// Should not panic.
	r.WritePacket(StreamRemote, 1000, time.Unix(0, 0), []byte{0x01, 0x02})
}

func TestRecorderStopIdempotent(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, "idem")

	if err := r.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	path1 := r.Stop()
	path2 := r.Stop()

	if path1 == "" {
		t.Error("first Stop() returned empty")
	}
	if path2 != "" {
		t.Errorf("second Stop() returned %q, want empty", path2)
	}
}

func TestRecorderMixesOverlappingStreamsIntoSingleTimeline(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, "mixed")

	if err := r.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	recordedAt := time.Unix(0, 0)
	r.WritePacket(StreamRemote, 2000, recordedAt, []byte{0x80, 0x80, 0x80, 0x80})
	r.WritePacket(StreamLocal, 3000, recordedAt, []byte{0x80, 0x80, 0x80, 0x80})

	path := r.Stop()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	dataSize := binary.LittleEndian.Uint32(data[40:44])
	if dataSize != 8 {
		t.Errorf("data size = %d, want 8", dataSize)
	}
}
