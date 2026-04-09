// Package recording handles audio recording of call sessions to WAV files.
package recording

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Stream identifies which RTP leg a packet belongs to.
type Stream string

const (
	StreamRemote Stream = "remote"
	StreamLocal  Stream = "local"
)

// WAV file format constants for PCM audio.
const (
	wavHeaderSize    = 44
	wavFormatPCM     = 1
	wavChannelsMono  = 1
	wavSampleRate    = 8000
	wavBitsPerSample = 16
	wavByteRate      = wavSampleRate * wavChannelsMono * (wavBitsPerSample / 8)
	wavBlockAlign    = wavChannelsMono * (wavBitsPerSample / 8)
	wavFmtChunkSize  = 16

	// Byte offsets within the WAV header.
	wavOffsetRIFFSize = 4
	wavOffsetDataSize = 40

	// Size of the header after the RIFF chunk ID + size fields.
	wavRIFFHeaderOverhead = 36 // total header (44) - RIFF tag (4) - RIFF size (4)
)

type streamState struct {
	initialized       bool
	baseTimestamp     uint32
	baseOffsetSamples int
}

// Recorder mixes RTP audio payloads into a mono PCM WAV file.
type Recorder struct {
	mu               sync.Mutex
	callID           string
	dir              string
	path             string
	started          bool
	recordingStartAt time.Time
	remoteState      streamState
	localState       streamState
	mixedSamples     []int16
}

// NewRecorder creates a recorder that will write to a WAV file in dir.
func NewRecorder(dir, callID string) *Recorder {
	return &Recorder{
		dir:    dir,
		callID: callID,
	}
}

// Start opens the WAV file and writes the initial header.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return nil
	}

	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return fmt.Errorf("create recording directory %s: %w", r.dir, err)
	}

	r.path = filepath.Join(r.dir, r.callID+".wav")
	r.started = true
	r.recordingStartAt = time.Time{}
	r.remoteState = streamState{}
	r.localState = streamState{}
	r.mixedSamples = nil

	log.Debug().Str("call_id", r.callID).Str("path", r.path).Msg("recording started")
	return nil
}

// WritePacket mixes an RTP payload into the recording timeline.
func (r *Recorder) WritePacket(stream Stream, timestamp uint32, receivedAt time.Time, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.started || len(payload) == 0 {
		return
	}

	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}

	if r.recordingStartAt.IsZero() {
		r.recordingStartAt = receivedAt
	}

	state := r.streamState(stream)
	if state == nil {
		return
	}

	if !state.initialized {
		state.initialized = true
		state.baseTimestamp = timestamp
		state.baseOffsetSamples = samplesFromDuration(receivedAt.Sub(r.recordingStartAt))
	}

	sampleOffset := state.baseOffsetSamples + int(timestamp-state.baseTimestamp)
	if sampleOffset < 0 {
		sampleOffset = 0
	}

	r.ensureCapacity(sampleOffset + len(payload))
	for i, encoded := range payload {
		index := sampleOffset + i
		r.mixedSamples[index] = mixSample(r.mixedSamples[index], muLawToPCM(encoded))
	}
}

// Stop writes the mixed WAV file and returns its path.
func (r *Recorder) Stop() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.started {
		return ""
	}

	if err := r.writeWAVFile(); err != nil {
		log.Warn().Err(err).Str("call_id", r.callID).Str("path", r.path).Msg("failed to finalize recording")
	}

	path := r.path
	dataSize := len(r.mixedSamples) * (wavBitsPerSample / 8)
	r.path = ""
	r.started = false
	r.recordingStartAt = time.Time{}
	r.remoteState = streamState{}
	r.localState = streamState{}
	r.mixedSamples = nil

	log.Debug().Str("call_id", r.callID).Str("path", path).Int("data_bytes", dataSize).Msg("recording stopped")
	return path
}

func (r *Recorder) writeWAVFile() error {
	file, err := os.Create(r.path)
	if err != nil {
		return fmt.Errorf("create recording file %s: %w", r.path, err)
	}
	defer file.Close()

	if err := writeWAVHeader(file, uint32(len(r.mixedSamples)*(wavBitsPerSample/8))); err != nil {
		return err
	}

	for _, sample := range r.mixedSamples {
		if err := binary.Write(file, binary.LittleEndian, sample); err != nil {
			return fmt.Errorf("write WAV sample: %w", err)
		}
	}

	return nil
}

func writeWAVHeader(file *os.File, dataSize uint32) error {
	header := make([]byte, wavHeaderSize)

	// RIFF header
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], dataSize+wavRIFFHeaderOverhead)
	copy(header[8:12], "WAVE")

	// fmt sub-chunk
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], wavFmtChunkSize)
	binary.LittleEndian.PutUint16(header[20:22], wavFormatPCM)
	binary.LittleEndian.PutUint16(header[22:24], wavChannelsMono)
	binary.LittleEndian.PutUint32(header[24:28], wavSampleRate)
	binary.LittleEndian.PutUint32(header[28:32], wavByteRate)
	binary.LittleEndian.PutUint16(header[32:34], wavBlockAlign)
	binary.LittleEndian.PutUint16(header[34:36], wavBitsPerSample)

	// data sub-chunk
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)

	_, err := file.Write(header)
	return err
}

func (r *Recorder) streamState(stream Stream) *streamState {
	switch stream {
	case StreamRemote:
		return &r.remoteState
	case StreamLocal:
		return &r.localState
	default:
		return nil
	}
}

func (r *Recorder) ensureCapacity(size int) {
	if len(r.mixedSamples) >= size {
		return
	}

	r.mixedSamples = append(r.mixedSamples, make([]int16, size-len(r.mixedSamples))...)
}

func samplesFromDuration(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}

	return int(duration.Milliseconds() * wavSampleRate / 1000)
}

func mixSample(existing, incoming int16) int16 {
	sum := int32(existing) + int32(incoming)
	if sum > 32767 {
		return 32767
	}
	if sum < -32768 {
		return -32768
	}
	return int16(sum)
}

func muLawToPCM(value byte) int16 {
	value = ^value

	magnitude := int16((value&0x0F)<<3) + 0x84
	magnitude <<= (value & 0x70) >> 4

	if value&0x80 != 0 {
		return 0x84 - magnitude
	}

	return magnitude - 0x84
}
