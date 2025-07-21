package stream

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/skypro1111/tlv-audio-service/internal/audio"
	"github.com/skypro1111/tlv-audio-service/internal/protocol"
	"github.com/skypro1111/tlv-audio-service/internal/transcription"
	"github.com/skypro1111/tlv-audio-service/internal/vad"
)

// StreamSession represents an active audio stream session with VAD processing
type StreamSession struct {
	ID           uint32
	ChannelID    string
	Extension    string
	CallerID     string
	CalledID     string
	StartTime    time.Time
	LastActivity time.Time
	
	// Audio buffers for both directions
	RXBuffer     *audio.Buffer // Received audio
	TXBuffer     *audio.Buffer // Transmitted audio
	
	// VAD processors for both directions
	RXVADProcessor *vad.Processor // VAD for received audio
	TXVADProcessor *vad.Processor // VAD for transmitted audio
	
	// Audio chunkers for both directions
	RXChunker *audio.Chunker // Chunker for received audio
	TXChunker *audio.Chunker // Chunker for transmitted audio
	
	// Processing state
	rxLastProcessedWindow int
	txLastProcessedWindow int
	
	// Voice activity tracking
	rxVoiceSegments []*vad.VoiceSegment
	txVoiceSegments []*vad.VoiceSegment
	
	// Transcription tracking
	chunksGenerated  uint64
	chunksSent       uint64
	chunksSuccessful uint64
	chunksFailed     uint64
	
	// Processing control
	processingCtx    context.Context
	processingCancel context.CancelFunc
	processingWG     sync.WaitGroup
	
	// Manager reference for transcription
	manager *Manager
	
	// Thread safety
	mu           sync.RWMutex
}

// Manager manages all active stream sessions with VAD processing
type Manager struct {
	sessions map[uint32]*StreamSession
	mu       sync.RWMutex
	logger   *slog.Logger
	timeout  time.Duration
	
	// VAD configuration
	vadConfig VADConfig
	
	// Chunking configuration
	chunkingConfig audio.ChunkingConfig
	
	// Transcription client
	transcriptionClient *transcription.Client
	
	// Cleanup management
	ctx      context.Context
	cancel   context.CancelFunc
	cleanup  chan struct{}
}

// VADConfig holds VAD processor configuration
type VADConfig struct {
	ModelPath          string
	Threshold          float32
	WindowSize         int
	SampleRate         int
	MinSpeechDuration  time.Duration
	MinSilenceDuration time.Duration
}

// ManagerConfig contains configuration for the stream manager
type ManagerConfig struct {
	VADConfig           VADConfig
	ChunkingConfig      audio.ChunkingConfig
	TranscriptionConfig transcription.Config
}

// NewManager creates a new stream manager with VAD processing capabilities
func NewManager(logger *slog.Logger, timeout time.Duration, config ManagerConfig) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create transcription client
	transcriptionClient, err := transcription.NewClient(config.TranscriptionConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create transcription client: %w", err)
	}
	
	mgr := &Manager{
		sessions:            make(map[uint32]*StreamSession),
		logger:              logger,
		timeout:             timeout,
		ctx:                 ctx,
		cancel:              cancel,
		cleanup:             make(chan struct{}),
		vadConfig:           config.VADConfig,
		chunkingConfig:      config.ChunkingConfig,
		transcriptionClient: transcriptionClient,
	}
	
	// Start cleanup goroutine
	go mgr.startCleanupRoutine()
	
	return mgr, nil
}

