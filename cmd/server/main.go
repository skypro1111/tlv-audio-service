package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skypro1111/tlv-audio-service/internal/audio"
	"github.com/skypro1111/tlv-audio-service/internal/config"
	"github.com/skypro1111/tlv-audio-service/internal/metrics"
	"github.com/skypro1111/tlv-audio-service/internal/server"
	"github.com/skypro1111/tlv-audio-service/internal/stream"
	"github.com/skypro1111/tlv-audio-service/internal/transcription"
)

const (
	defaultConfigPath = "configs/config.yaml"
	serviceName       = "tlv-audio-service"
	serviceVersion    = "1.0.0"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", defaultConfigPath, "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger based on configuration
	logger := initLogger(cfg.Logging)
	
	// Log service startup
	logger.Info("Service starting",
		slog.String("service", serviceName),
		slog.String("version", serviceVersion),
		slog.String("config_path", *configPath),
	)

	// Log configuration summary (without sensitive data)
	logger.Info("Configuration loaded",
		slog.Int("udp_port", cfg.Server.UDPPort),
		slog.String("bind_address", cfg.Server.BindAddress),
		slog.Int("max_concurrent_streams", cfg.Server.MaxConcurrentStreams),
		slog.Int("sample_rate", cfg.Audio.SampleRate),
		slog.Float64("chunk_min_duration", cfg.Audio.ChunkMinDuration),
		slog.Float64("chunk_max_duration", cfg.Audio.ChunkMaxDuration),
		slog.String("vad_model_path", cfg.VAD.ModelPath),
		slog.Float64("vad_threshold", float64(cfg.VAD.Threshold)),
		slog.String("transcription_endpoint", cfg.Transcription.Endpoint),
		slog.String("log_level", cfg.Logging.Level),
	)

	// Create cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Prometheus metrics
	appMetrics := metrics.NewMetrics()
	logger.Info("Prometheus metrics initialized")

	// Create stream manager configuration
	streamConfig := stream.ManagerConfig{
		VADConfig: stream.VADConfig{
			ModelPath:          cfg.VAD.ModelPath,
			Threshold:          cfg.VAD.Threshold,
			WindowSize:         cfg.VAD.WindowSize,
			SampleRate:         cfg.Audio.SampleRate,
			MinSpeechDuration:  cfg.VAD.GetMinSpeechDuration(),
			MinSilenceDuration: cfg.VAD.GetMinSilenceDuration(),
		},
		ChunkingConfig: audio.ChunkingConfig{
			MinDuration:        cfg.Audio.GetChunkMinDuration(),
			MaxDuration:        cfg.Audio.GetChunkMaxDuration(),
			MinSpeechDuration:  cfg.VAD.GetMinSpeechDuration(),
			MinSilenceDuration: cfg.VAD.GetMinSilenceDuration(),
			SampleRate:         cfg.Audio.SampleRate,
			Format:             "raw", // Use raw PCM format to avoid corruption
		},
		TranscriptionConfig: transcription.Config{
			Endpoint:      cfg.Transcription.Endpoint,
			APIKey:        cfg.Transcription.APIKey,
			Timeout:       cfg.Transcription.GetTimeoutDuration(),
			MaxRetries:    cfg.Transcription.MaxRetries,
			MaxConcurrent: cfg.Transcription.MaxConcurrent,
			OutputFormat:  cfg.Transcription.OutputFormat,
		},
	}

	// Initialize stream manager
	streamMgr, err := stream.NewManager(logger, cfg.Audio.GetStreamTimeoutDuration(), streamConfig)
	if err != nil {
		logger.Error("Failed to create stream manager", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("Stream manager initialized",
		slog.Duration("stream_timeout", cfg.Audio.GetStreamTimeoutDuration()),
		slog.String("vad_model_path", streamConfig.VADConfig.ModelPath),
		slog.String("transcription_endpoint", streamConfig.TranscriptionConfig.Endpoint),
	)

	// Initialize UDP server
	udpServer := server.NewUDPServer(&cfg.Server, logger, streamMgr)
	logger.Info("UDP server initialized")

	// Initialize HTTP API server (if enabled)
	var httpServer *server.HTTPServer
	if cfg.HTTP.Enabled {
		httpConfig := server.HTTPServerConfig{
			Port:    cfg.HTTP.Port,
			Address: cfg.HTTP.Address,
			Enabled: cfg.HTTP.Enabled,
		}
		httpServer = server.NewHTTPServer(httpConfig, logger, cfg, streamMgr, udpServer, appMetrics)
		logger.Info("HTTP API server initialized",
			slog.String("address", fmt.Sprintf("%s:%d", cfg.HTTP.Address, cfg.HTTP.Port)),
		)
	}

	// Start UDP server
	if err := udpServer.Start(); err != nil {
		logger.Error("Failed to start UDP server", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Start HTTP server (if enabled)
	if httpServer != nil {
		if err := httpServer.Start(); err != nil {
			logger.Error("Failed to start HTTP server", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info("Service started successfully, waiting for signals...",
		slog.String("udp_address", fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.UDPPort)),
	)

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", slog.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down")
	}

	logger.Info("Starting graceful shutdown...")

	// Stop HTTP server first (stop accepting new requests)
	if httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		
		if err := httpServer.Stop(shutdownCtx); err != nil {
			logger.Error("Error stopping HTTP server", slog.String("error", err.Error()))
		}
	}

	// Stop UDP server (stop accepting new packets)
	if err := udpServer.Stop(); err != nil {
		logger.Error("Error stopping UDP server", slog.String("error", err.Error()))
	}

	// Stop stream manager (cleanup sessions and stop background routines)
	streamMgr.Stop()

	// Get final statistics
	stats := udpServer.GetStatistics()
	logger.Info("Final server statistics",
		slog.Uint64("packets_received", stats.PacketsReceived),
		slog.Uint64("packets_processed", stats.PacketsProcessed),
		slog.Uint64("parse_errors", stats.ParseErrors),
		slog.Uint64("active_streams", stats.ActiveStreams),
	)

	logger.Info("Service stopped")
}

// initLogger creates and configures the structured logger based on configuration
func initLogger(cfg config.LoggingConfig) *slog.Logger {
	// Parse log level
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // default fallback
	}

	// Configure handler options
	opts := &slog.HandlerOptions{
		Level: level,
		AddSource: level == slog.LevelDebug, // Add source info for debug level
	}

	// Determine output destination
	var output *os.File
	switch cfg.Output {
	case "stderr":
		output = os.Stderr
	case "stdout", "":
		output = os.Stdout
	default:
		// Assume it's a file path
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v, falling back to stdout\n", cfg.Output, err)
			output = os.Stdout
		} else {
			output = file
		}
	}

	// Create handler based on format
	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(output, opts)
	case "text", "":
		handler = slog.NewTextHandler(output, opts)
	default:
		// Default to text format
		handler = slog.NewTextHandler(output, opts)
	}

	return slog.New(handler)
} 