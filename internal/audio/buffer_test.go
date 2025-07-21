package audio

import (
	"testing"
	"time"

	"github.com/skypro1111/tlv-audio-service/internal/protocol"
)

func TestNewBuffer(t *testing.T) {
	streamID := uint32(12345)
	direction := uint8(protocol.DirectionRX)
	sampleRate := 8000

	buffer := NewBuffer(streamID, direction, sampleRate)

	if buffer == nil {
		t.Fatal("NewBuffer returned nil")
	}

	if buffer.GetStreamID() != streamID {
		t.Errorf("Expected stream ID %d, got %d", streamID, buffer.GetStreamID())
	}

	if buffer.GetDirection() != direction {
		t.Errorf("Expected direction %d, got %d", direction, buffer.GetDirection())
	}

	if buffer.sampleRate != sampleRate {
		t.Errorf("Expected sample rate %d, got %d", sampleRate, buffer.sampleRate)
	}

	if buffer.GetLastSequence() != 0 {
		t.Errorf("Expected initial sequence 0, got %d", buffer.GetLastSequence())
	}

	if buffer.Size() != 0 {
		t.Errorf("Expected initial size 0, got %d", buffer.Size())
	}
}

func TestAddAudioData(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	initialTime := buffer.GetLastUpdate()
	
	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)
	
	// Create test audio data (160 samples = 320 bytes)
	audioData := make([]byte, 320)
	for i := 0; i < len(audioData); i += 2 {
		audioData[i] = byte(i % 256)     // Low byte
		audioData[i+1] = byte(i/2 % 256) // High byte
	}
	sequence := uint32(100)

	err := buffer.AddAudioData(sequence, audioData)
	if err != nil {
		t.Errorf("Failed to add audio data: %v", err)
	}

	if buffer.GetLastSequence() != sequence {
		t.Errorf("Expected sequence %d, got %d", sequence, buffer.GetLastSequence())
	}

	// Check that last update time was updated
	if !buffer.GetLastUpdate().After(initialTime) {
		t.Error("Expected last update time to be updated")
	}

	// Check that audio data was converted to samples
	if buffer.Size() != 160 {
		t.Errorf("Expected 160 samples, got %d", buffer.Size())
	}

	// Check total packets
	if buffer.GetTotalPackets() != 1 {
		t.Errorf("Expected 1 total packet, got %d", buffer.GetTotalPackets())
	}
}

func TestSequenceOrdering(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	// Create test audio data
	createAudioData := func(seq uint32) []byte {
		data := make([]byte, 160) // 80 samples
		for i := 0; i < len(data); i += 2 {
			data[i] = byte(seq % 256)
			data[i+1] = byte(seq / 256)
		}
		return data
	}

	// Add packets out of order: 1, 3, 2, 4
	err := buffer.AddAudioData(1, createAudioData(1))
	if err != nil {
		t.Errorf("Failed to add packet 1: %v", err)
	}

	err = buffer.AddAudioData(3, createAudioData(3))
	if err != nil {
		t.Errorf("Failed to add packet 3: %v", err)
	}

	// At this point, packet 2 is missing, so packet 3 should be buffered
	if buffer.Size() != 80 { // Only packet 1 should be in the main buffer
		t.Errorf("Expected 80 samples after packets 1,3, got %d", buffer.Size())
	}

	// Add packet 2 - this should trigger processing of packet 3
	err = buffer.AddAudioData(2, createAudioData(2))
	if err != nil {
		t.Errorf("Failed to add packet 2: %v", err)
	}

	// Now all three packets should be in order
	if buffer.Size() != 240 { // 3 packets * 80 samples each
		t.Errorf("Expected 240 samples after reordering, got %d", buffer.Size())
	}

	if buffer.GetLastSequence() != 3 {
		t.Errorf("Expected last sequence 3, got %d", buffer.GetLastSequence())
	}

	// Add packet 4
	err = buffer.AddAudioData(4, createAudioData(4))
	if err != nil {
		t.Errorf("Failed to add packet 4: %v", err)
	}

	if buffer.Size() != 320 { // 4 packets * 80 samples each
		t.Errorf("Expected 320 samples after packet 4, got %d", buffer.Size())
	}
}

func TestPacketLossDetection(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	audioData := make([]byte, 160)
	
	// Add packet 1
	buffer.AddAudioData(1, audioData)
	
	// Add packet 30 (creates a large gap, should mark packets 2-29 as lost)
	buffer.AddAudioData(30, audioData)
	
	stats := buffer.GetStats()
	if stats.LostPackets == 0 {
		t.Error("Expected some lost packets to be detected")
	}
	
	if stats.LossRate == 0 {
		t.Error("Expected non-zero loss rate")
	}
	
	t.Logf("Detected %d lost packets (%.2f%% loss rate)", stats.LostPackets, stats.LossRate)
}

