package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			config: Config{
				Server: ServerConfig{
					UDPPort:              4444,
					BindAddress:          "0.0.0.0",
					BufferSize:           65536,
					MaxConcurrentStreams: 1000,
				},
				Audio: AudioConfig{
					SampleRate:       8000,
					Channels:         1,
					BitDepth:         16,
					ChunkMinDuration: 1.0,
					ChunkMaxDuration: 30.0,
					StreamTimeout:    60,
				},
				VAD: VADConfig{
					ModelPath:          "./models/silero_vad.onnx",
					Threshold:          0.5,
					WindowSize:         512,
					OverlapRatio:       0.5,
					MinSpeechDuration:  0.5,
					MinSilenceDuration: 0.3,
				},
				Transcription: TranscriptionConfig{
					Endpoint:      "https://api.example.com/transcribe",
					APIKey:        "test-key",
					Timeout:       30,
					MaxRetries:    3,
					MaxConcurrent: 10,
					OutputFormat:  "json",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			expectError: false,
		},
		{
			name: "invalid server port",
			config: Config{
				Server: ServerConfig{
					UDPPort:              70000, // Invalid port
					BindAddress:          "0.0.0.0",
					BufferSize:           65536,
					MaxConcurrentStreams: 1000,
				},
				Audio: AudioConfig{
					SampleRate:       8000,
					Channels:         1,
					BitDepth:         16,
					ChunkMinDuration: 1.0,
					ChunkMaxDuration: 30.0,
					StreamTimeout:    60,
				},
				VAD: VADConfig{
					ModelPath:          "./models/silero_vad.onnx",
					Threshold:          0.5,
					WindowSize:         512,
					OverlapRatio:       0.5,
					MinSpeechDuration:  0.5,
					MinSilenceDuration: 0.3,
				},
				Transcription: TranscriptionConfig{
					Endpoint:      "https://api.example.com/transcribe",
					APIKey:        "test-key",
					Timeout:       30,
					MaxRetries:    3,
					MaxConcurrent: 10,
					OutputFormat:  "json",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			expectError: true,
			errorMsg:    "udp_port must be between 1 and 65535",
		},
		{
			name: "invalid audio sample rate",
			config: Config{
				Server: ServerConfig{
					UDPPort:              4444,
					BindAddress:          "0.0.0.0",
					BufferSize:           65536,
					MaxConcurrentStreams: 1000,
				},
				Audio: AudioConfig{
					SampleRate:       16000, // Invalid for TLV protocol
					Channels:         1,
					BitDepth:         16,
					ChunkMinDuration: 1.0,
					ChunkMaxDuration: 30.0,
					StreamTimeout:    60,
				},
				VAD: VADConfig{
					ModelPath:          "./models/silero_vad.onnx",
					Threshold:          0.5,
					WindowSize:         512,
					OverlapRatio:       0.5,
					MinSpeechDuration:  0.5,
					MinSilenceDuration: 0.3,
				},
				Transcription: TranscriptionConfig{
					Endpoint:      "https://api.example.com/transcribe",
					APIKey:        "test-key",
					Timeout:       30,
					MaxRetries:    3,
					MaxConcurrent: 10,
					OutputFormat:  "json",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			expectError: true,
			errorMsg:    "sample_rate must be 8000 Hz",
		},
		{
			name: "invalid chunk duration",
			config: Config{
				Server: ServerConfig{
					UDPPort:              4444,
					BindAddress:          "0.0.0.0",
					BufferSize:           65536,
					MaxConcurrentStreams: 1000,
				},
				Audio: AudioConfig{
					SampleRate:       8000,
					Channels:         1,
					BitDepth:         16,
					ChunkMinDuration: 30.0, // Min greater than max
					ChunkMaxDuration: 1.0,
					StreamTimeout:    60,
				},
				VAD: VADConfig{
					ModelPath:          "./models/silero_vad.onnx",
					Threshold:          0.5,
					WindowSize:         512,
					OverlapRatio:       0.5,
					MinSpeechDuration:  0.5,
					MinSilenceDuration: 0.3,
				},
				Transcription: TranscriptionConfig{
					Endpoint:      "https://api.example.com/transcribe",
					APIKey:        "test-key",
					Timeout:       30,
					MaxRetries:    3,
					MaxConcurrent: 10,
					OutputFormat:  "json",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			expectError: true,
			errorMsg:    "chunk_max_duration",
		},
		{
			name: "invalid VAD threshold",
			config: Config{
				Server: ServerConfig{
					UDPPort:              4444,
					BindAddress:          "0.0.0.0",
					BufferSize:           65536,
					MaxConcurrentStreams: 1000,
				},
				Audio: AudioConfig{
					SampleRate:       8000,
					Channels:         1,
					BitDepth:         16,
					ChunkMinDuration: 1.0,
					ChunkMaxDuration: 30.0,
					StreamTimeout:    60,
				},
				VAD: VADConfig{
					ModelPath:          "./models/silero_vad.onnx",
					Threshold:          1.5, // Invalid threshold > 1
					WindowSize:         512,
					OverlapRatio:       0.5,
					MinSpeechDuration:  0.5,
					MinSilenceDuration: 0.3,
				},
				Transcription: TranscriptionConfig{
					Endpoint:      "https://api.example.com/transcribe",
					APIKey:        "test-key",
					Timeout:       30,
					MaxRetries:    3,
					MaxConcurrent: 10,
					OutputFormat:  "json",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			expectError: true,
			errorMsg:    "threshold must be between 0 and 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestConfigLoad(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		configYAML  string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config file",
			configYAML: `
server:
  udp_port: 4444
  bind_address: "0.0.0.0"
  buffer_size: 65536
  max_concurrent_streams: 1000
audio:
  sample_rate: 8000
  channels: 1
  bit_depth: 16
  chunk_min_duration: 1.0
  chunk_max_duration: 30.0
  stream_timeout: 60
vad:
  model_path: "./models/silero_vad.onnx"
  threshold: 0.5
  window_size: 512
  overlap_ratio: 0.5
  min_speech_duration: 0.5
  min_silence_duration: 0.3
transcription:
  endpoint: "https://api.example.com/transcribe"
  api_key: "test-key"
  timeout: 30
  max_retries: 3
  max_concurrent: 10
  output_format: "json"
logging:
  level: "info"
  format: "json"
  output: "stdout"
`,
			expectError: false,
		},
		{
			name: "invalid YAML syntax",
			configYAML: `
server:
  udp_port: 4444
  bind_address: "0.0.0.0"
  buffer_size: invalid_number
`,
			expectError: true,
			errorMsg:    "failed to parse",
		},
		{
			name: "missing required fields",
			configYAML: `
server:
  udp_port: 4444
  # missing bind_address
`,
			expectError: true,
			errorMsg:    "bind_address cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			configPath := filepath.Join(tempDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0644)
			if err != nil {
				t.Fatalf("Failed to create test config file: %v", err)
			}

			// Load configuration
			config, err := Load(configPath)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				} else if config == nil {
					t.Errorf("Expected config to be loaded but got nil")
				}
			}
		})
	}
}

func TestConfigLoadNonexistentFile(t *testing.T) {
	_, err := Load("nonexistent.yaml")
	if err == nil {
		t.Errorf("Expected error for nonexistent file but got none")
	}
	if !contains(err.Error(), "failed to read config file") {
		t.Errorf("Expected error about reading file, got: %v", err)
	}
}

func TestDurationHelpers(t *testing.T) {
	audio := AudioConfig{
		ChunkMinDuration: 1.5,
		ChunkMaxDuration: 10.0,
		StreamTimeout:    60,
	}

	if audio.GetChunkMinDuration() != 1500*time.Millisecond {
		t.Errorf("Expected 1.5 seconds, got %v", audio.GetChunkMinDuration())
	}

	if audio.GetChunkMaxDuration() != 10*time.Second {
		t.Errorf("Expected 10 seconds, got %v", audio.GetChunkMaxDuration())
	}

	if audio.GetStreamTimeoutDuration() != 60*time.Second {
		t.Errorf("Expected 60 seconds, got %v", audio.GetStreamTimeoutDuration())
	}

	vad := VADConfig{
		MinSpeechDuration:  0.5,
		MinSilenceDuration: 0.3,
	}

	if vad.GetMinSpeechDuration() != 500*time.Millisecond {
		t.Errorf("Expected 0.5 seconds, got %v", vad.GetMinSpeechDuration())
	}

	if vad.GetMinSilenceDuration() != 300*time.Millisecond {
		t.Errorf("Expected 0.3 seconds, got %v", vad.GetMinSilenceDuration())
	}

	transcription := TranscriptionConfig{
		Timeout: 30,
	}

	if transcription.GetTimeoutDuration() != 30*time.Second {
		t.Errorf("Expected 30 seconds, got %v", transcription.GetTimeoutDuration())
	}
}

func TestServerConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config ServerConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: ServerConfig{
				UDPPort:              4444,
				BindAddress:          "0.0.0.0",
				BufferSize:           65536,
				MaxConcurrentStreams: 1000,
			},
			valid: true,
		},
		{
			name: "port too low",
			config: ServerConfig{
				UDPPort:              0,
				BindAddress:          "0.0.0.0",
				BufferSize:           65536,
				MaxConcurrentStreams: 1000,
			},
			valid: false,
		},
		{
			name: "port too high",
			config: ServerConfig{
				UDPPort:              70000,
				BindAddress:          "0.0.0.0",
				BufferSize:           65536,
				MaxConcurrentStreams: 1000,
			},
			valid: false,
		},
		{
			name: "empty bind address",
			config: ServerConfig{
				UDPPort:              4444,
				BindAddress:          "",
				BufferSize:           65536,
				MaxConcurrentStreams: 1000,
			},
			valid: false,
		},
		{
			name: "buffer too small",
			config: ServerConfig{
				UDPPort:              4444,
				BindAddress:          "0.0.0.0",
				BufferSize:           512,
				MaxConcurrentStreams: 1000,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.valid && err != nil {
				t.Errorf("Expected valid config but got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("Expected invalid config but got no error")
			}
		})
	}
}

func TestLoggingConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config LoggingConfig
		valid  bool
	}{
		{
			name: "valid json to stdout",
			config: LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
			valid: true,
		},
		{
			name: "valid text to stderr",
			config: LoggingConfig{
				Level:  "debug",
				Format: "text",
				Output: "stderr",
			},
			valid: true,
		},
		{
			name: "invalid log level",
			config: LoggingConfig{
				Level:  "trace",
				Format: "json",
				Output: "stdout",
			},
			valid: false,
		},
		{
			name: "invalid format",
			config: LoggingConfig{
				Level:  "info",
				Format: "xml",
				Output: "stdout",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.valid && err != nil {
				t.Errorf("Expected valid config but got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("Expected invalid config but got no error")
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
} 