package audio

import (
	"testing"
	"math"
)

func TestEncodeWAV(t *testing.T) {
	// Generate test audio samples (440Hz sine wave for 0.1 seconds at 8kHz)
	sampleRate := 8000
	duration := 0.1 // 0.1 seconds
	frequency := 440.0 // 440Hz (A4 note)
	
	numSamples := int(float64(sampleRate) * duration)
	samples := make([]int16, numSamples)
	
	for i := 0; i < numSamples; i++ {
		// Generate sine wave
		t := float64(i) / float64(sampleRate)
		amplitude := 16383.0 // Half of max int16 to avoid clipping
		sample := amplitude * math.Sin(2*math.Pi*frequency*t)
		samples[i] = int16(sample)
	}
	
	// Encode to WAV
	wavData, err := EncodeWAV(samples, sampleRate)
	if err != nil {
		t.Fatalf("EncodeWAV failed: %v", err)
	}
	
	// Check that we got some data
	if len(wavData) == 0 {
		t.Fatal("WAV data is empty")
	}
	
	// WAV header should be 44 bytes
	expectedSize := 44 + len(samples)*2
	if len(wavData) != expectedSize {
		t.Errorf("Expected WAV size %d, got %d", expectedSize, len(wavData))
	}
	
	// Validate WAV format
	if err := ValidateWAV(wavData); err != nil {
		t.Errorf("Generated WAV is invalid: %v", err)
	}
	
	// Check WAV info
	info, err := GetWAVInfo(wavData)
	if err != nil {
		t.Errorf("Failed to get WAV info: %v", err)
	}
	
	if info.SampleRate != uint32(sampleRate) {
		t.Errorf("Expected sample rate %d, got %d", sampleRate, info.SampleRate)
	}
	
	if info.Channels != 1 {
		t.Errorf("Expected 1 channel, got %d", info.Channels)
	}
	
	if info.BitsPerSample != 16 {
		t.Errorf("Expected 16 bits per sample, got %d", info.BitsPerSample)
	}
	
	expectedDuration := float64(numSamples) / float64(sampleRate)
	if math.Abs(info.Duration-expectedDuration) > 0.001 {
		t.Errorf("Expected duration %.3f, got %.3f", expectedDuration, info.Duration)
	}
}

func TestDecodeWAV(t *testing.T) {
	// Create test samples
	originalSamples := []int16{100, -200, 300, -400, 500}
	sampleRate := 8000
	
	// Encode to WAV
	wavData, err := EncodeWAV(originalSamples, sampleRate)
	if err != nil {
		t.Fatalf("EncodeWAV failed: %v", err)
	}
	
	// Decode back to samples
	decodedSamples, decodedSampleRate, err := DecodeWAV(wavData)
	if err != nil {
		t.Fatalf("DecodeWAV failed: %v", err)
	}
	
	// Check sample rate
	if decodedSampleRate != sampleRate {
		t.Errorf("Expected sample rate %d, got %d", sampleRate, decodedSampleRate)
	}
	
	// Check samples match
	if len(decodedSamples) != len(originalSamples) {
		t.Errorf("Expected %d samples, got %d", len(originalSamples), len(decodedSamples))
	}
	
	for i, original := range originalSamples {
		if i >= len(decodedSamples) {
			break
		}
		if decodedSamples[i] != original {
			t.Errorf("Sample %d: expected %d, got %d", i, original, decodedSamples[i])
		}
	}
}

func TestEncodeWAVEmpty(t *testing.T) {
	// Test with empty samples
	_, err := EncodeWAV([]int16{}, 8000)
	if err == nil {
		t.Error("Expected error for empty samples")
	}
}

func TestEncodeWAVInvalidSampleRate(t *testing.T) {
	// Test with invalid sample rate
	samples := []int16{100, 200, 300}
	_, err := EncodeWAV(samples, 0)
	if err == nil {
		t.Error("Expected error for zero sample rate")
	}
	
	_, err = EncodeWAV(samples, -1000)
	if err == nil {
		t.Error("Expected error for negative sample rate")
	}
}

func TestValidateWAV(t *testing.T) {
	// Test with too short data
	err := ValidateWAV([]byte{1, 2, 3})
	if err == nil {
		t.Error("Expected error for too short WAV data")
	}
	
	// Test with invalid header
	invalidWAV := make([]byte, 50)
	copy(invalidWAV[0:4], []byte("FAKE"))
	err = ValidateWAV(invalidWAV)
	if err == nil {
		t.Error("Expected error for invalid RIFF header")
	}
}

func TestGetWAVDuration(t *testing.T) {
	// Create 1 second of audio at 8kHz
	sampleRate := 8000
	samples := make([]int16, sampleRate) // 1 second
	for i := range samples {
		samples[i] = int16(i % 1000)
	}
	
	wavData, err := EncodeWAV(samples, sampleRate)
	if err != nil {
		t.Fatalf("EncodeWAV failed: %v", err)
	}
	
	duration, err := GetWAVDuration(wavData)
	if err != nil {
		t.Fatalf("GetWAVDuration failed: %v", err)
	}
	
	expectedDuration := 1.0 // 1 second
	if math.Abs(duration-expectedDuration) > 0.001 {
		t.Errorf("Expected duration %.3f, got %.3f", expectedDuration, duration)
	}
} 