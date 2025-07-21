package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	
	"github.com/skypro1111/tlv-audio-service/internal/config"
	"github.com/skypro1111/tlv-audio-service/internal/metrics"
	"github.com/skypro1111/tlv-audio-service/internal/stream"
)

// HTTPServer provides HTTP API endpoints for monitoring and management
type HTTPServer struct {
	server    *http.Server
	logger    *slog.Logger
	config    *config.Config
	streamMgr *stream.Manager
	udpServer *UDPServer
	metrics   *metrics.Metrics
	
	// Server state
	startTime time.Time
	mu        sync.RWMutex
}

// HTTPServerConfig contains HTTP server configuration
type HTTPServerConfig struct {
	Port    int    `yaml:"port"`
	Address string `yaml:"address"`
	Enabled bool   `yaml:"enabled"`
}

// NewHTTPServer creates a new HTTP API server
func NewHTTPServer(cfg HTTPServerConfig, logger *slog.Logger, 
	appConfig *config.Config, streamMgr *stream.Manager, udpServer *UDPServer, m *metrics.Metrics) *HTTPServer {
	
	h := &HTTPServer{
		logger:    logger,
		config:    appConfig,
		streamMgr: streamMgr,
		udpServer: udpServer,
		metrics:   m,
		startTime: time.Now(),
	}
	
	// Create HTTP server with routes
	mux := http.NewServeMux()
	h.setupRoutes(mux)
	
	h.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Address, cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	return h
}

// setupRoutes configures HTTP API routes
func (h *HTTPServer) setupRoutes(mux *http.ServeMux) {
	// Health check endpoint
	mux.HandleFunc("/health", h.withMetrics("/health", h.handleHealth))
	
	// Streams monitoring endpoints
	mux.HandleFunc("/streams", h.withMetrics("/streams", h.handleStreams))
	mux.HandleFunc("/streams/", h.withMetrics("/streams/{id}", h.handleStreamDetail))
	
	// Configuration endpoint
	mux.HandleFunc("/config", h.withMetrics("/config", h.handleConfig))
	
	// Statistics endpoint
	mux.HandleFunc("/stats", h.withMetrics("/stats", h.handleStats))
	
	// Transcription statistics
	mux.HandleFunc("/stats/transcription", h.withMetrics("/stats/transcription", h.handleTranscriptionStats))
	
	// Prometheus metrics endpoint (no metrics needed for metrics endpoint)
	mux.Handle("/metrics", promhttp.Handler())
	
	// Root endpoint with API documentation
	mux.HandleFunc("/", h.withMetrics("/", h.handleRoot))
}

// withMetrics wraps an HTTP handler with metrics collection
func (h *HTTPServer) withMetrics(endpoint string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		
		// Create a response writer wrapper to capture status code
		ww := &responseWriter{ResponseWriter: w, statusCode: 200}
		
		// Call the original handler
		handler(ww, r)
		
		// Record metrics
		duration := time.Since(startTime).Seconds()
		statusCode := fmt.Sprintf("%d", ww.statusCode)
		
		h.metrics.RecordHTTPRequest(r.Method, endpoint, statusCode, duration)
		
		// Record error if status code indicates an error
		if ww.statusCode >= 400 {
			errorType := "client_error"
			if ww.statusCode >= 500 {
				errorType = "server_error"
			}
			h.metrics.RecordHTTPError(r.Method, endpoint, errorType)
		}
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Start starts the HTTP server
func (h *HTTPServer) Start() error {
	h.logger.Info("Starting HTTP API server",
		slog.String("address", h.server.Addr),
	)
	
	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error("HTTP server error", slog.String("error", err.Error()))
		}
	}()
	
	return nil
}

// Stop gracefully stops the HTTP server
func (h *HTTPServer) Stop(ctx context.Context) error {
	h.logger.Info("Stopping HTTP API server...")
	
	return h.server.Shutdown(ctx)
}

