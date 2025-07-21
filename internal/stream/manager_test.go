package stream

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/skypro1111/tlv-audio-service/internal/audio"
	"github.com/skypro1111/tlv-audio-service/internal/protocol"
	"github.com/skypro1111/tlv-audio-service/internal/transcription"
)

// createTestManagerConfig creates a test configuration for the manager
func createTestManagerConfig() ManagerConfig {
	return ManagerConfig{
		VADConfig: VADConfig{
			ModelPath:          "./test_model.onnx", // Fake path for testing
			Threshold:          0.5,
			WindowSize:         512,
			SampleRate:         8000,
			MinSpeechDuration:  500 * time.Millisecond,
			MinSilenceDuration: 300 * time.Millisecond,
		},
		ChunkingConfig: audio.ChunkingConfig{
			MinDuration:        1 * time.Second,
			MaxDuration:        30 * time.Second,
			MinSpeechDuration:  500 * time.Millisecond,
			MinSilenceDuration: 300 * time.Millisecond,
		},
		TranscriptionConfig: transcription.Config{
			Endpoint:      "http://localhost:9999/test",
			APIKey:        "test-key",
			Timeout:       30 * time.Second,
			MaxRetries:    3,
			MaxConcurrent: 5,
			OutputFormat:  "json",
		},
	}
}

func TestNewManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	timeout := 60 * time.Second
	config := createTestManagerConfig()

	mgr, err := NewManager(logger, timeout, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, mgr.timeout)
	}

	if mgr.GetActiveSessionCount() != 0 {
		t.Errorf("Expected 0 active sessions, got %d", mgr.GetActiveSessionCount())
	}
}

func TestCreateSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	// Create test signaling payload
	signaling := createTestSignalingPayload()

	// Create session
	session, err := mgr.CreateSession(12345, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if session.ID != 12345 {
		t.Errorf("Expected stream ID 12345, got %d", session.ID)
	}

	if session.CallerID != "John Doe" {
		t.Errorf("Expected caller ID 'John Doe', got '%s'", session.CallerID)
	}

	if mgr.GetActiveSessionCount() != 1 {
		t.Errorf("Expected 1 active session, got %d", mgr.GetActiveSessionCount())
	}
}

func TestCreateSessionDuplicate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	signaling := createTestSignalingPayload()

	// Create session first time
	session1, err := mgr.CreateSession(12345, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create session second time (should update existing)
	signaling2 := createTestSignalingPayload()
	copy(signaling2.CallerID[:], []byte("Jane Smith"))

	session2, err := mgr.CreateSession(12345, signaling2, 8000)
	if err != nil {
		t.Fatalf("Failed to create/update session: %v", err)
	}

	// Should return the same session instance
	if session1 != session2 {
		t.Error("Expected same session instance for duplicate stream ID")
	}

	// Should have updated caller ID
	if session2.CallerID != "Jane Smith" {
		t.Errorf("Expected updated caller ID 'Jane Smith', got '%s'", session2.CallerID)
	}

	// Should still have only 1 session
	if mgr.GetActiveSessionCount() != 1 {
		t.Errorf("Expected 1 active session, got %d", mgr.GetActiveSessionCount())
	}
}

func TestGetSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	signaling := createTestSignalingPayload()
	originalSession, err := mgr.CreateSession(12345, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Test getting existing session
	session, exists := mgr.GetSession(12345)
	if !exists {
		t.Error("Expected session to exist")
	}

	if session != originalSession {
		t.Error("Expected same session instance")
	}

	// Test getting non-existent session
	_, exists = mgr.GetSession(99999)
	if exists {
		t.Error("Expected session to not exist")
	}
}

func TestUpdateActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	signaling := createTestSignalingPayload()
	session, err := mgr.CreateSession(12345, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	originalActivity := session.LastActivity

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update activity
	mgr.UpdateActivity(12345)

	if !session.LastActivity.After(originalActivity) {
		t.Error("Expected last activity to be updated")
	}

	// Test updating non-existent session (should not panic)
	mgr.UpdateActivity(99999)
}

func TestRemoveSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	signaling := createTestSignalingPayload()
	_, err2 := mgr.CreateSession(12345, signaling, 8000)
	if err2 != nil {
		t.Fatalf("Failed to create session: %v", err2)
	}

	if mgr.GetActiveSessionCount() != 1 {
		t.Errorf("Expected 1 active session, got %d", mgr.GetActiveSessionCount())
	}

	// Remove session
	removed := mgr.RemoveSession(12345)
	if !removed {
		t.Error("Expected session to be removed")
	}

	if mgr.GetActiveSessionCount() != 0 {
		t.Errorf("Expected 0 active sessions, got %d", mgr.GetActiveSessionCount())
	}

	// Try to remove non-existent session
	removed = mgr.RemoveSession(99999)
	if removed {
		t.Error("Expected non-existent session to not be removed")
	}
}

func TestGetAllSessions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	// Create multiple sessions
	signaling1 := createTestSignalingPayload()
	signaling2 := createTestSignalingPayload()
	copy(signaling2.CallerID[:], []byte("Jane Smith"))

	_, err2 := mgr.CreateSession(12345, signaling1, 8000)
	if err2 != nil {
		t.Fatalf("Failed to create session 1: %v", err2)
	}

	_, err3 := mgr.CreateSession(67890, signaling2, 8000)
	if err3 != nil {
		t.Fatalf("Failed to create session 2: %v", err3)
	}

	sessions := mgr.GetAllSessions()
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}

	// Verify session data
	sessionMap := make(map[uint32]*StreamSession)
	for _, session := range sessions {
		sessionMap[session.ID] = session
	}

	if _, exists := sessionMap[12345]; !exists {
		t.Error("Expected session 12345 to exist")
	}

	if _, exists := sessionMap[67890]; !exists {
		t.Error("Expected session 67890 to exist")
	}
}