// CreateSession creates a new stream session with VAD processors
func (m *Manager) CreateSession(streamID uint32, signaling *protocol.SignalingPayload, sampleRate int) (*StreamSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check if session already exists
	if existing, exists := m.sessions[streamID]; exists {
		m.logger.Warn("Session already exists, updating metadata",
			slog.Uint64("stream_id", uint64(streamID)),
			slog.String("existing_caller", existing.CallerID),
			slog.String("new_caller", signaling.GetCallerID()),
		)
		
		// Update existing session metadata
		existing.mu.Lock()
		existing.ChannelID = signaling.GetChannelID()
		existing.Extension = signaling.GetExtension()
		existing.CallerID = signaling.GetCallerID()
		existing.CalledID = signaling.GetCalledID()
		existing.LastActivity = time.Now()
		existing.mu.Unlock()
		
		return existing, nil
	}
	
	// Create VAD processors
	rxVAD, err := vad.NewProcessor(m.vadConfig.ModelPath, m.vadConfig.Threshold, 
		m.vadConfig.WindowSize, m.vadConfig.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("failed to create RX VAD processor: %w", err)
	}
	
	txVAD, err := vad.NewProcessor(m.vadConfig.ModelPath, m.vadConfig.Threshold, 
		m.vadConfig.WindowSize, m.vadConfig.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("failed to create TX VAD processor: %w", err)
	}
	
	// Initialize VAD processors
	if err := rxVAD.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize RX VAD processor: %w", err)
	}
	
	if err := txVAD.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize TX VAD processor: %w", err)
	}
	
	// Create audio chunkers for both directions
	rxChunker := audio.NewChunker(m.chunkingConfig)
	txChunker := audio.NewChunker(m.chunkingConfig)
	
	// Create processing context for this session
	processingCtx, processingCancel := context.WithCancel(m.ctx)
	
	// Create new session
	now := time.Now()
	session := &StreamSession{
		ID:           streamID,
		ChannelID:    signaling.GetChannelID(),
		Extension:    signaling.GetExtension(),
		CallerID:     signaling.GetCallerID(),
		CalledID:     signaling.GetCalledID(),
		StartTime:    now,
		LastActivity: now,
		
		// Create audio buffers for both directions
		RXBuffer: audio.NewBuffer(streamID, protocol.DirectionRX, sampleRate),
		TXBuffer: audio.NewBuffer(streamID, protocol.DirectionTX, sampleRate),
		
		// Assign VAD processors
		RXVADProcessor: rxVAD,
		TXVADProcessor: txVAD,
		
		// Assign chunkers
		RXChunker: rxChunker,
		TXChunker: txChunker,
		
		// Initialize voice segments
		rxVoiceSegments: make([]*vad.VoiceSegment, 0),
		txVoiceSegments: make([]*vad.VoiceSegment, 0),
		
		// Processing control
		processingCtx:    processingCtx,
		processingCancel: processingCancel,
		
		// Manager reference
		manager: m,
	}
	
	m.sessions[streamID] = session
	
	// Start VAD processing for this session
	session.startVADProcessing(m.logger)
	
	m.logger.Info("Created new stream session with VAD processing",
		slog.Uint64("stream_id", uint64(streamID)),
		slog.String("channel_id", session.ChannelID),
		slog.String("extension", session.Extension),
		slog.String("caller_id", session.CallerID),
		slog.String("called_id", session.CalledID),
	)
	
	return session, nil
}

// GetSession retrieves an existing stream session
func (m *Manager) GetSession(streamID uint32) (*StreamSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	session, exists := m.sessions[streamID]
	return session, exists
}

// UpdateActivity updates the last activity time for a stream
func (m *Manager) UpdateActivity(streamID uint32) {
	m.mu.RLock()
	session, exists := m.sessions[streamID]
	m.mu.RUnlock()
	
	if !exists {
		m.logger.Warn("Attempted to update activity for non-existent stream",
			slog.Uint64("stream_id", uint64(streamID)),
		)
		return
	}
	
	session.mu.Lock()
	session.LastActivity = time.Now()
	session.mu.Unlock()
}

// GetActiveSessionCount returns the number of currently active sessions
func (m *Manager) GetActiveSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// GetAllSessions returns a snapshot of all active sessions (for monitoring)
func (m *Manager) GetAllSessions() []*StreamSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	sessions := make([]*StreamSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	
	return sessions
}

// RemoveSession removes a stream session and stops its processing
func (m *Manager) RemoveSession(streamID uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	session, exists := m.sessions[streamID]
	if !exists {
		return false
	}
	
	m.logger.Info("Finalizing stream session",
		slog.Uint64("stream_id", uint64(streamID)),
		slog.String("caller_id", session.CallerID),
		slog.Duration("duration", time.Since(session.StartTime)),
		slog.Uint64("chunks_generated", session.chunksGenerated),
		slog.Uint64("chunks_sent", session.chunksSent),
		slog.Uint64("chunks_successful", session.chunksSuccessful),
	)
	
	// Stop VAD processing for this session (includes final chunk processing)
	session.stopVADProcessing()
	
	delete(m.sessions, streamID)
	
	m.logger.Info("Stream session removed successfully",
		slog.Uint64("stream_id", uint64(streamID)),
		slog.String("caller_id", session.CallerID),
		slog.Duration("total_duration", time.Since(session.StartTime)),
	)
	
	return true
}