func TestAudioWindowExtraction(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	// Add enough audio data for multiple windows
	audioData := make([]byte, 320) // 160 samples
	for i := 0; i < 10; i++ {
		err := buffer.AddAudioData(uint32(i+1), audioData)
		if err != nil {
			t.Fatalf("Failed to add audio data %d: %v", i+1, err)
		}
	}
	
	// Should have enough data for windows
	availableWindows := buffer.GetAvailableWindows()
	if availableWindows == 0 {
		t.Error("Expected at least one available window")
	}
	
	// Extract first window
	window, err := buffer.GetAudioWindow(0)
	if err != nil {
		t.Errorf("Failed to get audio window: %v", err)
	}
	
	if len(window.Samples) != 512 {
		t.Errorf("Expected 512 samples in window, got %d", len(window.Samples))
	}
	
	if window.StartSeq == 0 && window.EndSeq == 0 {
		t.Error("Expected non-zero sequence numbers in window")
	}
	
	if window.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp in window")
	}
}

func TestBufferTrimming(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	// Add lots of audio data
	audioData := make([]byte, 320) // 160 samples per packet
	for i := 0; i < 50; i++ {
		buffer.AddAudioData(uint32(i+1), audioData)
	}
	
	initialSize := buffer.Size()
	
	// Trim to keep only 10 windows worth of data
	buffer.TrimProcessedAudio(10)
	
	finalSize := buffer.Size()
	
	if finalSize >= initialSize {
		t.Errorf("Expected buffer to be trimmed: initial=%d, final=%d", initialSize, finalSize)
	}
	
	// Should still be able to extract windows
	if buffer.GetAvailableWindows() == 0 {
		t.Error("Expected to still have available windows after trimming")
	}
}

func TestBufferStats(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	audioData := make([]byte, 160)
	
	// Add packets in sequence without gaps for predictable behavior
	buffer.AddAudioData(1, audioData)
	buffer.AddAudioData(2, audioData)
	buffer.AddAudioData(3, audioData)
	buffer.AddAudioData(4, audioData)
	
	stats := buffer.GetStats()
	
	if stats.StreamID != 12345 {
		t.Errorf("Expected stream ID 12345, got %d", stats.StreamID)
	}
	
	if stats.Direction != protocol.DirectionRX {
		t.Errorf("Expected direction %d, got %d", protocol.DirectionRX, stats.Direction)
	}
	
	if stats.TotalPackets != 4 {
		t.Errorf("Expected 4 total packets, got %d", stats.TotalPackets)
	}
	
	if stats.BufferSize <= 0 {
		t.Errorf("Expected positive buffer size, got %d", stats.BufferSize)
	}
	
	if stats.LastSequence != 4 {
		t.Errorf("Expected last sequence 4, got %d", stats.LastSequence)
	}
}

func TestInvalidAudioData(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	// Test odd-length data (should fail)
	oddData := make([]byte, 159) // Odd number of bytes
	err := buffer.AddAudioData(1, oddData)
	if err == nil {
		t.Error("Expected error for odd-length audio data")
	}
	
	// Test empty data
	err = buffer.AddAudioData(2, []byte{})
	if err != nil {
		t.Errorf("Unexpected error for empty audio data: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	// Test concurrent access
	done := make(chan bool)
	
	// Concurrent readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = buffer.GetStreamID()
				_ = buffer.GetDirection()
				_ = buffer.GetLastSequence()
				_ = buffer.Size()
				_ = buffer.GetLastUpdate()
				_ = buffer.GetTotalPackets()
				_ = buffer.GetLostPackets()
				_ = buffer.GetPacketLossRate()
				_ = buffer.GetAvailableWindows()
				_ = buffer.GetStats()
			}
			done <- true
		}()
	}
	
	// Concurrent writers
	for i := 0; i < 5; i++ {
		go func(id int) {
			audioData := make([]byte, 160)
			for j := 0; j < 100; j++ {
				seq := uint32(id*1000 + j)
				buffer.AddAudioData(seq, audioData)
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify final state
	finalStats := buffer.GetStats()
	if finalStats.TotalPackets == 0 {
		t.Error("Expected non-zero total packets after concurrent writes")
	}
}

func TestWindowExtractionError(t *testing.T) {
	buffer := NewBuffer(12345, protocol.DirectionRX, 8000)
	
	// Try to extract window when there's not enough data
	_, err := buffer.GetAudioWindow(0)
	if err == nil {
		t.Error("Expected error when extracting window without enough data")
	}
	
	// Add some data but not enough for a full window
	smallData := make([]byte, 100) // Only 50 samples
	buffer.AddAudioData(1, smallData)
	
	_, err = buffer.GetAudioWindow(0)
	if err == nil {
		t.Error("Expected error when extracting window without enough data")
	}
} 