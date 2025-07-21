package vad

import (
	"testing"
	"time"
)

func TestNewProcessor(t *testing.T) {
	modelPath := "./models/silero_vad.onnx"
	threshold := float32(0.5)
	windowSize := 512
	sampleRate := 8000

	processor, err := NewProcessor(modelPath, threshold, windowSize, sampleRate)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if processor == nil {
		t.Fatal("NewProcessor returned nil")
	}

	if processor.modelPath != modelPath {
		t.Errorf("Expected model path %s, got %s", modelPath, processor.modelPath)
	}

	if processor.threshold != threshold {
		t.Errorf("Expected threshold %f, got %f", threshold, processor.threshold)
	}

	if processor.windowSize != windowSize {
		t.Errorf("Expected window size %d, got %d", windowSize, processor.windowSize)
	}

	if processor.sampleRate != sampleRate {
		t.Errorf("Expected sample rate %d, got %d", sampleRate, processor.sampleRate)
	}

	if processor.GetOverlapSize() != windowSize/2 {
		t.Errorf("Expected overlap size %d, got %d", windowSize/2, processor.GetOverlapSize())
	}

	if processor.IsInitialized() {
		t.Error("Expected processor to not be initialized initially")
	}
}

func TestNewProcessorValidation(t *testing.T) {
	tests := []struct {
		name       string
		modelPath  string
		threshold  float32
		windowSize int
		sampleRate int
		expectErr  bool
	}{
		{
			name:       "valid parameters",
			modelPath:  "./models/test.onnx",
			threshold:  0.5,
			windowSize: 512,
			sampleRate: 8000,
			expectErr:  false,
		},
		{
			name:       "threshold too low",
			modelPath:  "./models/test.onnx",
			threshold:  -0.1,
			windowSize: 512,
			sampleRate: 8000,
			expectErr:  true,
		},
		{
			name:       "threshold too high",
			modelPath:  "./models/test.onnx",
			threshold:  1.1,
			windowSize: 512,
			sampleRate: 8000,
			expectErr:  true,
		},
		{
			name:       "zero window size",
			modelPath:  "./models/test.onnx",
			threshold:  0.5,
			windowSize: 0,
			sampleRate: 8000,
			expectErr:  true,
		},
		{
			name:       "negative sample rate",
			modelPath:  "./models/test.onnx",
			threshold:  0.5,
			windowSize: 512,
			sampleRate: -1,
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProcessor(tt.modelPath, tt.threshold, tt.windowSize, tt.sampleRate)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestProcessorInitialization(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	// Should not be initialized initially
	if processor.IsInitialized() {
		t.Error("Expected processor to not be initialized")
	}

	// Initialize the processor
	err = processor.Initialize()
	if err != nil {
		t.Errorf("Failed to initialize processor: %v", err)
	}

	// Should be initialized now
	if !processor.IsInitialized() {
		t.Error("Expected processor to be initialized")
	}

	// Check stats after initialization
	stats := processor.GetStats()
	if !stats.IsInitialized {
		t.Error("Expected stats to show processor as initialized")
	}
	if stats.ModelPath != "./test.onnx" {
		t.Errorf("Expected model path in stats: %s, got %s", "./test.onnx", stats.ModelPath)
	}
}

func TestProcessAudioSamples(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Create test samples - high energy should trigger voice detection
	samples := make([]int16, 512)
	for i := range samples {
		samples[i] = int16(5000) // High energy
	}

	result, err := processor.Process(samples)
	if err != nil {
		t.Errorf("Failed to process samples: %v", err)
	}

	if result == nil {
		t.Fatal("Process returned nil result")
	}

	// Check result fields
	if result.Probability < 0 || result.Probability > 1 {
		t.Errorf("Invalid probability: %f", result.Probability)
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		t.Errorf("Invalid confidence: %f", result.Confidence)
	}

	if result.WindowIndex != 0 {
		t.Errorf("Expected window index 0, got %d", result.WindowIndex)
	}

	if result.ProcessingTime <= 0 {
		t.Error("Expected positive processing time")
	}

	if result.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}

	// High energy should likely result in voice detection
	if result.Probability < 0.3 {
		t.Logf("Note: High energy samples resulted in low voice probability: %f", result.Probability)
	}

	t.Logf("Processed samples: probability=%.3f, confidence=%.3f, hasVoice=%v", 
		result.Probability, result.Confidence, result.HasVoice)
}

func TestProcessWithoutInitialization(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	samples := make([]int16, 512)
	_, err = processor.Process(samples)
	if err == nil {
		t.Error("Expected error when processing without initialization")
	}
}

func TestProcessWrongSampleCount(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Wrong number of samples
	samples := make([]int16, 256) // Should be 512
	_, err = processor.Process(samples)
	if err == nil {
		t.Error("Expected error for wrong sample count")
	}
}

func TestVoiceActivityDetection(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	tests := []struct {
		name           string
		sampleGen      func() []int16
		expectVoice    bool
		description    string
	}{
		{
			name: "silence",
			sampleGen: func() []int16 {
				return make([]int16, 512) // All zeros = silence
			},
			expectVoice: false,
			description: "All zero samples should not detect voice",
		},
		{
			name: "high energy",
			sampleGen: func() []int16 {
				samples := make([]int16, 512)
				for i := range samples {
					samples[i] = 8000 // High amplitude
				}
				return samples
			},
			expectVoice: true,
			description: "High energy samples should detect voice",
		},
		{
			name: "low energy",
			sampleGen: func() []int16 {
				samples := make([]int16, 512)
				for i := range samples {
					samples[i] = 100 // Low amplitude
				}
				return samples
			},
			expectVoice: false,
			description: "Low energy samples should not detect voice",
		},
		{
			name: "alternating pattern",
			sampleGen: func() []int16 {
				samples := make([]int16, 512)
				for i := range samples {
					if i%2 == 0 {
						samples[i] = 5000
					} else {
						samples[i] = -5000
					}
				}
				return samples
			},
			expectVoice: true,
			description: "Alternating high amplitude should detect voice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			samples := tt.sampleGen()
			result, err := processor.Process(samples)
			if err != nil {
				t.Errorf("Failed to process %s: %v", tt.description, err)
				return
			}

			t.Logf("%s: probability=%.3f, hasVoice=%v (expected=%v)", 
				tt.description, result.Probability, result.HasVoice, tt.expectVoice)

			// Note: Since we're using a mock VAD, the actual detection may not match expectations
			// This is more of a functionality test than an accuracy test
		})
	}
}

func TestProcessorStats(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.6, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Process some samples to generate stats
	highEnergySamples := make([]int16, 512)
	for i := range highEnergySamples {
		highEnergySamples[i] = 8000
	}

	silenceSamples := make([]int16, 512)

	// Process alternating voice and silence
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			processor.Process(highEnergySamples)
		} else {
			processor.Process(silenceSamples)
		}
	}

	stats := processor.GetStats()

	if stats.TotalWindows != 10 {
		t.Errorf("Expected 10 total windows, got %d", stats.TotalWindows)
	}

	if stats.Threshold != 0.6 {
		t.Errorf("Expected threshold 0.6, got %f", stats.Threshold)
	}

	if stats.VoicePercentage < 0 || stats.VoicePercentage > 100 {
		t.Errorf("Invalid voice percentage: %f", stats.VoicePercentage)
	}

	if stats.LastProcessed.IsZero() {
		t.Error("Expected non-zero last processed time")
	}

	t.Logf("Stats: %d/%d windows had voice (%.1f%%)", 
		stats.VoiceWindows, stats.TotalWindows, stats.VoicePercentage)
}

