package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete service configuration
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	HTTP          HTTPConfig          `yaml:"http"`
	Audio         AudioConfig         `yaml:"audio"`
	VAD           VADConfig           `yaml:"vad"`
	Transcription TranscriptionConfig `yaml:"transcription"`
	Logging       LoggingConfig       `yaml:"logging"`
}

// ServerConfig contains UDP server configuration
type ServerConfig struct {
	UDPPort              int    `yaml:"udp_port"`
	BindAddress          string `yaml:"bind_address"`
	BufferSize           int    `yaml:"buffer_size"`
	MaxConcurrentStreams int    `yaml:"max_concurrent_streams"`
}

// HTTPConfig contains HTTP API server configuration
type HTTPConfig struct {
	Port    int    `yaml:"port"`
	Address string `yaml:"address"`
	Enabled bool   `yaml:"enabled"`
}

// AudioConfig contains audio processing parameters
type AudioConfig struct {
	SampleRate         int     `yaml:"sample_rate"`
	Channels           int     `yaml:"channels"`
	BitDepth           int     `yaml:"bit_depth"`
	ChunkMinDuration   float64 `yaml:"chunk_min_duration"`   // seconds
	ChunkMaxDuration   float64 `yaml:"chunk_max_duration"`   // seconds
	StreamTimeout      int     `yaml:"stream_timeout"`       // seconds
}

// VADConfig contains Voice Activity Detection configuration
type VADConfig struct {
	ModelPath           string  `yaml:"model_path"`
	Threshold           float32 `yaml:"threshold"`
	WindowSize          int     `yaml:"window_size"`           // samples
	OverlapRatio        float64 `yaml:"overlap_ratio"`
	MinSpeechDuration   float64 `yaml:"min_speech_duration"`   // seconds
	MinSilenceDuration  float64 `yaml:"min_silence_duration"`  // seconds
}

