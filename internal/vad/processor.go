package vad

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Processor provides Voice Activity Detection using Silero VAD model
type Processor struct {
	modelPath   string
	threshold   float32
	windowSize  int     // Samples per window (512 for 64ms at 8kHz)
	overlapSize int     // Overlap samples (256 for 50% overlap)
	sampleRate  int     // Audio sample rate (8000 Hz)
	
	// VAD state
	isInitialized bool
	lastResult    float32
	smoothing     float32  // Smoothing factor for results
	
	// Statistics
	totalWindows  uint64
	voiceWindows  uint64
	lastProcessed time.Time
	
	mu sync.RWMutex
}

// VADResult represents the result of voice activity detection
type VADResult struct {
	Probability    float32   `json:"probability"`     // Voice probability (0.0 - 1.0)
	HasVoice       bool      `json:"has_voice"`       // Whether voice was detected
	Confidence     float32   `json:"confidence"`      // Confidence in the result
	WindowIndex    int       `json:"window_index"`    // Window index processed
	ProcessingTime time.Duration `json:"processing_time"` // Time taken to process
	Timestamp      time.Time `json:"timestamp"`       // When processing occurred
}

// VoiceSegment represents a continuous segment of voice activity
type VoiceSegment struct {
	StartTime  time.Time `json:"start_time"`  // When voice segment started
	EndTime    time.Time `json:"end_time"`    // When voice segment ended
	Duration   time.Duration `json:"duration"` // Duration of the segment
	Confidence float32   `json:"confidence"`   // Average confidence for the segment
	StartSeq   uint32    `json:"start_seq"`    // Starting sequence number
	EndSeq     uint32    `json:"end_seq"`      // Ending sequence number
}

// ProcessorStats represents VAD processor statistics
type ProcessorStats struct {
	ModelPath       string    `json:"model_path"`
	IsInitialized   bool      `json:"is_initialized"`
	TotalWindows    uint64    `json:"total_windows"`
	VoiceWindows    uint64    `json:"voice_windows"`
	VoicePercentage float64   `json:"voice_percentage"`
	LastProcessed   time.Time `json:"last_processed"`
	Threshold       float32   `json:"threshold"`
}

// NewProcessor creates a new VAD processor instance
func NewProcessor(modelPath string, threshold float32, windowSize int, sampleRate int) (*Processor, error) {
	if threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("threshold must be between 0 and 1, got %f", threshold)
	}
	
	if windowSize <= 0 {
		return nil, fmt.Errorf("window size must be positive, got %d", windowSize)
	}
	
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sample rate must be positive, got %d", sampleRate)
	}
	
	processor := &Processor{
		modelPath:   modelPath,
		threshold:   threshold,
		windowSize:  windowSize,
		overlapSize: windowSize / 2, // 50% overlap
		sampleRate:  sampleRate,
		smoothing:   0.1, // Light smoothing factor
	}
	
	return processor, nil
}

// Initialize initializes the VAD processor and loads the model
func (p *Processor) Initialize() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// TODO: In a real implementation, this would load the ONNX model
	// For now, we'll use a mock implementation
	
	// Simulate model loading time
	time.Sleep(100 * time.Millisecond)
	
	p.isInitialized = true
	p.lastProcessed = time.Now()
	
	return nil
}

// Process processes a window of audio samples and returns voice activity probability
func (p *Processor) Process(samples []int16) (*VADResult, error) {
	startTime := time.Now()
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.isInitialized {
		return nil, fmt.Errorf("processor not initialized")
	}
	
	if len(samples) != p.windowSize {
		return nil, fmt.Errorf("expected %d samples, got %d", p.windowSize, len(samples))
	}
	
	// Process the audio window
	probability := p.processWindow(samples)
	
	// Apply smoothing
	if p.totalWindows > 0 {
		probability = p.smoothing*probability + (1-p.smoothing)*p.lastResult
	}
	p.lastResult = probability
	
	// Determine if voice is detected
	hasVoice := probability >= p.threshold
	
	// Update statistics
	p.totalWindows++
	if hasVoice {
		p.voiceWindows++
	}
	p.lastProcessed = time.Now()
	
	// Calculate confidence (higher when probability is far from threshold)
	confidence := float32(math.Abs(float64(probability - p.threshold)))
	if confidence > 0.5 {
		confidence = 0.5
	}
	confidence = confidence * 2 // Scale to 0-1
	
	result := &VADResult{
		Probability:    probability,
		HasVoice:       hasVoice,
		Confidence:     confidence,
		WindowIndex:    int(p.totalWindows - 1),
		ProcessingTime: time.Since(startTime),
		Timestamp:      time.Now(),
	}
	
	return result, nil
}

