package audio

import (
	"fmt"
	"sync"
	"time"
)

// Buffer represents an enhanced audio buffer for a specific stream and direction
// with sequence reordering, packet loss detection, and PCM audio processing
type Buffer struct {
	streamID     uint32
	direction    uint8
	sampleRate   int          // 8000 Hz
	
	// Audio data storage
	rawAudioData []byte       // Raw audio bytes
	
	// Sequence tracking
	lastSeq      uint32       // Last processed sequence number
	expectedSeq  uint32       // Next expected sequence number
	rawSeqBuffer map[uint32][]byte   // Raw bytes buffer
	
	// Packet loss tracking
	lostPackets  map[uint32]bool     // Lost sequence numbers
	maxGap       uint32              // Maximum sequence gap to wait for
	
	// Timing and metadata
	lastUpdate   time.Time    // Last time buffer was updated
	totalPackets uint32       // Total packets processed
	lostCount    uint32       // Total lost packets
	
	// Processing windows for VAD
	windowSize   int          // VAD window size (512 samples = 64ms at 8kHz)
	overlapSize  int          // VAD overlap size (256 samples = 50% overlap)
	
	mu           sync.RWMutex
}

// AudioWindow represents a window of audio samples for VAD processing
type AudioWindow struct {
	Samples    []int16   // PCM samples
	StartSeq   uint32    // Starting sequence number
	EndSeq     uint32    // Ending sequence number
	Timestamp  time.Time // When the window was created
	HasGaps    bool      // Whether this window has missing packets
}

// BufferStats represents buffer statistics for monitoring
type BufferStats struct {
	StreamID     uint32  `json:"stream_id"`
	Direction    uint8   `json:"direction"`
	TotalPackets uint32  `json:"total_packets"`
	LostPackets  uint32  `json:"lost_packets"`
	LossRate     float64 `json:"loss_rate"`
	BufferSize   int     `json:"buffer_size_samples"`
	PendingSeqs  int     `json:"pending_sequences"`
	LastSequence uint32  `json:"last_sequence"`
}

// NewBuffer creates a new enhanced audio buffer
func NewBuffer(streamID uint32, direction uint8, sampleRate int) *Buffer {
	return &Buffer{
		streamID:     streamID,
		direction:    direction,
		sampleRate:   sampleRate,
		rawAudioData: make([]byte, 0, sampleRate*4),  // Pre-allocate for 2 seconds of 16-bit samples
		rawSeqBuffer: make(map[uint32][]byte),
		lostPackets:  make(map[uint32]bool),
		lastUpdate:   time.Now(),
		maxGap:       20, // Wait for up to 20 missing packets
		windowSize:   512, // 64ms at 8kHz
		overlapSize:  256, // 50% overlap
	}
}



// AddAudioData adds PCM audio data to the buffer with sequence handling
func (b *Buffer) AddAudioData(sequence uint32, rawData []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if len(rawData)%2 != 0 {
		return fmt.Errorf("audio data length must be even (got %d bytes)", len(rawData))
	}
	
	// Store raw data directly without conversion
	// Update metadata
	b.lastUpdate = time.Now()
	b.totalPackets++
	
	// Handle sequence ordering with raw bytes
	return b.addRawBytesWithSequence(sequence, rawData)
}



// markMissingAsLost marks a range of sequence numbers as lost
func (b *Buffer) markMissingAsLost(start, end uint32) {
	for seq := start; seq <= end; seq++ {
		if _, buffered := b.rawSeqBuffer[seq]; !buffered {
			b.lostPackets[seq] = true
			b.lostCount++
		}
	}
}

// cleanupOldLostPackets removes very old lost packet tracking
func (b *Buffer) cleanupOldLostPackets() {
	cutoff := b.lastSeq - 100 // Keep tracking for last 100 packets
	for seq := range b.lostPackets {
		if seq < cutoff {
			delete(b.lostPackets, seq)
		}
	}
}

