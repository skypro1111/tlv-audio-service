package audio

import (
	"testing"
	"time"
	
	"github.com/skypro1111/tlv-audio-service/internal/vad"
)

func TestNewChunker(t *testing.T) {
	config := ChunkingConfig{
		MinDuration:        1 * time.Second,
		MaxDuration:        30 * time.Second,
		MinSpeechDuration:  500 * time.Millisecond,
		MinSilenceDuration: 300 * time.Millisecond,
		SampleRate:         8000,
		Format:             "wav",
	}
	
	chunker := NewChunker(config)
	if chunker == nil {
		t.Fatal("NewChunker returned nil")
	}
	
	if !chunker.IsIdle() {
		t.Error("New chunker should be idle")
	}
	
	if chunker.HasPendingChunk() {
		t.Error("New chunker should not have pending chunk")
	}
}

func TestChunkerSimpleVoiceSequence(t *testing.T) {
	config := ChunkingConfig{
		MinDuration:        500 * time.Millisecond,  // Reduced for testing
		MaxDuration:        10 * time.Second,
		MinSpeechDuration:  200 * time.Millisecond,  // Reduced for testing
		MinSilenceDuration: 100 * time.Millisecond,  // Reduced for testing
		SampleRate:         8000,
		Format:             "wav",
	}
	
	chunker := NewChunker(config)
	
	// Create test audio windows
	windowSize := 512
	samples := make([]int16, windowSize)
	for i := range samples {
		samples[i] = int16(1000) // Some audio data
	}
	
	// Create voice detection results
	voiceResult := &vad.VADResult{
		Probability: 0.8,
		HasVoice:    true,
		Confidence:  0.9,
		Timestamp:   time.Now(),
	}
	
	silenceResult := &vad.VADResult{
		Probability: 0.2,
		HasVoice:    false,
		Confidence:  0.7,
		Timestamp:   time.Now(),
	}
	
	window := &AudioWindow{
		Samples:   samples,
		StartSeq:  1,
		EndSeq:    1,
		Timestamp: time.Now(),
		HasGaps:   false,
	}
	
	// Send some voice frames to start a chunk
	for i := 0; i < 20; i++ { // About 1.2 seconds of voice at 64ms per window
		window.StartSeq = uint32(i + 1)
		window.EndSeq = uint32(i + 1)
		window.Timestamp = time.Now()
		
		chunk, err := chunker.ProcessVADResult(12345, 1, "test-caller", window, voiceResult)
		if err != nil {
			t.Fatalf("ProcessVADResult failed: %v", err)
		}
		
		// Should not create chunk yet
		if chunk != nil && i < 15 {
			t.Errorf("Unexpected chunk created at frame %d", i)
		}
		
		// Simulate real-time processing delay (64ms per window)
		time.Sleep(10 * time.Millisecond)
	}
	
	// Now send silence to trigger chunk creation
	for i := 0; i < 10; i++ {
		window.StartSeq = uint32(i + 21)
		window.EndSeq = uint32(i + 21)
		window.Timestamp = time.Now()
		
		chunk, err := chunker.ProcessVADResult(12345, 1, "test-caller", window, silenceResult)
		if err != nil {
			t.Fatalf("ProcessVADResult failed: %v", err)
		}
		
		// Should create chunk after enough silence
		if chunk != nil {
			if chunk.StreamID != 12345 {
				t.Errorf("Expected stream ID 12345, got %d", chunk.StreamID)
			}
			if chunk.Direction != 1 {
				t.Errorf("Expected direction 1, got %d", chunk.Direction)
			}
			if chunk.CallerID != "test-caller" {
				t.Errorf("Expected caller ID 'test-caller', got '%s'", chunk.CallerID)
			}
			if chunk.Format != "wav" {
				t.Errorf("Expected format 'wav', got '%s'", chunk.Format)
			}
			if chunk.SampleRate != 8000 {
				t.Errorf("Expected sample rate 8000, got %d", chunk.SampleRate)
			}
			if len(chunk.Samples) == 0 {
				t.Error("Chunk should have audio samples")
			}
			if chunk.Duration <= 0 {
				t.Error("Chunk duration should be positive")
			}
			
			// Found chunk, test passed
			return
		}
		
		// Simulate real-time processing delay
		time.Sleep(15 * time.Millisecond)
	}
	
	t.Error("Expected chunk to be created but none was found")
}

