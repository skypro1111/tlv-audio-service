package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// WAVHeader represents the header structure of a WAV file
type WAVHeader struct {
	ChunkID       [4]byte // "RIFF"
	ChunkSize     uint32  // File size - 8 bytes
	Format        [4]byte // "WAVE"
	Subchunk1ID   [4]byte // "fmt "
	Subchunk1Size uint32  // 16 for PCM
	AudioFormat   uint16  // 1 for PCM
	NumChannels   uint16  // Number of channels
	SampleRate    uint32  // Sample rate
	ByteRate      uint32  // SampleRate * NumChannels * BitsPerSample / 8
	BlockAlign    uint16  // NumChannels * BitsPerSample / 8
	BitsPerSample uint16  // Bits per sample
	Subchunk2ID   [4]byte // "data"
	Subchunk2Size uint32  // Number of bytes in the data
}

// EncodeWAV encodes PCM-16 samples into WAV format
func EncodeWAV(samples []int16, sampleRate int) ([]byte, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("cannot encode empty audio samples")
	}
	
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sample rate must be positive, got %d", sampleRate)
	}
	
	// Calculate sizes
	numChannels := uint16(1)   // Mono
	bitsPerSample := uint16(16) // 16-bit PCM
	dataSize := uint32(len(samples) * 2) // 2 bytes per sample
	fileSize := 36 + dataSize // WAV header is 44 bytes, data starts at offset 44
	
	// Create WAV header
	header := WAVHeader{
		ChunkID:       [4]byte{'R', 'I', 'F', 'F'},
		ChunkSize:     fileSize,
		Format:        [4]byte{'W', 'A', 'V', 'E'},
		Subchunk1ID:   [4]byte{'f', 'm', 't', ' '},
		Subchunk1Size: 16,
		AudioFormat:   1, // PCM
		NumChannels:   numChannels,
		SampleRate:    uint32(sampleRate),
		ByteRate:      uint32(sampleRate) * uint32(numChannels) * uint32(bitsPerSample) / 8,
		BlockAlign:    numChannels * bitsPerSample / 8,
		BitsPerSample: bitsPerSample,
		Subchunk2ID:   [4]byte{'d', 'a', 't', 'a'},
		Subchunk2Size: dataSize,
	}
	
	// Create buffer for the entire WAV file
	buf := bytes.NewBuffer(make([]byte, 0, 44+len(samples)*2))
	
	// Write WAV header
	if err := binary.Write(buf, binary.LittleEndian, header); err != nil {
		return nil, fmt.Errorf("failed to write WAV header: %w", err)
	}
	
	// Write audio data (PCM samples)
	if err := binary.Write(buf, binary.LittleEndian, samples); err != nil {
		return nil, fmt.Errorf("failed to write audio data: %w", err)
	}
	
	return buf.Bytes(), nil
}

// DecodeWAV decodes WAV format data back to PCM-16 samples
func DecodeWAV(data []byte) ([]int16, int, error) {
	if len(data) < 44 {
		return nil, 0, fmt.Errorf("WAV data too short: need at least 44 bytes, got %d", len(data))
	}
	
	// Read and validate WAV header
	buf := bytes.NewReader(data)
	var header WAVHeader
	
	if err := binary.Read(buf, binary.LittleEndian, &header); err != nil {
		return nil, 0, fmt.Errorf("failed to read WAV header: %w", err)
	}
	
	// Validate WAV format
	if string(header.ChunkID[:]) != "RIFF" {
		return nil, 0, fmt.Errorf("invalid WAV file: missing RIFF header")
	}
	
	if string(header.Format[:]) != "WAVE" {
		return nil, 0, fmt.Errorf("invalid WAV file: missing WAVE format")
	}
	
	if string(header.Subchunk1ID[:]) != "fmt " {
		return nil, 0, fmt.Errorf("invalid WAV file: missing fmt chunk")
	}
	
	if string(header.Subchunk2ID[:]) != "data" {
		return nil, 0, fmt.Errorf("invalid WAV file: missing data chunk")
	}
	
	if header.AudioFormat != 1 {
		return nil, 0, fmt.Errorf("unsupported audio format: %d (only PCM is supported)", header.AudioFormat)
	}
	
	if header.BitsPerSample != 16 {
		return nil, 0, fmt.Errorf("unsupported bit depth: %d (only 16-bit is supported)", header.BitsPerSample)
	}
	
	if header.NumChannels != 1 {
		return nil, 0, fmt.Errorf("unsupported channel count: %d (only mono is supported)", header.NumChannels)
	}
	
	// Calculate number of samples
	numSamples := int(header.Subchunk2Size) / 2 // 2 bytes per sample
	if numSamples <= 0 {
		return nil, 0, fmt.Errorf("no audio data found")
	}
	
	// Read audio samples
	samples := make([]int16, numSamples)
	if err := binary.Read(buf, binary.LittleEndian, samples); err != nil {
		return nil, 0, fmt.Errorf("failed to read audio samples: %w", err)
	}
	
	return samples, int(header.SampleRate), nil
}

