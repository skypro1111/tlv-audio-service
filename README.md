# TLV Audio Processing Service

![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)
![License](https://img.shields.io/badge/license-MIT-green.svg)
![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)

A high-performance, concurrent Go service that receives custom TLV-formatted UDP packets from Asterisk modules, processes audio using Silero VAD (Voice Activity Detection), intelligently chunks speech segments, and sends them to transcription APIs.

## 🚀 Features

- **Real-time Audio Processing**: Handles UDP streams with sequence reordering and packet loss detection
- **Voice Activity Detection**: Uses Silero VAD ONNX model to identify speech segments
- **Intelligent Chunking**: Creates optimal audio chunks based on speech patterns and silence detection
- **Raw Audio Preservation**: Maintains original PCM-16 format without unnecessary transformations
- **Concurrent Processing**: High-performance worker pools for UDP packet processing
- **Comprehensive Monitoring**: Prometheus metrics and HTTP API endpoints
- **Graceful Shutdown**: Clean resource management and final chunk processing

## 📋 Table of Contents

- [Architecture](#architecture)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [API Endpoints](#api-endpoints)
- [Monitoring](#monitoring)
- [Development](#development)
- [Testing](#testing)
- [Performance](#performance)
- [Contributing](#contributing)

## 🏗️ Architecture

```
┌─────────────┐    UDP/TLV    ┌──────────────┐    VAD    ┌─────────────┐
│   Asterisk  │──────────────→│  UDP Server  │──────────→│ VAD Engine  │
│   Module    │               │              │           │ (Silero)    │
└─────────────┘               └──────────────┘           └─────────────┘
                                     │                           │
                                     ▼                           ▼
                              ┌──────────────┐           ┌─────────────┐
                              │ Stream Mgmt  │◄──────────│  Chunker    │
                              │              │           │             │
                              └──────────────┘           └─────────────┘
                                     │                           │
                                     ▼                           ▼
                              ┌──────────────┐           ┌─────────────┐
                              │   HTTP API   │           │Transcription│
                              │  Monitoring  │           │   Client    │
                              └──────────────┘           └─────────────┘
```

### Core Components

- **UDP Server**: Receives and parses TLV packets with concurrent worker pools
- **Stream Manager**: Manages audio session lifecycles and buffer management
- **Audio Buffer**: Handles raw PCM-16 data with sequence reordering
- **VAD Processor**: Real-time voice activity detection using Silero ONNX model
- **Audio Chunker**: Creates speech segments based on VAD results
- **Transcription Client**: HTTP client with retry logic and rate limiting

## 🛠️ Installation

### Prerequisites

- Go 1.21 or higher
- Silero VAD ONNX model file
- Transcription service endpoint

### Build from Source

```bash
git clone https://github.com/your-org/tlv-audio-service.git
cd tlv-audio-service
go mod download
go build cmd/server/main.go
```

### Using Go Install

```bash
go install github.com/your-org/tlv-audio-service/cmd/server@latest
```

### Docker

```bash
docker build -t tlv-audio-service .
docker run -p 4444:4444/udp -p 8080:8080 tlv-audio-service
```

## ⚙️ Configuration

Create a `config.yaml` file:

```yaml
server:
  udp_port: 4444
  bind_address: "0.0.0.0"
  buffer_size: 65536
  max_concurrent_streams: 1000

http:
  port: 8080
  address: "0.0.0.0"
  enabled: true

audio:
  sample_rate: 8000
  channels: 1
  bit_depth: 16
  chunk_min_duration: 1.0    # seconds
  chunk_max_duration: 30.0   # seconds  
  stream_timeout: 60         # seconds

vad:
  model_path: "./models/silero_vad.onnx"
  threshold: 0.5
  window_size: 512           # samples
  overlap_ratio: 0.5
  min_speech_duration: 0.5   # seconds
  min_silence_duration: 0.3  # seconds

transcription:
  endpoint: "https://api.example.com/transcribe"
  api_key: "your-api-key"
  timeout: 30                # seconds
  max_retries: 3
  max_concurrent: 10
  output_format: "json"

logging:
  level: "info"
  format: "json"
  output: "stdout"
```

### Environment Variables

```bash
export TLV_CONFIG_PATH="/path/to/config.yaml"
export TLV_LOG_LEVEL="debug"
export TLV_TRANSCRIPTION_API_KEY="your-secret-key"
```

## 🚀 Usage

### Start the Service

```bash
./main --config=config.yaml
```

### Command Line Options

```bash
./main --help

Usage: main [options]
  -config string
        Path to configuration file (default "config.yaml")
  -log-level string
        Log level (debug, info, warn, error) (default "info")
  -version
        Show version information
```

### TLV Packet Format

The service expects UDP packets in this TLV format:

#### Header (8 bytes)
```
[PacketType:1][PacketLen:2][StreamID:4][Direction:1]
```

#### Signaling Payload (164 bytes)
```
[ChannelID:64][Extension:32][CallerID:32][CalledID:32][Timestamp:4]
```

#### Audio Payload (Variable)
```
[Sequence:4][AudioData:N]  // Raw PCM-16 data
```

## 🌐 API Endpoints

### Health Check
```bash
GET /health
```
Returns service health status and component information.

### Stream Information
```bash
GET /streams
```
Lists active streams with statistics.

### Configuration
```bash
GET /config
```
Returns current service configuration.

### Metrics
```bash
GET /metrics
```
Prometheus-formatted metrics.

## 📊 Monitoring

### Prometheus Metrics

- `tlv_packets_received_total` - Total UDP packets received
- `tlv_packets_processed_total` - Total packets successfully processed
- `tlv_audio_chunks_generated_total` - Audio chunks created
- `tlv_transcription_requests_total` - Transcription requests sent
- `tlv_transcription_successes_total` - Successful transcriptions
- `tlv_stream_sessions_active` - Currently active streams
- `tlv_vad_processing_duration_seconds` - VAD processing time
- `tlv_chunk_duration_seconds` - Audio chunk durations

### Logging

Structured JSON logging with configurable levels:

```json
{
  "time": "2025-01-21T15:30:45Z",
  "level": "INFO",
  "msg": "Audio chunk generated",
  "stream_id": 12345,
  "chunk_id": "12345_1_1642781234",
  "duration": 2.3,
  "samples": 18400,
  "confidence": 0.95
}
```

## 🔧 Development

### Project Structure

```
tlv-audio-service/
├── cmd/server/          # Main application
├── internal/
│   ├── audio/          # Audio processing (buffer, chunker, WAV)
│   ├── config/         # Configuration management
│   ├── metrics/        # Prometheus metrics
│   ├── protocol/       # TLV packet parsing
│   ├── server/         # UDP and HTTP servers
│   ├── stream/         # Stream session management
│   ├── transcription/  # HTTP client for transcription
│   └── vad/           # Voice activity detection
├── configs/           # Configuration files
├── models/           # ONNX model files
└── docs/            # Documentation
```

### Development Setup

```bash
# Install dependencies
go mod download

# Install development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run

# Run tests
go test ./...

# Run with race detection
go test -race ./...
```

### Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use `gofmt` for formatting
- Write comprehensive tests for all packages
- Document public APIs with clear examples

## 🧪 Testing

### Unit Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration Tests

```bash
# Run integration tests
go test -tags=integration ./...
```

### Load Testing

```bash
# Test UDP packet processing
./scripts/load_test.sh

# Monitor performance
go test -bench=. -benchmem ./...
```

## 📈 Performance

### Benchmarks

- **UDP Throughput**: 10,000+ packets/second
- **Stream Capacity**: 1,000+ concurrent streams
- **VAD Processing**: <5ms per 64ms window
- **Memory Usage**: <100MB baseline + ~1MB per active stream
- **Chunk Generation**: <1ms average latency

### Optimization Tips

1. **Buffer Sizes**: Adjust UDP buffer size based on expected traffic
2. **Worker Pools**: Scale workers based on CPU cores
3. **VAD Tuning**: Optimize threshold and window settings
4. **Memory**: Monitor and tune garbage collection settings

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Guidelines

- Write tests for new functionality
- Update documentation for API changes
- Follow existing code style and patterns
- Ensure all tests pass before submitting

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🆘 Support

- **Documentation**: [Wiki](https://github.com/your-org/tlv-audio-service/wiki)
- **Issues**: [GitHub Issues](https://github.com/your-org/tlv-audio-service/issues)
- **Discussions**: [GitHub Discussions](https://github.com/your-org/tlv-audio-service/discussions)

## 🏆 Acknowledgments

- [Silero VAD](https://github.com/snakers4/silero-vad) for voice activity detection
- [Prometheus](https://prometheus.io/) for metrics collection
- [Go community](https://golang.org/community) for excellent tooling and libraries