// Stop gracefully stops the stream manager
func (m *Manager) Stop() {
	m.logger.Info("Stopping stream manager...")
	
	// Stop all sessions first
	m.mu.Lock()
	for _, session := range m.sessions {
		session.stopVADProcessing()
	}
	m.mu.Unlock()
	
	// Close transcription client
	if err := m.transcriptionClient.Close(); err != nil {
		m.logger.Warn("Error closing transcription client", slog.String("error", err.Error()))
	}
	
	// Cancel context to stop cleanup routine
	m.cancel()
	
	// Wait for cleanup routine to finish
	<-m.cleanup
	
	// Log final statistics
	activeCount := m.GetActiveSessionCount()
	transcriptionStats := m.transcriptionClient.GetStats()
	
	m.logger.Info("Stream manager stopped",
		slog.Int("remaining_sessions", activeCount),
		slog.Uint64("total_transcription_requests", transcriptionStats.TotalRequests),
		slog.Uint64("successful_transcriptions", transcriptionStats.SuccessRequests),
		slog.Float64("transcription_success_rate", transcriptionStats.SuccessRate),
	)
}

// GetTranscriptionStats returns current transcription client statistics
func (m *Manager) GetTranscriptionStats() transcription.ClientStats {
	return m.transcriptionClient.GetStats()
}

// startCleanupRoutine runs in a separate goroutine to clean up expired sessions
func (m *Manager) startCleanupRoutine() {
	defer close(m.cleanup)
	
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()
	
	m.logger.Info("Stream cleanup routine started",
		slog.Duration("timeout", m.timeout),
		slog.Duration("check_interval", 30*time.Second),
	)
	
	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info("Stream cleanup routine stopping")
			return
			
		case <-ticker.C:
			m.cleanupExpiredSessions()
		}
	}
}

// cleanupExpiredSessions removes sessions that have been inactive for too long
func (m *Manager) cleanupExpiredSessions() {
	now := time.Now()
	expiredSessions := make([]uint32, 0)
	
	// Find expired sessions
	m.mu.RLock()
	for streamID, session := range m.sessions {
		session.mu.RLock()
		lastActivity := session.LastActivity
		session.mu.RUnlock()
		
		if now.Sub(lastActivity) > m.timeout {
			expiredSessions = append(expiredSessions, streamID)
		}
	}
	m.mu.RUnlock()
	
	// Remove expired sessions
	if len(expiredSessions) > 0 {
		m.logger.Info("Cleaning up expired sessions",
			slog.Int("expired_count", len(expiredSessions)),
		)
		
		for _, streamID := range expiredSessions {
			m.RemoveSession(streamID)
		}
	}
}

// startVADProcessing starts VAD processing for the session
func (s *StreamSession) startVADProcessing(logger *slog.Logger) {
	// Start RX VAD processing
	s.processingWG.Add(1)
	go func() {
		defer s.processingWG.Done()
		s.vadProcessingLoop(protocol.DirectionRX, logger)
	}()
	
	// Start TX VAD processing
	s.processingWG.Add(1)
	go func() {
		defer s.processingWG.Done()
		s.vadProcessingLoop(protocol.DirectionTX, logger)
	}()
}

// stopVADProcessing stops VAD processing for the session
func (s *StreamSession) stopVADProcessing() {
	// Force finalize any pending chunks before stopping
	s.finalizeAllPendingChunks()
	
	s.processingCancel()
	s.processingWG.Wait()
}