// ValidateWAV validates a WAV file format without decoding the entire audio data
func ValidateWAV(data []byte) error {
	if len(data) < 44 {
		return fmt.Errorf("WAV data too short: need at least 44 bytes, got %d", len(data))
	}
	
	// Check RIFF header
	if string(data[0:4]) != "RIFF" {
		return fmt.Errorf("invalid WAV file: missing RIFF header")
	}
	
	// Check WAVE format
	if string(data[8:12]) != "WAVE" {
		return fmt.Errorf("invalid WAV file: missing WAVE format")
	}
	
	// Check fmt chunk
	if string(data[12:16]) != "fmt " {
		return fmt.Errorf("invalid WAV file: missing fmt chunk")
	}
	
	// Check data chunk
	if string(data[36:40]) != "data" {
		return fmt.Errorf("invalid WAV file: missing data chunk")
	}
	
	return nil
}

// GetWAVDuration calculates the duration of a WAV file in seconds
func GetWAVDuration(data []byte) (float64, error) {
	if err := ValidateWAV(data); err != nil {
		return 0, err
	}
	
	// Read sample rate from header
	sampleRate := binary.LittleEndian.Uint32(data[24:28])
	if sampleRate == 0 {
		return 0, fmt.Errorf("invalid sample rate: 0")
	}
	
	// Read data chunk size
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	
	// Calculate duration (dataSize / 2 samples / sampleRate)
	numSamples := dataSize / 2 // 2 bytes per sample
	duration := float64(numSamples) / float64(sampleRate)
	
	return duration, nil
}

// GetWAVInfo returns basic information about a WAV file
type WAVInfo struct {
	SampleRate    uint32  `json:"sample_rate"`
	Channels      uint16  `json:"channels"`
	BitsPerSample uint16  `json:"bits_per_sample"`
	Duration      float64 `json:"duration_seconds"`
	DataSize      uint32  `json:"data_size_bytes"`
	NumSamples    uint32  `json:"num_samples"`
}

// GetWAVInfo extracts metadata from a WAV file
func GetWAVInfo(data []byte) (*WAVInfo, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("WAV data too short: need at least 44 bytes, got %d", len(data))
	}
	
	// Read header fields
	buf := bytes.NewReader(data)
	var header WAVHeader
	
	if err := binary.Read(buf, binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("failed to read WAV header: %w", err)
	}
	
	// Validate basic format
	if err := ValidateWAV(data); err != nil {
		return nil, err
	}
	
	// Calculate derived values
	numSamples := header.Subchunk2Size / (uint32(header.BitsPerSample) / 8)
	duration := float64(numSamples) / float64(header.SampleRate)
	
	return &WAVInfo{
		SampleRate:    header.SampleRate,
		Channels:      header.NumChannels,
		BitsPerSample: header.BitsPerSample,
		Duration:      duration,
		DataSize:      header.Subchunk2Size,
		NumSamples:    numSamples,
	}, nil
} 