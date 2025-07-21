package audio

import (
	"fmt"
	"sync"
	"time"
	
	"github.com/skypro1111/tlv-audio-service/internal/vad"
)

// ChunkState represents the current state of the chunking process
type ChunkState int

const (
	StateIdle ChunkState = iota
	StateCollecting
	StateWaitingSilence
)

// AudioChunk represents a processed audio chunk ready for transcription
type AudioChunk struct {
	StreamID    uint32        `json:"stream_id"`
	Direction   uint8         `json:"direction"`
	CallerID    string        `json:"caller_id,omitempty"`
	ChunkID     string        `json:"chunk_id"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Duration    time.Duration `json:"duration"`
	SampleRate  int           `json:"sample_rate"`
	Samples     []int16       `json:"-"` // Audio data (not serialized)
	AudioData   []byte        `json:"-"` // Encoded audio data (WAV/FLAC)
	Format      string        `json:"format"` // "raw", "wav" or "flac"
	Confidence  float32       `json:"confidence"` // Average VAD confidence
	StartSeq    uint32        `json:"start_seq"`
	EndSeq      uint32        `json:"end_seq"`
}

// ChunkingConfig contains configuration for the chunking process
type ChunkingConfig struct {
	MinDuration        time.Duration
	MaxDuration        time.Duration
	MinSpeechDuration  time.Duration
	MinSilenceDuration time.Duration
	SampleRate         int
	Format             string // "raw", "wav" or "flac"
}

// Chunker manages the audio chunking process for a stream
type Chunker struct {
	config         ChunkingConfig
	state          ChunkState
	currentChunk   *AudioChunk
	
	// Timing tracking
	speechStartTime    time.Time
	lastSpeechTime     time.Time
	silenceStartTime   time.Time
	chunkStartTime     time.Time
	
	// Audio segment tracking (byte positions in buffer)
	segmentStartByte   int
	segmentEndByte     int
	confidenceSum      float32
	confidenceCount    int
	
	// Sequence tracking
	startSeq       uint32
	endSeq         uint32
	
	// Statistics
	chunksCreated  uint64
	totalDuration  time.Duration
	
	mu sync.RWMutex
}

// ChunkerStats represents chunker statistics
type ChunkerStats struct {
	State         string        `json:"state"`
	ChunksCreated uint64        `json:"chunks_created"`
	TotalDuration time.Duration `json:"total_duration"`
	CurrentSize   int           `json:"current_chunk_samples"`
	AvgChunkSize  float64       `json:"avg_chunk_duration_sec"`
}

// NewChunker creates a new audio chunker
func NewChunker(config ChunkingConfig) *Chunker {
	return &Chunker{
		config: config,
		state:  StateIdle,
	}
}

// ProcessVADResult processes a VAD result and potentially creates audio chunks
func (c *Chunker) ProcessVADResult(streamID uint32, direction uint8, callerID string, 
	window *AudioWindow, vadResult *vad.VADResult, buffer *Buffer) (*AudioChunk, error) {
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	
	// Track confidence
	c.confidenceSum += vadResult.Confidence
	c.confidenceCount++
	
	// Calculate byte positions for this window (window samples * 2 bytes per sample)
	windowStartByte := int(window.StartSeq-buffer.GetLastSequence()) * 160 * 2 // Approximate
	windowEndByte := int(window.EndSeq-buffer.GetLastSequence()) * 160 * 2
	
	// Update segment end position
	c.segmentEndByte = windowEndByte
	c.endSeq = window.EndSeq
	
	switch c.state {
	case StateIdle:
		if vadResult.HasVoice {
			// Start collecting a new chunk
			c.startNewChunk(streamID, direction, callerID, window, now, windowStartByte)
			c.state = StateCollecting
		}
		
	case StateCollecting:
		if vadResult.HasVoice {
			// Continue collecting - reset silence timer
			c.lastSpeechTime = now
			c.silenceStartTime = time.Time{} // Reset silence timer
			
			// Check for max duration timeout even during voice
			chunkDuration := now.Sub(c.chunkStartTime)
			if chunkDuration >= c.config.MaxDuration {
				chunk := c.finalizeChunk(now, buffer)
				c.resetForNextChunk()
				return chunk, nil
			}
		} else {
			// Silence detected - check if we should wait or finalize
			if c.silenceStartTime.IsZero() {
				c.silenceStartTime = now
			}
			
			silenceDuration := now.Sub(c.silenceStartTime)
			chunkDuration := now.Sub(c.chunkStartTime)
			
			// Check if we have enough silence to split or if chunk is too long
			if silenceDuration >= c.config.MinSilenceDuration || 
			   chunkDuration >= c.config.MaxDuration {
				
				speechDuration := c.lastSpeechTime.Sub(c.speechStartTime)
				
				// Only create chunk if we have enough speech
				if speechDuration >= c.config.MinSpeechDuration && 
				   chunkDuration >= c.config.MinDuration {
					
					chunk := c.finalizeChunk(now, buffer)
					c.resetForNextChunk()
					return chunk, nil
				} else {
					// Not enough speech yet, but silence detected - wait for more
					c.state = StateWaitingSilence
				}
			}
		}
		
	case StateWaitingSilence:
		if !vadResult.HasVoice {
			silenceDuration := now.Sub(c.silenceStartTime)
			if silenceDuration >= c.config.MinSilenceDuration {
				chunk := c.finalizeChunk(now, buffer)
				c.resetForNextChunk()
				return chunk, nil
			}
		} else {
			// Speech resumed, go back to collecting
			c.state = StateCollecting
			c.lastSpeechTime = now
			c.silenceStartTime = time.Time{}
		}
	}
	
	// Check for max duration timeout
	if !c.chunkStartTime.IsZero() {
		chunkDuration := now.Sub(c.chunkStartTime)
		if chunkDuration >= c.config.MaxDuration {
			chunk := c.finalizeChunk(now, buffer)
			c.resetForNextChunk()
			return chunk, nil
		}
	}
	
	return nil, nil
}

// startNewChunk initializes a new chunk collection
func (c *Chunker) startNewChunk(streamID uint32, direction uint8, callerID string, 
	window *AudioWindow, now time.Time, windowStartByte int) {
	
	c.currentChunk = &AudioChunk{
		StreamID:   streamID,
		Direction:  direction,
		CallerID:   callerID,
		ChunkID:    fmt.Sprintf("%d_%d_%d", streamID, direction, now.Unix()),
		StartTime:  now,
		SampleRate: c.config.SampleRate,
		Format:     c.config.Format,
		StartSeq:   window.StartSeq,
	}
	
	c.speechStartTime = now
	c.lastSpeechTime = now
	c.chunkStartTime = now
	c.silenceStartTime = time.Time{}
	c.startSeq = window.StartSeq
	c.segmentStartByte = windowStartByte
}

// finalizeChunk creates a complete audio chunk
func (c *Chunker) finalizeChunk(endTime time.Time, buffer *Buffer) *AudioChunk {
	if c.currentChunk == nil {
		return nil
	}
	
	// Finalize chunk metadata
	c.currentChunk.EndTime = endTime
	c.currentChunk.Duration = endTime.Sub(c.currentChunk.StartTime)
	c.currentChunk.EndSeq = c.endSeq
	
	// Calculate average confidence
	if c.confidenceCount > 0 {
		c.currentChunk.Confidence = c.confidenceSum / float32(c.confidenceCount)
	}
	
	// Extract raw audio segment from buffer
	var err error
	if c.segmentStartByte >= 0 && c.segmentEndByte > c.segmentStartByte {
		c.currentChunk.AudioData, err = buffer.GetRawAudioSegment(c.segmentStartByte, c.segmentEndByte)
		if err != nil {
			// Log error but continue
			c.currentChunk.AudioData = nil
		}
		
		// Convert raw bytes to samples for backward compatibility (if needed)
		if c.currentChunk.AudioData != nil {
			numSamples := len(c.currentChunk.AudioData) / 2
			c.currentChunk.Samples = make([]int16, numSamples)
			for i := 0; i < numSamples; i++ {
				c.currentChunk.Samples[i] = int16(c.currentChunk.AudioData[i*2]) | 
					int16(c.currentChunk.AudioData[i*2+1])<<8
			}
		}
	}
	
	// Update statistics
	c.chunksCreated++
	c.totalDuration += c.currentChunk.Duration
	
	chunk := c.currentChunk
	c.currentChunk = nil
	
	return chunk
}

// resetForNextChunk resets the chunker state for the next chunk
func (c *Chunker) resetForNextChunk() {
	c.state = StateIdle
	c.confidenceSum = 0
	c.confidenceCount = 0
	c.speechStartTime = time.Time{}
	c.lastSpeechTime = time.Time{}
	c.silenceStartTime = time.Time{}
	c.chunkStartTime = time.Time{}
	c.startSeq = 0
	c.endSeq = 0
	c.segmentStartByte = 0
	c.segmentEndByte = 0
}

// ForceFinalize forces the current chunk to be finalized (used on stream timeout)
func (c *Chunker) ForceFinalize(buffer *Buffer) *AudioChunk {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.state == StateIdle || c.currentChunk == nil {
		return nil
	}
	
	now := time.Now()
	chunk := c.finalizeChunk(now, buffer)
	c.resetForNextChunk()
	
	return chunk
}

// GetStats returns current chunker statistics
func (c *Chunker) GetStats() ChunkerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	stateStr := "idle"
	switch c.state {
	case StateCollecting:
		stateStr = "collecting"
	case StateWaitingSilence:
		stateStr = "waiting_silence"
	}
	
	avgDuration := float64(0)
	if c.chunksCreated > 0 {
		avgDuration = c.totalDuration.Seconds() / float64(c.chunksCreated)
	}
	
	return ChunkerStats{
		State:         stateStr,
		ChunksCreated: c.chunksCreated,
		TotalDuration: c.totalDuration,
		CurrentSize:   (c.segmentEndByte - c.segmentStartByte) / 2, // Convert bytes to samples
		AvgChunkSize:  avgDuration,
	}
}

// GetCurrentChunkDuration returns the duration of the current chunk being collected
func (c *Chunker) GetCurrentChunkDuration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if c.chunkStartTime.IsZero() {
		return 0
	}
	
	return time.Since(c.chunkStartTime)
}

// IsIdle returns whether the chunker is currently idle
func (c *Chunker) IsIdle() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.state == StateIdle
}

// HasPendingChunk returns whether there's a chunk currently being collected
func (c *Chunker) HasPendingChunk() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.currentChunk != nil
}

// Note: AudioWindow and VADResult types are imported from their respective packages
// AudioWindow is defined in buffer.go
// VADResult is defined in the vad package 