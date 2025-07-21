package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/skypro1111/tlv-audio-service/internal/config"
	"github.com/skypro1111/tlv-audio-service/internal/protocol"
	"github.com/skypro1111/tlv-audio-service/internal/stream"
)

// UDPServer handles incoming TLV packets from Asterisk
type UDPServer struct {
	conn      *net.UDPConn
	config    *config.ServerConfig
	logger    *slog.Logger
	streamMgr *stream.Manager
	
	// Concurrency management
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	
	// Packet processing
	packetChan chan *incomingPacket
	
	// Metrics (basic counters for now)
	packetsReceived uint64
	packetsProcessed uint64
	parseErrors     uint64
	mu              sync.RWMutex
}

// incomingPacket represents a received UDP packet with metadata
type incomingPacket struct {
	data       []byte
	remoteAddr *net.UDPAddr
	timestamp  time.Time
}

// NewUDPServer creates a new UDP server instance
func NewUDPServer(cfg *config.ServerConfig, logger *slog.Logger, streamMgr *stream.Manager) *UDPServer {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &UDPServer{
		config:     cfg,
		logger:     logger,
		streamMgr:  streamMgr,
		ctx:        ctx,
		cancel:     cancel,
		packetChan: make(chan *incomingPacket, 1000), // Buffer for 1000 packets
	}
}

// Start begins listening for UDP packets
func (s *UDPServer) Start() error {
	// Create UDP address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.UDPPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// Create UDP connection
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	s.conn = conn

	// Set buffer size
	if err := s.conn.SetReadBuffer(s.config.BufferSize); err != nil {
		s.logger.Warn("Failed to set UDP read buffer size", 
			slog.Int("buffer_size", s.config.BufferSize),
			slog.String("error", err.Error()),
		)
	}

	s.logger.Info("UDP server started",
		slog.String("address", addr.String()),
		slog.Int("buffer_size", s.config.BufferSize),
	)

	// Start packet processing workers
	numWorkers := 4 // Process packets concurrently
	for i := 0; i < numWorkers; i++ {
		s.wg.Add(1)
		go s.packetProcessor(i)
	}

	// Start main receiver loop
	s.wg.Add(1)
	go s.receiveLoop()

	return nil
}

// Stop gracefully stops the UDP server
func (s *UDPServer) Stop() error {
	s.logger.Info("Stopping UDP server...")

	// Cancel context to signal shutdown
	s.cancel()

	// Close UDP connection to unblock the receive loop
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			s.logger.Warn("Error closing UDP connection", slog.String("error", err.Error()))
		}
	}

	// Close packet channel to signal workers to stop
	close(s.packetChan)

	// Wait for all goroutines to finish
	s.wg.Wait()

	// Log final statistics
	s.mu.RLock()
	packetsReceived := s.packetsReceived
	packetsProcessed := s.packetsProcessed
	parseErrors := s.parseErrors
	s.mu.RUnlock()

	s.logger.Info("UDP server stopped",
		slog.Uint64("packets_received", packetsReceived),
		slog.Uint64("packets_processed", packetsProcessed),
		slog.Uint64("parse_errors", parseErrors),
	)

	return nil
}

// receiveLoop is the main packet receiving loop
func (s *UDPServer) receiveLoop() {
	defer s.wg.Done()

	buffer := make([]byte, s.config.BufferSize)

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Receive loop stopping due to context cancellation")
			return
		default:
			// Continue to receive packets
		}

		// Set read deadline to check for context cancellation periodically
		if err := s.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
			s.logger.Error("Failed to set read deadline", slog.String("error", err.Error()))
			continue
		}

		// Read packet
		n, remoteAddr, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			// Check if this is a timeout (expected during graceful shutdown)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Check context and try again
			}

			// Check if we're shutting down
			select {
			case <-s.ctx.Done():
				return
			default:
				s.logger.Error("Failed to read UDP packet", slog.String("error", err.Error()))
				continue
			}
		}

		// Update metrics
		s.mu.Lock()
		s.packetsReceived++
		s.mu.Unlock()

		// Create packet data copy (buffer will be reused)
		packetData := make([]byte, n)
		copy(packetData, buffer[:n])

		// Send to processing channel (non-blocking)
		packet := &incomingPacket{
			data:       packetData,
			remoteAddr: remoteAddr,
			timestamp:  time.Now(),
		}

		select {
		case s.packetChan <- packet:
			// Packet queued successfully
		default:
			// Channel full, drop packet and log warning
			s.logger.Warn("Packet processing queue full, dropping packet",
				slog.String("remote_addr", remoteAddr.String()),
				slog.Int("packet_size", n),
			)
		}
	}
}

// packetProcessor processes packets from the packet channel
func (s *UDPServer) packetProcessor(workerID int) {
	defer s.wg.Done()

	s.logger.Debug("Packet processor started", slog.Int("worker_id", workerID))

	for packet := range s.packetChan {
		s.handlePacket(packet, workerID)
	}

	s.logger.Debug("Packet processor stopped", slog.Int("worker_id", workerID))
}