// finalizeAllPendingChunks forces finalization of any pending chunks in both directions
func (s *StreamSession) finalizeAllPendingChunks() {
	logger := s.manager.logger
	
	// Force finalize RX chunker
	if s.RXChunker.HasPendingChunk() {
		rxChunk := s.RXChunker.ForceFinalize(s.RXBuffer)
		if rxChunk != nil {
			s.mu.Lock()
			s.chunksGenerated++
			s.mu.Unlock()
			
			logger.Info("Final RX chunk generated on stream end",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("chunk_id", rxChunk.ChunkID),
				slog.Float64("duration", rxChunk.Duration.Seconds()),
				slog.Int("samples", len(rxChunk.Samples)),
			)
			
			// Send final chunk for transcription
			go s.processTranscription(rxChunk, logger, s.manager.transcriptionClient)
		}
	}
	
	// Force finalize TX chunker  
	if s.TXChunker.HasPendingChunk() {
		txChunk := s.TXChunker.ForceFinalize(s.TXBuffer)
		if txChunk != nil {
			s.mu.Lock()
			s.chunksGenerated++
			s.mu.Unlock()
			
			logger.Info("Final TX chunk generated on stream end",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("chunk_id", txChunk.ChunkID),
				slog.Float64("duration", txChunk.Duration.Seconds()),
				slog.Int("samples", len(txChunk.Samples)),
			)
			
			// Send final chunk for transcription
			go s.processTranscription(txChunk, logger, s.manager.transcriptionClient)
		}
	}
}

// vadProcessingLoop continuously processes audio through VAD
func (s *StreamSession) vadProcessingLoop(direction uint8, logger *slog.Logger) {
	ticker := time.NewTicker(100 * time.Millisecond) // Check for new audio every 100ms
	defer ticker.Stop()
	
	directionStr := "RX"
	if direction == protocol.DirectionTX {
		directionStr = "TX"
	}
	
	logger.Debug("VAD processing loop started",
		slog.Uint64("stream_id", uint64(s.ID)),
		slog.String("direction", directionStr),
	)
	
	for {
		select {
		case <-s.processingCtx.Done():
			logger.Debug("VAD processing loop stopping",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("direction", directionStr),
			)
			return
		case <-ticker.C:
			s.processAvailableWindows(direction, logger, s.manager)
		}
	}
}

// processAvailableWindows processes any available audio windows through VAD and chunking
func (s *StreamSession) processAvailableWindows(direction uint8, logger *slog.Logger, manager *Manager) {
	var buffer *audio.Buffer
	var vadProcessor *vad.Processor
	var chunker *audio.Chunker
	var lastProcessedWindow *int
	
	s.mu.Lock()
	switch direction {
	case protocol.DirectionRX:
		buffer = s.RXBuffer
		vadProcessor = s.RXVADProcessor
		chunker = s.RXChunker
		lastProcessedWindow = &s.rxLastProcessedWindow
	case protocol.DirectionTX:
		buffer = s.TXBuffer
		vadProcessor = s.TXVADProcessor
		chunker = s.TXChunker
		lastProcessedWindow = &s.txLastProcessedWindow
	default:
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	
	// Check if there are new windows to process
	availableWindows := buffer.GetAvailableWindows()
	if availableWindows <= *lastProcessedWindow {
		return // No new windows
	}
	
	// Process new windows
	for windowIndex := *lastProcessedWindow; windowIndex < availableWindows; windowIndex++ {
		window, err := buffer.GetAudioWindow(windowIndex)
		if err != nil {
			logger.Warn("Failed to get audio window",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("direction", directionString(direction)),
				slog.Int("window_index", windowIndex),
				slog.String("error", err.Error()),
			)
			break
		}
		
		// Process through VAD
		result, err := vadProcessor.Process(window.Samples)
		if err != nil {
			logger.Error("VAD processing failed",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("direction", directionString(direction)),
				slog.Int("window_index", windowIndex),
				slog.String("error", err.Error()),
			)
			continue
		}
		
		// Process through chunker (VAD result → potential audio chunk)
		chunk, err := chunker.ProcessVADResult(s.ID, direction, s.CallerID, window, result, buffer)
		if err != nil {
			logger.Error("Chunker processing failed",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("direction", directionString(direction)),
				slog.Int("window_index", windowIndex),
				slog.String("error", err.Error()),
			)
			continue
		}
		
		// If chunk was created, send for transcription
		if chunk != nil {
			s.mu.Lock()
			s.chunksGenerated++
			s.mu.Unlock()
			
			logger.Info("Audio chunk generated",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("direction", directionString(direction)),
				slog.String("chunk_id", chunk.ChunkID),
				slog.Float64("duration", chunk.Duration.Seconds()),
				slog.Float64("confidence", float64(chunk.Confidence)),
				slog.Int("samples", len(chunk.Samples)),
			)
			
			// Send chunk for transcription asynchronously
			go s.processTranscription(chunk, logger, manager.transcriptionClient)
		}
		
		// Log voice activity detection
		if result.HasVoice {
			logger.Debug("Voice activity detected",
				slog.Uint64("stream_id", uint64(s.ID)),
				slog.String("direction", directionString(direction)),
				slog.Int("window_index", windowIndex),
				slog.Float64("probability", float64(result.Probability)),
				slog.Float64("confidence", float64(result.Confidence)),
			)
		}
		
		*lastProcessedWindow = windowIndex + 1
		
		// Trim old audio data periodically to prevent memory growth
		if windowIndex%20 == 0 { // Trim every 20 windows
			buffer.TrimProcessedAudio(10) // Keep last 10 windows
		}
	}
}

// GetSessionInfo returns enhanced session information including VAD stats
func (s *StreamSession) GetSessionInfo() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	rxStats := s.RXVADProcessor.GetStats()
	txStats := s.TXVADProcessor.GetStats()
	
	return SessionInfo{
		StreamID:     s.ID,
		ChannelID:    s.ChannelID,
		Extension:    s.Extension,
		CallerID:     s.CallerID,
		CalledID:     s.CalledID,
		StartTime:    s.StartTime,
		LastActivity: s.LastActivity,
		Duration:     time.Since(s.StartTime),
		RXPackets:    s.RXBuffer.GetLastSequence(),
		TXPackets:    s.TXBuffer.GetLastSequence(),
		
		// Enhanced VAD information
		RXVoicePercentage: rxStats.VoicePercentage,
		TXVoicePercentage: txStats.VoicePercentage,
		RXWindowsProcessed: rxStats.TotalWindows,
		TXWindowsProcessed: txStats.TotalWindows,
		
		// Chunking and transcription statistics
		ChunksGenerated:  s.chunksGenerated,
		ChunksSent:       s.chunksSent,
		ChunksSuccessful: s.chunksSuccessful,
		ChunksFailed:     s.chunksFailed,
	}
}