// addRawBytesWithSequence handles sequence-ordered addition of raw bytes
func (b *Buffer) addRawBytesWithSequence(sequence uint32, rawData []byte) error {
	// Initialize expected sequence on first packet
	if b.totalPackets == 1 {
		b.expectedSeq = sequence
		b.lastSeq = sequence - 1
	}
	
	if sequence == b.expectedSeq {
		// Perfect order - add directly
		b.rawAudioData = append(b.rawAudioData, rawData...)
		b.lastSeq = sequence
		b.expectedSeq = sequence + 1
		
		// Check if we can process any buffered out-of-order packets
		b.processBufferedRawPackets()
		
	} else if sequence > b.expectedSeq {
		// Future packet - buffer it
		b.rawSeqBuffer[sequence] = make([]byte, len(rawData))
		copy(b.rawSeqBuffer[sequence], rawData)
		
		// Mark missing packets as lost if gap is too large
		if sequence-b.expectedSeq > b.maxGap {
			b.markMissingAsLost(b.expectedSeq, sequence-1)
			b.expectedSeq = sequence
		}
		
	} else if sequence > b.lastSeq {
		// Late but not too late - insert it
		b.rawSeqBuffer[sequence] = make([]byte, len(rawData))
		copy(b.rawSeqBuffer[sequence], rawData)
		b.processBufferedRawPackets()
		
	} else {
		// Very old packet or duplicate - ignore
		return fmt.Errorf("ignoring old/duplicate packet: seq=%d, lastSeq=%d", sequence, b.lastSeq)
	}
	
	// Cleanup old lost packet tracking
	b.cleanupOldLostPackets()
	
	return nil
}

// processBufferedRawPackets processes any consecutive buffered raw packets
func (b *Buffer) processBufferedRawPackets() {
	for {
		rawData, exists := b.rawSeqBuffer[b.expectedSeq]
		if !exists {
			break
		}
		
		// Add the buffered raw data
		b.rawAudioData = append(b.rawAudioData, rawData...)
		delete(b.rawSeqBuffer, b.expectedSeq)
		
		// Remove from lost packets if it was marked as lost
		delete(b.lostPackets, b.expectedSeq)
		
		b.lastSeq = b.expectedSeq
		b.expectedSeq++
	}
}

// GetAudioWindow extracts a window of audio samples for VAD processing
func (b *Buffer) GetAudioWindow(windowIndex int) (*AudioWindow, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	// Calculate byte positions (we always work with raw bytes now)
	startByte := windowIndex * (b.windowSize - b.overlapSize) * 2 // 2 bytes per sample
	endByte := startByte + b.windowSize*2
	
	if endByte > len(b.rawAudioData) {
		return nil, fmt.Errorf("not enough raw audio data: need %d bytes, have %d", 
			endByte, len(b.rawAudioData))
	}
	
	// Convert raw bytes to samples for VAD processing
	samples := make([]int16, b.windowSize)
	for i := 0; i < b.windowSize; i++ {
		byteOffset := startByte + i*2
		// Convert bytes to int16 (little-endian for PCM-16)
		samples[i] = int16(b.rawAudioData[byteOffset]) | int16(b.rawAudioData[byteOffset+1])<<8
	}
	
	return &AudioWindow{
		Samples:   samples,
		StartSeq:  b.calculateSequenceForByte(startByte),
		EndSeq:    b.calculateSequenceForByte(endByte - 2),
		Timestamp: time.Now(),
		HasGaps:   b.hasGapsInRawRange(startByte, endByte),
	}, nil
}

// GetAvailableWindows returns the number of complete windows available for processing
func (b *Buffer) GetAvailableWindows() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	requiredBytes := b.windowSize * 2 // 2 bytes per sample
	if len(b.rawAudioData) < requiredBytes {
		return 0
	}
	
	// Calculate how many complete windows we can extract
	availableBytes := len(b.rawAudioData) - requiredBytes
	windowStepBytes := (b.windowSize - b.overlapSize) * 2
	
	return (availableBytes / windowStepBytes) + 1
}

// TrimProcessedAudio removes processed audio data to prevent unlimited growth
func (b *Buffer) TrimProcessedAudio(keepWindows int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if keepWindows <= 0 {
		keepWindows = 5 // Keep at least 5 windows by default
	}
	
	// Calculate how many bytes to keep (samples * 2 bytes per sample)
	windowStepBytes := (b.windowSize - b.overlapSize) * 2
	keepBytes := keepWindows * windowStepBytes
	
	if len(b.rawAudioData) > keepBytes*2 { // Only trim if we have much more
		bytesToRemove := len(b.rawAudioData) - keepBytes
		
		// Shift audio data
		copy(b.rawAudioData, b.rawAudioData[bytesToRemove:])
		b.rawAudioData = b.rawAudioData[:keepBytes]
	}
}