func TestSessionConcurrency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	// Test concurrent session creation
	numGoroutines := 10
	numSessionsPerGoroutine := 10
	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(routineID int) {
			defer wg.Done()
			
			for j := 0; j < numSessionsPerGoroutine; j++ {
				streamID := uint32(routineID*1000 + j)
				signaling := createTestSignalingPayload()
				
				_, err := mgr.CreateSession(streamID, signaling, 8000)
				if err != nil {
					t.Errorf("Failed to create session %d: %v", streamID, err)
					return
				}

				// Immediately update activity
				mgr.UpdateActivity(streamID)
			}
		}(i)
	}

	wg.Wait()

	expectedSessions := numGoroutines * numSessionsPerGoroutine
	if mgr.GetActiveSessionCount() != expectedSessions {
		t.Errorf("Expected %d active sessions, got %d", expectedSessions, mgr.GetActiveSessionCount())
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	
	// Use very short timeout for testing
	shortTimeout := 100 * time.Millisecond
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, shortTimeout, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	// Create a session
	signaling := createTestSignalingPayload()
	_, err2 := mgr.CreateSession(12345, signaling, 8000)
	if err2 != nil {
		t.Fatalf("Failed to create session: %v", err2)
	}

	if mgr.GetActiveSessionCount() != 1 {
		t.Errorf("Expected 1 active session, got %d", mgr.GetActiveSessionCount())
	}

	// Wait for session to expire and cleanup to run
	time.Sleep(shortTimeout + 50*time.Millisecond)
	mgr.cleanupExpiredSessions() // Manually trigger cleanup

	if mgr.GetActiveSessionCount() != 0 {
		t.Errorf("Expected 0 active sessions after cleanup, got %d", mgr.GetActiveSessionCount())
	}

	// Verify session is actually removed
	_, exists := mgr.GetSession(12345)
	if exists {
		t.Error("Expected session to be removed after cleanup")
	}

	// Test that updated activity prevents cleanup
	_, err = mgr.CreateSession(67890, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Update activity before timeout
	time.Sleep(shortTimeout / 2)
	mgr.UpdateActivity(67890)
	time.Sleep(shortTimeout / 2)
	mgr.cleanupExpiredSessions()

	// Session should still exist
	if mgr.GetActiveSessionCount() != 1 {
		t.Errorf("Expected 1 active session after activity update, got %d", mgr.GetActiveSessionCount())
	}

	// Now let it expire
	time.Sleep(shortTimeout + 50*time.Millisecond)
	mgr.cleanupExpiredSessions()

	if mgr.GetActiveSessionCount() != 0 {
		t.Errorf("Expected 0 active sessions after final cleanup, got %d", mgr.GetActiveSessionCount())
	}
}

func TestSessionAddAudioData(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	signaling := createTestSignalingPayload()
	session, err := mgr.CreateSession(12345, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	audioData := []byte{0x01, 0x02, 0x03, 0x04}

	// Test adding RX audio data
	err = session.AddAudioData(protocol.DirectionRX, 100, audioData)
	if err != nil {
		t.Errorf("Failed to add RX audio data: %v", err)
	}

	// Test adding TX audio data
	err = session.AddAudioData(protocol.DirectionTX, 200, audioData)
	if err != nil {
		t.Errorf("Failed to add TX audio data: %v", err)
	}

	// Test invalid direction
	err = session.AddAudioData(0x99, 300, audioData)
	if err == nil {
		t.Error("Expected error for invalid direction")
	}

	// Verify buffers have correct sequence numbers
	if session.RXBuffer.GetLastSequence() != 100 {
		t.Errorf("Expected RX last sequence 100, got %d", session.RXBuffer.GetLastSequence())
	}

	if session.TXBuffer.GetLastSequence() != 200 {
		t.Errorf("Expected TX last sequence 200, got %d", session.TXBuffer.GetLastSequence())
	}
}

func TestSessionGetBuffer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	signaling := createTestSignalingPayload()
	session, err := mgr.CreateSession(12345, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Test getting RX buffer
	rxBuffer, err := session.GetBuffer(protocol.DirectionRX)
	if err != nil {
		t.Errorf("Failed to get RX buffer: %v", err)
	}

	if rxBuffer != session.RXBuffer {
		t.Error("Expected RX buffer to match session RX buffer")
	}

	// Test getting TX buffer
	txBuffer, err := session.GetBuffer(protocol.DirectionTX)
	if err != nil {
		t.Errorf("Failed to get TX buffer: %v", err)
	}

	if txBuffer != session.TXBuffer {
		t.Error("Expected TX buffer to match session TX buffer")
	}

	// Test invalid direction
	_, err = session.GetBuffer(0x99)
	if err == nil {
		t.Error("Expected error for invalid direction")
	}
}

func TestSessionGetSessionInfo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := createTestManagerConfig()
	mgr, err := NewManager(logger, 60*time.Second, config)
	if err != nil {
		t.Skipf("Skipping test due to VAD model not available: %v", err)
	}
	defer mgr.Stop()

	signaling := createTestSignalingPayload()
	session, err := mgr.CreateSession(12345, signaling, 8000)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add some audio data to update sequence numbers
	session.AddAudioData(protocol.DirectionRX, 100, []byte{1, 2, 3, 4})
	session.AddAudioData(protocol.DirectionTX, 200, []byte{1, 2, 3, 4})

	info := session.GetSessionInfo()

	if info.StreamID != 12345 {
		t.Errorf("Expected stream ID 12345, got %d", info.StreamID)
	}

	if info.CallerID != "John Doe" {
		t.Errorf("Expected caller ID 'John Doe', got '%s'", info.CallerID)
	}

	if info.RXPackets != 100 {
		t.Errorf("Expected RX packets 100, got %d", info.RXPackets)
	}

	if info.TXPackets != 200 {
		t.Errorf("Expected TX packets 200, got %d", info.TXPackets)
	}

	if info.Duration <= 0 {
		t.Errorf("Expected positive duration, got %v", info.Duration)
	}
}

// Helper function to create test signaling payload
func createTestSignalingPayload() *protocol.SignalingPayload {
	payload := &protocol.SignalingPayload{}
	
	copy(payload.ChannelID[:], []byte("SIP/1001-00000001"))
	copy(payload.Extension[:], []byte("1001"))
	copy(payload.CallerID[:], []byte("John Doe"))
	copy(payload.CalledID[:], []byte("Jane Smith"))
	payload.Timestamp = 1701234567
	
	return payload
} 