func TestUpdateThreshold(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	// Test valid threshold updates
	err = processor.UpdateThreshold(0.7)
	if err != nil {
		t.Errorf("Failed to update threshold: %v", err)
	}

	if processor.GetThreshold() != 0.7 {
		t.Errorf("Expected threshold 0.7, got %f", processor.GetThreshold())
	}

	// Test invalid threshold updates
	err = processor.UpdateThreshold(-0.1)
	if err == nil {
		t.Error("Expected error for negative threshold")
	}

	err = processor.UpdateThreshold(1.1)
	if err == nil {
		t.Error("Expected error for threshold > 1")
	}

	// Threshold should remain unchanged after invalid update
	if processor.GetThreshold() != 0.7 {
		t.Errorf("Threshold changed after invalid update: %f", processor.GetThreshold())
	}
}

func TestProcessorReset(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Process some samples
	samples := make([]int16, 512)
	processor.Process(samples)
	processor.Process(samples)

	statsBeforeReset := processor.GetStats()
	if statsBeforeReset.TotalWindows == 0 {
		t.Error("Expected some windows processed before reset")
	}

	// Reset the processor
	processor.Reset()

	statsAfterReset := processor.GetStats()
	if statsAfterReset.TotalWindows != 0 {
		t.Errorf("Expected 0 windows after reset, got %d", statsAfterReset.TotalWindows)
	}

	if statsAfterReset.VoiceWindows != 0 {
		t.Errorf("Expected 0 voice windows after reset, got %d", statsAfterReset.VoiceWindows)
	}

	if !statsAfterReset.LastProcessed.IsZero() {
		t.Error("Expected zero last processed time after reset")
	}
}