// TranscriptionConfig contains transcription API configuration
type TranscriptionConfig struct {
	Endpoint       string `yaml:"endpoint"`
	APIKey         string `yaml:"api_key"`
	Timeout        int    `yaml:"timeout"`         // seconds
	MaxRetries     int    `yaml:"max_retries"`
	MaxConcurrent  int    `yaml:"max_concurrent"`
	OutputFormat   string `yaml:"output_format"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// Validate performs comprehensive validation of the configuration
func (c *Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	if err := c.HTTP.Validate(); err != nil {
		return fmt.Errorf("http config: %w", err)
	}

	if err := c.Audio.Validate(); err != nil {
		return fmt.Errorf("audio config: %w", err)
	}

	if err := c.VAD.Validate(); err != nil {
		return fmt.Errorf("vad config: %w", err)
	}

	if err := c.Transcription.Validate(); err != nil {
		return fmt.Errorf("transcription config: %w", err)
	}

	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging config: %w", err)
	}

	return nil
}

// Validate validates server configuration
func (s *ServerConfig) Validate() error {
	if s.UDPPort < 1 || s.UDPPort > 65535 {
		return fmt.Errorf("udp_port must be between 1 and 65535, got %d", s.UDPPort)
	}

	if s.BindAddress == "" {
		return fmt.Errorf("bind_address cannot be empty")
	}

	if s.BufferSize < 1024 {
		return fmt.Errorf("buffer_size must be at least 1024 bytes, got %d", s.BufferSize)
	}

	if s.MaxConcurrentStreams < 1 {
		return fmt.Errorf("max_concurrent_streams must be at least 1, got %d", s.MaxConcurrentStreams)
	}

	return nil
}

// Validate validates HTTP configuration
func (h *HTTPConfig) Validate() error {
	if h.Enabled {
		if h.Port < 1 || h.Port > 65535 {
			return fmt.Errorf("http port must be between 1 and 65535, got %d", h.Port)
		}
		
		if h.Address == "" {
			return fmt.Errorf("http address cannot be empty when HTTP is enabled")
		}
	}
	
	return nil
}

// Validate validates audio configuration
func (a *AudioConfig) Validate() error {
	if a.SampleRate != 8000 {
		return fmt.Errorf("sample_rate must be 8000 Hz for TLV protocol, got %d", a.SampleRate)
	}

	if a.Channels != 1 {
		return fmt.Errorf("channels must be 1 (mono) for TLV protocol, got %d", a.Channels)
	}

	if a.BitDepth != 16 {
		return fmt.Errorf("bit_depth must be 16 for TLV protocol, got %d", a.BitDepth)
	}

	if a.ChunkMinDuration <= 0 {
		return fmt.Errorf("chunk_min_duration must be positive, got %f", a.ChunkMinDuration)
	}

	if a.ChunkMaxDuration <= a.ChunkMinDuration {
		return fmt.Errorf("chunk_max_duration (%f) must be greater than chunk_min_duration (%f)", 
			a.ChunkMaxDuration, a.ChunkMinDuration)
	}

	if a.StreamTimeout < 1 {
		return fmt.Errorf("stream_timeout must be at least 1 second, got %d", a.StreamTimeout)
	}

	return nil
}

// Validate validates VAD configuration
func (v *VADConfig) Validate() error {
	if v.ModelPath == "" {
		return fmt.Errorf("model_path cannot be empty")
	}

	if v.Threshold < 0 || v.Threshold > 1 {
		return fmt.Errorf("threshold must be between 0 and 1, got %f", v.Threshold)
	}

	if v.WindowSize < 256 || v.WindowSize > 2048 {
		return fmt.Errorf("window_size must be between 256 and 2048 samples, got %d", v.WindowSize)
	}

	if v.OverlapRatio < 0 || v.OverlapRatio >= 1 {
		return fmt.Errorf("overlap_ratio must be between 0 and 1 (exclusive), got %f", v.OverlapRatio)
	}

	if v.MinSpeechDuration <= 0 {
		return fmt.Errorf("min_speech_duration must be positive, got %f", v.MinSpeechDuration)
	}

	if v.MinSilenceDuration <= 0 {
		return fmt.Errorf("min_silence_duration must be positive, got %f", v.MinSilenceDuration)
	}

	return nil
}

// Validate validates transcription configuration
func (t *TranscriptionConfig) Validate() error {
	if t.Endpoint == "" {
		return fmt.Errorf("endpoint cannot be empty")
	}

	if t.APIKey == "" {
		return fmt.Errorf("api_key cannot be empty")
	}

	if t.Timeout < 1 {
		return fmt.Errorf("timeout must be at least 1 second, got %d", t.Timeout)
	}

	if t.MaxRetries < 0 {
		return fmt.Errorf("max_retries cannot be negative, got %d", t.MaxRetries)
	}

	if t.MaxConcurrent < 1 {
		return fmt.Errorf("max_concurrent must be at least 1, got %d", t.MaxConcurrent)
	}

	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[t.OutputFormat] {
		return fmt.Errorf("output_format must be 'json' or 'text', got '%s'", t.OutputFormat)
	}

	return nil
}

// Validate validates logging configuration
func (l *LoggingConfig) Validate() error {
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLevels[l.Level] {
		return fmt.Errorf("level must be one of [debug, info, warn, error], got '%s'", l.Level)
	}

	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[l.Format] {
		return fmt.Errorf("format must be 'json' or 'text', got '%s'", l.Format)
	}

	validOutputs := map[string]bool{"stdout": true, "stderr": true}
	if !validOutputs[l.Output] && l.Output != "" {
		// Allow file paths, but validate common outputs
		if l.Output != "stdout" && l.Output != "stderr" {
			// Assume it's a file path, which is valid
		}
	}

	return nil
}

// GetStreamTimeoutDuration returns the stream timeout as a time.Duration
func (a *AudioConfig) GetStreamTimeoutDuration() time.Duration {
	return time.Duration(a.StreamTimeout) * time.Second
}

// GetChunkMinDuration returns the minimum chunk duration as a time.Duration
func (a *AudioConfig) GetChunkMinDuration() time.Duration {
	return time.Duration(a.ChunkMinDuration * float64(time.Second))
}

// GetChunkMaxDuration returns the maximum chunk duration as a time.Duration
func (a *AudioConfig) GetChunkMaxDuration() time.Duration {
	return time.Duration(a.ChunkMaxDuration * float64(time.Second))
}

// GetMinSpeechDuration returns the minimum speech duration as a time.Duration
func (v *VADConfig) GetMinSpeechDuration() time.Duration {
	return time.Duration(v.MinSpeechDuration * float64(time.Second))
}

// GetMinSilenceDuration returns the minimum silence duration as a time.Duration
func (v *VADConfig) GetMinSilenceDuration() time.Duration {
	return time.Duration(v.MinSilenceDuration * float64(time.Second))
}

// GetTimeoutDuration returns the transcription timeout as a time.Duration
func (t *TranscriptionConfig) GetTimeoutDuration() time.Duration {
	return time.Duration(t.Timeout) * time.Second
} 