package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics contains all Prometheus metrics for the TLV audio service
type Metrics struct {
	// UDP packet metrics
	PacketsReceived  prometheus.Counter
	PacketsProcessed prometheus.Counter
	ParseErrors      prometheus.Counter
	QueueSize        prometheus.Gauge
	
	// Stream metrics
	ActiveStreams    prometheus.Gauge
	StreamsCreated   prometheus.Counter
	StreamsDestroyed prometheus.Counter
	StreamDuration   prometheus.Histogram
	
	// VAD metrics
	VADWindowsProcessed prometheus.Counter
	VADVoiceDetected    prometheus.Counter
	VADProcessingTime   prometheus.Histogram
	
	// Audio chunking metrics
	ChunksGenerated     prometheus.Counter
	ChunkDuration       prometheus.Histogram
	ChunkSize           prometheus.Histogram
	ChunkConfidence     prometheus.Histogram
	
	// Transcription metrics
	TranscriptionRequests   prometheus.Counter
	TranscriptionSuccesses  prometheus.Counter
	TranscriptionFailures   prometheus.Counter
	TranscriptionDuration   prometheus.Histogram
	TranscriptionRetries    prometheus.Counter
	
	// HTTP API metrics
	HTTPRequests        *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPErrors          *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		// UDP packet metrics
		PacketsReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_packets_received_total",
			Help: "Total number of UDP packets received",
		}),
		PacketsProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_packets_processed_total",
			Help: "Total number of UDP packets successfully processed",
		}),
		ParseErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_parse_errors_total",
			Help: "Total number of packet parsing errors",
		}),
		QueueSize: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tlv_packet_queue_size",
			Help: "Current number of packets in processing queue",
		}),
		
		// Stream metrics
		ActiveStreams: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tlv_active_streams",
			Help: "Current number of active audio streams",
		}),
		StreamsCreated: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_streams_created_total",
			Help: "Total number of streams created",
		}),
		StreamsDestroyed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_streams_destroyed_total",
			Help: "Total number of streams destroyed",
		}),
		StreamDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "tlv_stream_duration_seconds",
			Help:    "Duration of audio streams in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1s to ~17 minutes
		}),
		
		// VAD metrics
		VADWindowsProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_vad_windows_processed_total",
			Help: "Total number of VAD windows processed",
		}),
		VADVoiceDetected: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_vad_voice_detected_total",
			Help: "Total number of VAD windows with voice detected",
		}),
		VADProcessingTime: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "tlv_vad_processing_duration_seconds",
			Help:    "Time spent processing VAD windows",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		}),
		
		// Audio chunking metrics
		ChunksGenerated: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_audio_chunks_generated_total",
			Help: "Total number of audio chunks generated",
		}),
		ChunkDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "tlv_chunk_duration_seconds",
			Help:    "Duration of generated audio chunks",
			Buckets: prometheus.ExponentialBuckets(0.5, 2, 8), // 0.5s to ~2 minutes
		}),
		ChunkSize: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "tlv_chunk_size_bytes",
			Help:    "Size of generated audio chunks in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 12), // 1KB to ~4MB
		}),
		ChunkConfidence: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "tlv_chunk_confidence",
			Help:    "Confidence score of generated audio chunks",
			Buckets: prometheus.LinearBuckets(0, 0.1, 11), // 0.0 to 1.0
		}),
		
		// Transcription metrics
		TranscriptionRequests: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_transcription_requests_total",
			Help: "Total number of transcription requests sent",
		}),
		TranscriptionSuccesses: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_transcription_successes_total",
			Help: "Total number of successful transcription requests",
		}),
		TranscriptionFailures: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_transcription_failures_total",
			Help: "Total number of failed transcription requests",
		}),
		TranscriptionDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "tlv_transcription_duration_seconds",
			Help:    "Duration of transcription requests",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 100ms to ~2 minutes
		}),
		TranscriptionRetries: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tlv_transcription_retries_total",
			Help: "Total number of transcription request retries",
		}),
		
		// HTTP API metrics
		HTTPRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tlv_http_requests_total",
			Help: "Total number of HTTP requests",
		}, []string{"method", "endpoint", "status_code"}),
		HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "tlv_http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "endpoint"}),
		HTTPErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tlv_http_errors_total",
			Help: "Total number of HTTP errors",
		}, []string{"method", "endpoint", "error_type"}),
	}
}

// RecordPacketReceived increments the packets received counter
func (m *Metrics) RecordPacketReceived() {
	m.PacketsReceived.Inc()
}

// RecordPacketProcessed increments the packets processed counter
func (m *Metrics) RecordPacketProcessed() {
	m.PacketsProcessed.Inc()
}

// RecordParseError increments the parse errors counter
func (m *Metrics) RecordParseError() {
	m.ParseErrors.Inc()
}

// SetQueueSize sets the current queue size
func (m *Metrics) SetQueueSize(size int) {
	m.QueueSize.Set(float64(size))
}

// SetActiveStreams sets the current number of active streams
func (m *Metrics) SetActiveStreams(count int) {
	m.ActiveStreams.Set(float64(count))
}

// RecordStreamCreated increments the streams created counter
func (m *Metrics) RecordStreamCreated() {
	m.StreamsCreated.Inc()
}

// RecordStreamDestroyed increments the streams destroyed counter and records duration
func (m *Metrics) RecordStreamDestroyed(durationSeconds float64) {
	m.StreamsDestroyed.Inc()
	m.StreamDuration.Observe(durationSeconds)
}

// RecordVADWindow increments VAD windows processed and optionally voice detected
func (m *Metrics) RecordVADWindow(hasVoice bool, processingTimeSeconds float64) {
	m.VADWindowsProcessed.Inc()
	if hasVoice {
		m.VADVoiceDetected.Inc()
	}
	m.VADProcessingTime.Observe(processingTimeSeconds)
}

// RecordChunkGenerated records a generated audio chunk
func (m *Metrics) RecordChunkGenerated(durationSeconds float64, sizeBytes int, confidence float64) {
	m.ChunksGenerated.Inc()
	m.ChunkDuration.Observe(durationSeconds)
	m.ChunkSize.Observe(float64(sizeBytes))
	m.ChunkConfidence.Observe(confidence)
}

// RecordTranscriptionRequest increments transcription requests counter
func (m *Metrics) RecordTranscriptionRequest() {
	m.TranscriptionRequests.Inc()
}

// RecordTranscriptionSuccess records a successful transcription
func (m *Metrics) RecordTranscriptionSuccess(durationSeconds float64) {
	m.TranscriptionSuccesses.Inc()
	m.TranscriptionDuration.Observe(durationSeconds)
}

// RecordTranscriptionFailure records a failed transcription
func (m *Metrics) RecordTranscriptionFailure(durationSeconds float64) {
	m.TranscriptionFailures.Inc()
	m.TranscriptionDuration.Observe(durationSeconds)
}

// RecordTranscriptionRetry increments the retry counter
func (m *Metrics) RecordTranscriptionRetry() {
	m.TranscriptionRetries.Inc()
}

// RecordHTTPRequest records an HTTP request
func (m *Metrics) RecordHTTPRequest(method, endpoint, statusCode string, durationSeconds float64) {
	m.HTTPRequests.WithLabelValues(method, endpoint, statusCode).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, endpoint).Observe(durationSeconds)
}

// RecordHTTPError records an HTTP error
func (m *Metrics) RecordHTTPError(method, endpoint, errorType string) {
	m.HTTPErrors.WithLabelValues(method, endpoint, errorType).Inc()
} 