// calculateSequenceForSample estimates the sequence number for a given sample position
func (b *Buffer) calculateSequenceForSample(samplePos int) uint32 {
	// This is an approximation - in practice, you'd need to track exact mapping
	// For now, assume roughly 160 samples per packet (20ms at 8kHz) = 320 bytes
	bytesPerPacket := 320
	bytePos := samplePos * 2 // Convert sample position to byte position
	return b.lastSeq - uint32(len(b.rawAudioData)-bytePos)/uint32(bytesPerPacket)
}

// hasGapsInRange checks if there are missing packets in the given sample range
func (b *Buffer) hasGapsInRange(startSample, endSample int) bool {
	// Simplified check - in practice, you'd track exact sample-to-sequence mapping
	return len(b.lostPackets) > 0
}

// calculateSequenceForByte estimates the sequence number for a given byte position
func (b *Buffer) calculateSequenceForByte(bytePos int) uint32 {
	// Assume roughly 160 samples per packet (20ms at 8kHz) = 320 bytes
	bytesPerPacket := 320
	return b.lastSeq - uint32(len(b.rawAudioData)-bytePos)/uint32(bytesPerPacket)
}

// hasGapsInRawRange checks if there are missing packets in the given byte range
func (b *Buffer) hasGapsInRawRange(startByte, endByte int) bool {
	// Simplified check - in practice, you'd track exact byte-to-sequence mapping
	return len(b.lostPackets) > 0
}

// GetRawAudioData returns the accumulated raw audio data
func (b *Buffer) GetRawAudioData() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	return b.rawAudioData
}

// GetRawAudioSegment extracts a segment of raw audio data by byte positions
func (b *Buffer) GetRawAudioSegment(startByte, endByte int) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if startByte < 0 || endByte > len(b.rawAudioData) || startByte >= endByte {
		return nil, fmt.Errorf("invalid segment range: start=%d, end=%d, available=%d", 
			startByte, endByte, len(b.rawAudioData))
	}
	
	// Copy the segment
	segment := make([]byte, endByte-startByte)
	copy(segment, b.rawAudioData[startByte:endByte])
	
	return segment, nil
}

// GetStats returns current buffer statistics
func (b *Buffer) GetStats() BufferStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	lossRate := float64(0)
	if b.totalPackets > 0 {
		lossRate = float64(b.lostCount) / float64(b.totalPackets) * 100
	}
	
	return BufferStats{
		StreamID:     b.streamID,
		Direction:    b.direction,
		TotalPackets: b.totalPackets,
		LostPackets:  b.lostCount,
		LossRate:     lossRate,
		BufferSize:   len(b.rawAudioData) / 2, // Convert bytes to samples for compatibility
		PendingSeqs:  len(b.rawSeqBuffer),
		LastSequence: b.lastSeq,
	}
}

// GetStreamID returns the stream ID for this buffer
func (b *Buffer) GetStreamID() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.streamID
}

// GetDirection returns the direction for this buffer
func (b *Buffer) GetDirection() uint8 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.direction
}

// GetLastSequence returns the last processed sequence number
func (b *Buffer) GetLastSequence() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastSeq
}

// GetLastUpdate returns the time of the last buffer update
func (b *Buffer) GetLastUpdate() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastUpdate
}

// Size returns the current number of samples in the buffer
func (b *Buffer) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.rawAudioData) / 2 // Convert bytes to samples
}

// GetTotalPackets returns the total number of packets processed
func (b *Buffer) GetTotalPackets() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.totalPackets
}

// GetLostPackets returns the number of lost packets
func (b *Buffer) GetLostPackets() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lostCount
}

// GetPacketLossRate returns the packet loss rate as a percentage
func (b *Buffer) GetPacketLossRate() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if b.totalPackets == 0 {
		return 0
	}
	return float64(b.lostCount) / float64(b.totalPackets) * 100
} 