// SessionInfo represents enhanced session information for monitoring and APIs
type SessionInfo struct {
	StreamID     uint32        `json:"stream_id"`
	ChannelID    string        `json:"channel_id"`
	Extension    string        `json:"extension"`
	CallerID     string        `json:"caller_id"`
	CalledID     string        `json:"called_id"`
	StartTime    time.Time     `json:"start_time"`
	LastActivity time.Time     `json:"last_activity"`
	Duration     time.Duration `json:"duration"`
	RXPackets    uint32        `json:"rx_packets"`
	TXPackets    uint32        `json:"tx_packets"`
	
	// VAD statistics
	RXVoicePercentage  float64 `json:"rx_voice_percentage"`
	TXVoicePercentage  float64 `json:"tx_voice_percentage"`
	RXWindowsProcessed uint64  `json:"rx_windows_processed"`
	TXWindowsProcessed uint64  `json:"tx_windows_processed"`
	
	// Chunking and transcription statistics
	ChunksGenerated  uint64 `json:"chunks_generated"`
	ChunksSent       uint64 `json:"chunks_sent"`
	ChunksSuccessful uint64 `json:"chunks_successful"`
	ChunksFailed     uint64 `json:"chunks_failed"`
}

// AddAudioData adds audio data to the appropriate buffer (triggers VAD processing)
func (s *StreamSession) AddAudioData(direction uint8, sequence uint32, data []byte) error {
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()
	
	var buffer *audio.Buffer
	switch direction {
	case protocol.DirectionRX:
		buffer = s.RXBuffer
	case protocol.DirectionTX:
		buffer = s.TXBuffer
	default:
		return fmt.Errorf("invalid direction: 0x%02x", direction)
	}
	
	return buffer.AddAudioData(sequence, data)
}

// GetBuffer returns the audio buffer for the specified direction
func (s *StreamSession) GetBuffer(direction uint8) (*audio.Buffer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	switch direction {
	case protocol.DirectionRX:
		return s.RXBuffer, nil
	case protocol.DirectionTX:
		return s.TXBuffer, nil
	default:
		return nil, fmt.Errorf("invalid direction: 0x%02x", direction)
	}
}