// handlePacket processes a single incoming packet
func (s *UDPServer) handlePacket(packet *incomingPacket, workerID int) {
	// Parse the TLV packet
	parsedPacket, err := protocol.ParsePacket(packet.data)
	if err != nil {
		s.mu.Lock()
		s.parseErrors++
		s.mu.Unlock()

		s.logger.Error("Failed to parse packet",
			slog.String("remote_addr", packet.remoteAddr.String()),
			slog.Int("packet_size", len(packet.data)),
			slog.String("error", err.Error()),
			slog.Int("worker_id", workerID),
		)
		return
	}

	// Update metrics
	s.mu.Lock()
	s.packetsProcessed++
	s.mu.Unlock()

	// Process based on packet type
	switch parsedPacket.Header.PacketType {
	case protocol.PacketTypeSignaling:
		s.processSignalingPacket(parsedPacket.Header, parsedPacket.Signaling, workerID)
	case protocol.PacketTypeAudio:
		s.processAudioPacket(parsedPacket.Header, parsedPacket.Audio, workerID)
	default:
		s.logger.Error("Unknown packet type",
			slog.Uint64("stream_id", uint64(parsedPacket.Header.StreamID)),
			slog.Int("packet_type", int(parsedPacket.Header.PacketType)),
			slog.Int("worker_id", workerID),
		)
	}
}

// processSignalingPacket handles signaling packets (session creation/update)
func (s *UDPServer) processSignalingPacket(header *protocol.Header, payload *protocol.SignalingPayload, workerID int) {
	s.logger.Debug("Processing signaling packet",
		slog.Uint64("stream_id", uint64(header.StreamID)),
		slog.String("channel_id", payload.GetChannelID()),
		slog.String("caller_id", payload.GetCallerID()),
		slog.String("called_id", payload.GetCalledID()),
		slog.String("direction", directionString(header.Direction)),
		slog.Int("worker_id", workerID),
	)

	// Create or update stream session
	// Use the configured sample rate from audio config
	session, err := s.streamMgr.CreateSession(header.StreamID, payload, 8000) // TODO: Get from config
	if err != nil {
		s.logger.Error("Failed to create stream session",
			slog.Uint64("stream_id", uint64(header.StreamID)),
			slog.String("error", err.Error()),
			slog.Int("worker_id", workerID),
		)
		return
	}

	s.logger.Info("Signaling packet processed",
		slog.Uint64("stream_id", uint64(header.StreamID)),
		slog.String("caller_id", session.CallerID),
		slog.String("called_id", session.CalledID),
		slog.Int("worker_id", workerID),
	)
}

// processAudioPacket handles audio packets (route to appropriate stream buffer)
func (s *UDPServer) processAudioPacket(header *protocol.Header, payload *protocol.AudioPayload, workerID int) {
	// Get the stream session
	session, exists := s.streamMgr.GetSession(header.StreamID)
	if !exists {
		s.logger.Warn("Received audio packet for unknown stream",
			slog.Uint64("stream_id", uint64(header.StreamID)),
			slog.Uint64("sequence", uint64(payload.Sequence)),
			slog.String("direction", directionString(header.Direction)),
			slog.Int("audio_size", len(payload.AudioData)),
			slog.Int("worker_id", workerID),
		)
		return
	}

	// Add audio data to the appropriate buffer
	err := session.AddAudioData(header.Direction, payload.Sequence, payload.AudioData)
	if err != nil {
		s.logger.Error("Failed to add audio data to session",
			slog.Uint64("stream_id", uint64(header.StreamID)),
			slog.Uint64("sequence", uint64(payload.Sequence)),
			slog.String("direction", directionString(header.Direction)),
			slog.String("error", err.Error()),
			slog.Int("worker_id", workerID),
		)
		return
	}

	s.logger.Debug("Audio packet processed",
		slog.Uint64("stream_id", uint64(header.StreamID)),
		slog.Uint64("sequence", uint64(payload.Sequence)),
		slog.String("direction", directionString(header.Direction)),
		slog.Int("audio_size", len(payload.AudioData)),
		slog.Int("worker_id", workerID),
	)
}

// GetStatistics returns current server statistics
func (s *UDPServer) GetStatistics() ServerStatistics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ServerStatistics{
		PacketsReceived:  s.packetsReceived,
		PacketsProcessed: s.packetsProcessed,
		ParseErrors:      s.parseErrors,
		ActiveStreams:    uint64(s.streamMgr.GetActiveSessionCount()),
		QueueSize:        uint64(len(s.packetChan)),
		QueueCapacity:    uint64(cap(s.packetChan)),
	}
}

// ServerStatistics represents server performance metrics
type ServerStatistics struct {
	PacketsReceived  uint64 `json:"packets_received"`
	PacketsProcessed uint64 `json:"packets_processed"`
	ParseErrors      uint64 `json:"parse_errors"`
	ActiveStreams    uint64 `json:"active_streams"`
	QueueSize        uint64 `json:"queue_size"`
	QueueCapacity    uint64 `json:"queue_capacity"`
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