func TestChunkerMaxDurationTimeout(t *testing.T) {
	config := ChunkingConfig{
		MinDuration:        100 * time.Millisecond,
		MaxDuration:        500 * time.Millisecond, // Shorter max duration for testing
		MinSpeechDuration:  50 * time.Millisecond,
		MinSilenceDuration: 100 * time.Millisecond,
		SampleRate:         8000,
		Format:             "wav",
	}
	
	chunker := NewChunker(config)
	
	samples := make([]int16, 512)
	for i := range samples {
		samples[i] = int16(1000)
	}
	
	voiceResult := &vad.VADResult{
		Probability: 0.8,
		HasVoice:    true,
		Confidence:  0.9,
		Timestamp:   time.Now(),
	}
	
	window := &AudioWindow{
		Samples:   samples,
		StartSeq:  1,
		EndSeq:    1,
		Timestamp: time.Now(),
		HasGaps:   false,
	}
	
	// Keep sending voice for longer than max duration
	var chunk *AudioChunk
	var err error
	
	for i := 0; i < 50; i++ { // Much longer than 1 second worth
		window.StartSeq = uint32(i + 1)
		window.EndSeq = uint32(i + 1)
		window.Timestamp = time.Now()
		
		chunk, err = chunker.ProcessVADResult(12345, 1, "", window, voiceResult)
		if err != nil {
			t.Fatalf("ProcessVADResult failed: %v", err)
		}
		
		if chunk != nil {
			break
		}
		
		// Small delay to simulate real-time processing
		time.Sleep(15 * time.Millisecond)
	}
	
	if chunk == nil {
		t.Error("Expected chunk to be created due to max duration timeout")
	}
}

func TestChunkerForceFinalize(t *testing.T) {
	config := ChunkingConfig{
		MinDuration:        1 * time.Second,
		MaxDuration:        30 * time.Second,
		MinSpeechDuration:  500 * time.Millisecond,
		MinSilenceDuration: 300 * time.Millisecond,
		SampleRate:         8000,
		Format:             "wav",
	}
	
	chunker := NewChunker(config)
	
	samples := make([]int16, 512)
	for i := range samples {
		samples[i] = int16(1000)
	}
	
	voiceResult := &vad.VADResult{
		Probability: 0.8,
		HasVoice:    true,
		Confidence:  0.9,
		Timestamp:   time.Now(),
	}
	
	window := &AudioWindow{
		Samples:   samples,
		StartSeq:  1,
		EndSeq:    1,
		Timestamp: time.Now(),
		HasGaps:   false,
	}
	
	// Start a chunk but don't finish it
	_, err := chunker.ProcessVADResult(12345, 1, "", window, voiceResult)
	if err != nil {
		t.Fatalf("ProcessVADResult failed: %v", err)
	}
	
	// Chunker should have pending chunk
	if !chunker.HasPendingChunk() {
		t.Error("Chunker should have pending chunk")
	}
	
	// Force finalize
	chunk := chunker.ForceFinalize()
	if chunk == nil {
		t.Error("ForceFinalize should return a chunk")
	}
	
	// Chunker should be idle now
	if !chunker.IsIdle() {
		t.Error("Chunker should be idle after force finalize")
	}
	
	if chunker.HasPendingChunk() {
		t.Error("Chunker should not have pending chunk after force finalize")
	}
}

func TestChunkerStats(t *testing.T) {
	config := ChunkingConfig{
		MinDuration:        100 * time.Millisecond,
		MaxDuration:        1 * time.Second,
		MinSpeechDuration:  50 * time.Millisecond,
		MinSilenceDuration: 50 * time.Millisecond,
		SampleRate:         8000,
		Format:             "wav",
	}
	
	chunker := NewChunker(config)
	
	// Initial stats
	stats := chunker.GetStats()
	if stats.ChunksCreated != 0 {
		t.Errorf("Expected 0 chunks created, got %d", stats.ChunksCreated)
	}
	if stats.State != "idle" {
		t.Errorf("Expected state 'idle', got '%s'", stats.State)
	}
	
	samples := make([]int16, 512)
	for i := range samples {
		samples[i] = int16(1000)
	}
	
	voiceResult := &vad.VADResult{
		Probability: 0.8,
		HasVoice:    true,
		Confidence:  0.9,
		Timestamp:   time.Now(),
	}
	
	silenceResult := &vad.VADResult{
		Probability: 0.2,
		HasVoice:    false,
		Confidence:  0.7,
		Timestamp:   time.Now(),
	}
	
	window := &AudioWindow{
		Samples:   samples,
		StartSeq:  1,
		EndSeq:    1,
		Timestamp: time.Now(),
		HasGaps:   false,
	}
	
	// Process some voice
	for i := 0; i < 10; i++ { // More frames to ensure minimum durations are met
		window.StartSeq = uint32(i + 1)
		window.EndSeq = uint32(i + 1)
		window.Timestamp = time.Now()
		chunker.ProcessVADResult(12345, 1, "", window, voiceResult)
		time.Sleep(15 * time.Millisecond) // Simulate real-time processing
	}
	
	// Should be collecting now
	stats = chunker.GetStats()
	if stats.State != "collecting" {
		t.Errorf("Expected state 'collecting', got '%s'", stats.State)
	}
	
	// Process silence to create chunk
	for i := 0; i < 10; i++ {
		window.StartSeq = uint32(i + 11)
		window.EndSeq = uint32(i + 11)
		window.Timestamp = time.Now()
		chunk, _ := chunker.ProcessVADResult(12345, 1, "", window, silenceResult)
		if chunk != nil {
			break
		}
		time.Sleep(20 * time.Millisecond) // Simulate real-time processing
	}
	
	// Check final stats
	stats = chunker.GetStats()
	if stats.ChunksCreated == 0 {
		t.Error("Expected at least 1 chunk to be created")
	}
	if stats.AvgChunkSize <= 0 {
		t.Error("Expected positive average chunk size")
	}
} 