// processWindow performs the actual VAD processing on a window of samples
func (p *Processor) processWindow(samples []int16) float32 {
	// TODO: In a real implementation, this would call the Silero VAD ONNX model
	// For now, we'll implement a simple energy-based VAD as a placeholder
	
	// Calculate RMS energy
	var energy float64
	for _, sample := range samples {
		energy += float64(sample) * float64(sample)
	}
	energy = math.Sqrt(energy / float64(len(samples)))
	
	// Normalize energy to 0-1 range (assuming max energy around 10000)
	normalizedEnergy := energy / 10000.0
	if normalizedEnergy > 1.0 {
		normalizedEnergy = 1.0
	}
	
	// Add some noise and variation to simulate real VAD
	variation := 0.1 * (float64(p.totalWindows%10) - 5) / 5 // ±0.1 variation
	probability := normalizedEnergy + variation
	
	if probability < 0 {
		probability = 0
	}
	if probability > 1 {
		probability = 1
	}
	
	return float32(probability)
}

// ProcessContinuous processes multiple overlapping windows and returns voice segments
func (p *Processor) ProcessContinuous(windows []*AudioWindow) ([]*VoiceSegment, error) {
	if len(windows) == 0 {
		return nil, nil
	}
	
	segments := make([]*VoiceSegment, 0)
	var currentSegment *VoiceSegment
	
	for i, window := range windows {
		result, err := p.Process(window.Samples)
		if err != nil {
			return nil, fmt.Errorf("failed to process window %d: %w", i, err)
		}
		
		if result.HasVoice {
			if currentSegment == nil {
				// Start new voice segment
				currentSegment = &VoiceSegment{
					StartTime:  window.Timestamp,
					StartSeq:   window.StartSeq,
					Confidence: result.Confidence,
				}
			} else {
				// Continue current segment
				currentSegment.Confidence = (currentSegment.Confidence + result.Confidence) / 2
			}
		} else {
			if currentSegment != nil {
				// End current voice segment
				currentSegment.EndTime = window.Timestamp
				currentSegment.EndSeq = window.EndSeq
				currentSegment.Duration = currentSegment.EndTime.Sub(currentSegment.StartTime)
				
				segments = append(segments, currentSegment)
				currentSegment = nil
			}
		}
	}
	
	// Close any remaining segment
	if currentSegment != nil {
		lastWindow := windows[len(windows)-1]
		currentSegment.EndTime = lastWindow.Timestamp
		currentSegment.EndSeq = lastWindow.EndSeq
		currentSegment.Duration = currentSegment.EndTime.Sub(currentSegment.StartTime)
		segments = append(segments, currentSegment)
	}
	
	return segments, nil
}

// GetStats returns current processor statistics
func (p *Processor) GetStats() ProcessorStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	voicePercentage := float64(0)
	if p.totalWindows > 0 {
		voicePercentage = float64(p.voiceWindows) / float64(p.totalWindows) * 100
	}
	
	return ProcessorStats{
		ModelPath:       p.modelPath,
		IsInitialized:   p.isInitialized,
		TotalWindows:    p.totalWindows,
		VoiceWindows:    p.voiceWindows,
		VoicePercentage: voicePercentage,
		LastProcessed:   p.lastProcessed,
		Threshold:       p.threshold,
	}
}

// UpdateThreshold updates the voice detection threshold
func (p *Processor) UpdateThreshold(threshold float32) error {
	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("threshold must be between 0 and 1, got %f", threshold)
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.threshold = threshold
	return nil
}

// Reset resets the processor state and statistics
func (p *Processor) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.totalWindows = 0
	p.voiceWindows = 0
	p.lastResult = 0
	p.lastProcessed = time.Time{}
}

// IsInitialized returns whether the processor is initialized and ready
func (p *Processor) IsInitialized() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isInitialized
}

// GetThreshold returns the current voice detection threshold
func (p *Processor) GetThreshold() float32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.threshold
}

// GetWindowSize returns the window size in samples
func (p *Processor) GetWindowSize() int {
	return p.windowSize
}

// GetOverlapSize returns the overlap size in samples
func (p *Processor) GetOverlapSize() int {
	return p.overlapSize
}

// AudioWindow represents a window of audio samples (imported from audio package)
type AudioWindow struct {
	Samples   []int16
	StartSeq  uint32
	EndSeq    uint32
	Timestamp time.Time
	HasGaps   bool
} 