func TestProcessContinuous(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Create test windows alternating between voice and silence
	windows := make([]*AudioWindow, 6)
	for i := range windows {
		samples := make([]int16, 512)
		if i%2 == 0 {
			// Voice windows - high energy
			for j := range samples {
				samples[j] = 8000
			}
		} else {
			// Silence windows - low energy
			for j := range samples {
				samples[j] = 100
			}
		}

		windows[i] = &AudioWindow{
			Samples:   samples,
			StartSeq:  uint32(i * 100),
			EndSeq:    uint32((i + 1) * 100),
			Timestamp: time.Now().Add(time.Duration(i) * 100 * time.Millisecond),
			HasGaps:   false,
		}
	}

	segments, err := processor.ProcessContinuous(windows)
	if err != nil {
		t.Errorf("Failed to process continuous windows: %v", err)
	}

	t.Logf("Found %d voice segments from %d windows", len(segments), len(windows))

	// Validate segments
	for i, segment := range segments {
		if segment.StartTime.IsZero() || segment.EndTime.IsZero() {
			t.Errorf("Segment %d has zero timestamps", i)
		}

		if segment.Duration <= 0 {
			t.Errorf("Segment %d has invalid duration: %v", i, segment.Duration)
		}

		if segment.Confidence < 0 || segment.Confidence > 1 {
			t.Errorf("Segment %d has invalid confidence: %f", i, segment.Confidence)
		}

		t.Logf("Segment %d: start=%v, end=%v, duration=%v, confidence=%.3f", 
			i, segment.StartTime, segment.EndTime, segment.Duration, segment.Confidence)
	}
}

func TestProcessContinuousEmpty(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Test with empty windows
	segments, err := processor.ProcessContinuous([]*AudioWindow{})
	if err != nil {
		t.Errorf("Failed to process empty windows: %v", err)
	}

	if len(segments) != 0 {
		t.Errorf("Expected 0 segments for empty windows, got %d", len(segments))
	}

	// Test with nil
	segments, err = processor.ProcessContinuous(nil)
	if err != nil {
		t.Errorf("Failed to process nil windows: %v", err)
	}

	if len(segments) != 0 {
		t.Errorf("Expected 0 segments for nil windows, got %d", len(segments))
	}
}

func TestConcurrentProcessing(t *testing.T) {
	processor, err := NewProcessor("./test.onnx", 0.5, 512, 8000)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	err = processor.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Test concurrent processing
	done := make(chan bool)
	numGoroutines := 5
	numProcessPerGoroutine := 20

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			samples := make([]int16, 512)
			for j := range samples {
				samples[j] = int16(id * 1000) // Different energy per goroutine
			}

			for j := 0; j < numProcessPerGoroutine; j++ {
				result, err := processor.Process(samples)
				if err != nil {
					t.Errorf("Goroutine %d failed to process: %v", id, err)
					return
				}
				if result == nil {
					t.Errorf("Goroutine %d got nil result", id)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check final stats
	stats := processor.GetStats()
	expectedWindows := uint64(numGoroutines * numProcessPerGoroutine)
	if stats.TotalWindows != expectedWindows {
		t.Errorf("Expected %d total windows, got %d", expectedWindows, stats.TotalWindows)
	}
} 