// GetVADProcessor returns the VAD processor for the specified direction
func (s *StreamSession) GetVADProcessor(direction uint8) (*vad.Processor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	switch direction {
	case protocol.DirectionRX:
		return s.RXVADProcessor, nil
	case protocol.DirectionTX:
		return s.TXVADProcessor, nil
	default:
		return nil, fmt.Errorf("invalid direction: 0x%02x", direction)
	}
}

// processTranscription sends an audio chunk for transcription
func (s *StreamSession) processTranscription(chunk *audio.AudioChunk, logger *slog.Logger, client *transcription.Client) {
	s.mu.Lock()
	s.chunksSent++
	s.mu.Unlock()
	
	// Create enriched transcription request with all available information
	request := &transcription.TranscriptionRequest{
		Chunk: &transcription.AudioChunk{
			// Basic chunk information
			StreamID:    chunk.StreamID,
			Direction:   chunk.Direction,
			ChunkID:     chunk.ChunkID,
			StartTime:   chunk.StartTime,
			EndTime:     chunk.EndTime,
			Duration:    chunk.Duration,
			
			// Call/Session metadata (complete context)
			CallerID:    s.CallerID,
			CalledID:    s.CalledID,
			ChannelID:   s.ChannelID,
			Extension:   s.Extension,
			
			// Audio technical details
			SampleRate:  chunk.SampleRate,
			AudioData:   chunk.AudioData,
			Format:      chunk.Format,
			Confidence:  chunk.Confidence,
			StartSeq:    chunk.StartSeq,
			EndSeq:      chunk.EndSeq,
			
			// Stream session context
			SessionStartTime: s.StartTime,
			SessionDuration:  time.Since(s.StartTime),
		},
		
		// Transcription parameters
		Language: "uk", // Ukrainian language
		Model:    "whisper-large-v3", // Specify model if available
		
		// Request metadata
		RequestID: fmt.Sprintf("%s_%d", chunk.ChunkID, time.Now().UnixNano()),
		Timestamp: time.Now(),
		ServiceInfo: struct {
			Version string `json:"version"`
			Name    string `json:"name"`
		}{
			Version: "1.0.0",
			Name:    "tlv-audio-service",
		},
	}
	
	// Log enriched request details
	logger.Info("Sending enriched chunk for transcription",
		slog.Uint64("stream_id", uint64(s.ID)),
		slog.String("chunk_id", chunk.ChunkID),
		slog.String("request_id", request.RequestID),
		slog.String("caller_id", s.CallerID),
		slog.String("called_id", s.CalledID),
		slog.String("channel_id", s.ChannelID),
		slog.String("extension", s.Extension),
		slog.Float64("duration", chunk.Duration.Seconds()),
		slog.Float64("session_duration", time.Since(s.StartTime).Seconds()),
		slog.String("format", chunk.Format),
		slog.String("language", request.Language),
		slog.Int("audio_data_size", len(chunk.AudioData)),
	)
	
	// Send transcription request with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	startTime := time.Now()
	response, err := client.Transcribe(ctx, request)
	duration := time.Since(startTime)
	
	if err != nil {
		s.mu.Lock()
		s.chunksFailed++
		s.mu.Unlock()
		
		logger.Error("Transcription failed",
			slog.Uint64("stream_id", uint64(s.ID)),
			slog.String("chunk_id", chunk.ChunkID),
			slog.String("error", err.Error()),
			slog.Float64("duration", duration.Seconds()),
		)
		return
	}
	
	s.mu.Lock()
	s.chunksSuccessful++
	s.mu.Unlock()
	
	logger.Info("Chunk transcription completed",
		slog.Uint64("stream_id", uint64(s.ID)),
		slog.String("chunk_id", chunk.ChunkID),
		slog.String("transcribed_text", response.Text),
		slog.Float64("transcription_confidence", float64(response.Confidence)),
		slog.Float64("duration", duration.Seconds()),
	)
}

// directionString converts direction code to human-readable string
func directionString(direction uint8) string {
	switch direction {
	case protocol.DirectionRX:
		return "RX"
	case protocol.DirectionTX:
		return "TX"
	default:
		return fmt.Sprintf("Unknown(0x%02x)", direction)
	}
} 