// handleHealth implements the /health endpoint
func (h *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	uptime := time.Since(h.startTime)
	udpStats := h.udpServer.GetStatistics()
	transcriptionStats := h.streamMgr.GetTranscriptionStats()
	
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"uptime":    uptime.String(),
		"service": map[string]interface{}{
			"name":    "tlv-audio-service",
			"version": "1.0.0",
		},
		"components": map[string]interface{}{
			"udp_server": map[string]interface{}{
				"status":            "running",
				"packets_received":  udpStats.PacketsReceived,
				"packets_processed": udpStats.PacketsProcessed,
				"parse_errors":      udpStats.ParseErrors,
				"queue_size":        udpStats.QueueSize,
			},
			"stream_manager": map[string]interface{}{
				"status":         "running",
				"active_streams": udpStats.ActiveStreams,
			},
			"transcription": map[string]interface{}{
				"status":         "running",
				"total_requests": transcriptionStats.TotalRequests,
				"success_rate":   transcriptionStats.SuccessRate,
				"active_requests": transcriptionStats.ActiveRequests,
			},
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleStreams implements the /streams endpoint
func (h *HTTPServer) handleStreams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	sessions := h.streamMgr.GetAllSessions()
	sessionInfos := make([]stream.SessionInfo, 0, len(sessions))
	
	for _, session := range sessions {
		sessionInfos = append(sessionInfos, session.GetSessionInfo())
	}
	
	response := map[string]interface{}{
		"total_streams": len(sessionInfos),
		"timestamp":     time.Now().UTC(),
		"streams":       sessionInfos,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStreamDetail implements the /streams/{stream_id} endpoint
func (h *HTTPServer) handleStreamDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Extract stream ID from URL path
	streamIDStr := r.URL.Path[len("/streams/"):]
	if streamIDStr == "" {
		http.Error(w, "Stream ID required", http.StatusBadRequest)
		return
	}
	
	streamID, err := strconv.ParseUint(streamIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid stream ID", http.StatusBadRequest)
		return
	}
	
	session, exists := h.streamMgr.GetSession(uint32(streamID))
	if !exists {
		http.Error(w, "Stream not found", http.StatusNotFound)
		return
	}
	
	sessionInfo := session.GetSessionInfo()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionInfo)
}

// handleConfig implements the /config endpoint
func (h *HTTPServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Return sanitized configuration (remove sensitive data)
	sanitizedConfig := map[string]interface{}{
		"server": map[string]interface{}{
			"udp_port":               h.config.Server.UDPPort,
			"bind_address":           h.config.Server.BindAddress,
			"buffer_size":            h.config.Server.BufferSize,
			"max_concurrent_streams": h.config.Server.MaxConcurrentStreams,
		},
		"audio": map[string]interface{}{
			"sample_rate":          h.config.Audio.SampleRate,
			"channels":             h.config.Audio.Channels,
			"bit_depth":            h.config.Audio.BitDepth,
			"chunk_min_duration":   h.config.Audio.ChunkMinDuration,
			"chunk_max_duration":   h.config.Audio.ChunkMaxDuration,
			"stream_timeout":       h.config.Audio.StreamTimeout,
		},
		"vad": map[string]interface{}{
			"model_path":           h.config.VAD.ModelPath,
			"threshold":            h.config.VAD.Threshold,
			"window_size":          h.config.VAD.WindowSize,
			"overlap_ratio":        h.config.VAD.OverlapRatio,
			"min_speech_duration":  h.config.VAD.MinSpeechDuration,
			"min_silence_duration": h.config.VAD.MinSilenceDuration,
		},
		"transcription": map[string]interface{}{
			"endpoint":       h.config.Transcription.Endpoint,
			"timeout":        h.config.Transcription.Timeout,
			"max_retries":    h.config.Transcription.MaxRetries,
			"max_concurrent": h.config.Transcription.MaxConcurrent,
			"output_format":  h.config.Transcription.OutputFormat,
			// Note: API key is intentionally omitted for security
		},
		"logging": map[string]interface{}{
			"level":  h.config.Logging.Level,
			"format": h.config.Logging.Format,
			"output": h.config.Logging.Output,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sanitizedConfig)
}

// handleStats implements the /stats endpoint
func (h *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	udpStats := h.udpServer.GetStatistics()
	transcriptionStats := h.streamMgr.GetTranscriptionStats()
	uptime := time.Since(h.startTime)
	
	stats := map[string]interface{}{
		"uptime":    uptime.String(),
		"timestamp": time.Now().UTC(),
		"udp": map[string]interface{}{
			"packets_received":  udpStats.PacketsReceived,
			"packets_processed": udpStats.PacketsProcessed,
			"parse_errors":      udpStats.ParseErrors,
			"active_streams":    udpStats.ActiveStreams,
			"queue_size":        udpStats.QueueSize,
			"queue_capacity":    udpStats.QueueCapacity,
		},
		"transcription": transcriptionStats,
		"streams": map[string]interface{}{
			"active_count": h.streamMgr.GetActiveSessionCount(),
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleTranscriptionStats implements the /stats/transcription endpoint
func (h *HTTPServer) handleTranscriptionStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	stats := h.streamMgr.GetTranscriptionStats()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleRoot implements the / endpoint with API documentation
func (h *HTTPServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	apiDoc := map[string]interface{}{
		"service": "TLV Audio Processing Service",
		"version": "1.0.0",
		"endpoints": map[string]interface{}{
			"GET /":                     "API documentation",
			"GET /health":               "Service health check",
			"GET /streams":              "List all active streams",
			"GET /streams/{stream_id}":  "Get detailed stream information",
			"GET /config":               "Get service configuration",
			"GET /stats":                "Get service statistics",
			"GET /stats/transcription":  "Get transcription statistics",
			"GET /metrics":              "Prometheus metrics",
		},
		"timestamp": time.Now().UTC(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiDoc)
} 