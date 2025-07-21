package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"sync"
	"time"
)

// Client provides HTTP client functionality for transcription API requests
type Client struct {
	config     Config
	httpClient *http.Client
	semaphore  chan struct{} // Rate limiting semaphore
	
	// Statistics
	totalRequests   uint64
	successRequests uint64
	failedRequests  uint64
	totalRetries    uint64
	avgResponseTime time.Duration
	
	mu sync.RWMutex
}

// Config contains transcription client configuration
type Config struct {
	Endpoint      string
	APIKey        string
	Timeout       time.Duration
	MaxRetries    int
	MaxConcurrent int
	OutputFormat  string // "json" or "text"
}

// AudioChunk represents an audio chunk to be transcribed (from audio package)
type AudioChunk struct {
	// Basic chunk information
	StreamID    uint32        `json:"stream_id"`
	Direction   uint8         `json:"direction"`
	ChunkID     string        `json:"chunk_id"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Duration    time.Duration `json:"duration"`
	
	// Call/Session metadata
	CallerID    string `json:"caller_id"`
	CalledID    string `json:"called_id"`
	ChannelID   string `json:"channel_id"`
	Extension   string `json:"extension"`
	
	// Audio technical details
	SampleRate  int     `json:"sample_rate"`
	AudioData   []byte  `json:"-"` // Not included in JSON, sent as file
	Format      string  `json:"format"`
	Confidence  float32 `json:"confidence"`
	StartSeq    uint32  `json:"start_seq"`
	EndSeq      uint32  `json:"end_seq"`
	
	// Stream session context
	SessionStartTime time.Time     `json:"session_start_time"`
	SessionDuration  time.Duration `json:"session_duration"`
}

// TranscriptionRequest represents a transcription request
type TranscriptionRequest struct {
	// Main audio chunk
	Chunk *AudioChunk `json:"chunk"`
	
	// Transcription parameters
	Language    string  `json:"language,omitempty"`
	Model       string  `json:"model,omitempty"`
	Prompt      string  `json:"prompt,omitempty"`
	Temperature float32 `json:"temperature,omitempty"`
	
	// Request metadata
	RequestID   string    `json:"request_id"`
	Timestamp   time.Time `json:"timestamp"`
	ServiceInfo struct {
		Version string `json:"version"`
		Name    string `json:"name"`
	} `json:"service_info"`
}

// TranscriptionResponse represents the response from the transcription API
type TranscriptionResponse struct {
	ChunkID     string    `json:"chunk_id"`
	StreamID    uint32    `json:"stream_id"`
	Text        string    `json:"text"`
	Confidence  float32   `json:"confidence"`
	Language    string    `json:"language,omitempty"`
	Segments    []Segment `json:"segments,omitempty"`
	Duration    float64   `json:"duration"`
	ProcessedAt time.Time `json:"processed_at"`
}

// Segment represents a segment of transcribed text
type Segment struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Text       string  `json:"text"`
	Confidence float32 `json:"confidence"`
}

// ClientStats represents client statistics
type ClientStats struct {
	TotalRequests   uint64        `json:"total_requests"`
	SuccessRequests uint64        `json:"success_requests"`
	FailedRequests  uint64        `json:"failed_requests"`
	SuccessRate     float64       `json:"success_rate"`
	TotalRetries    uint64        `json:"total_retries"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	ActiveRequests  int           `json:"active_requests"`
}

// NewClient creates a new transcription HTTP client
func NewClient(config Config) (*Client, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("endpoint cannot be empty")
	}
	
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}
	
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	
	if config.MaxRetries < 0 {
		config.MaxRetries = 3
	}
	
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 10
	}
	
	if config.OutputFormat == "" {
		config.OutputFormat = "json"
	}
	
	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	
	// Create semaphore for rate limiting
	semaphore := make(chan struct{}, config.MaxConcurrent)
	
	return &Client{
		config:     config,
		httpClient: httpClient,
		semaphore:  semaphore,
	}, nil
}

// Transcribe sends an audio chunk for transcription
func (c *Client) Transcribe(ctx context.Context, request *TranscriptionRequest) (*TranscriptionResponse, error) {
	// Acquire semaphore for rate limiting
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	
	startTime := time.Now()
	c.incrementTotalRequests()
	
	var lastErr error
	
	// Retry loop with exponential backoff
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.incrementTotalRetries()
			
			// Exponential backoff with jitter
			backoffTime := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			if backoffTime > 30*time.Second {
				backoffTime = 30 * time.Second
			}
			
			select {
			case <-time.After(backoffTime):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		
		response, err := c.doRequest(ctx, request)
		if err == nil {
			c.incrementSuccessRequests()
			c.updateAvgResponseTime(time.Since(startTime))
			return response, nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !c.isRetryableError(err) {
			break
		}
	}
	
	c.incrementFailedRequests()
	return nil, fmt.Errorf("transcription failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

// doRequest performs a single HTTP request to the transcription API
func (c *Client) doRequest(ctx context.Context, request *TranscriptionRequest) (*TranscriptionResponse, error) {
	// Create multipart form data
	body, contentType, err := c.createMultipartRequest(request)
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart request: %w", err)
	}
	
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Set headers
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "TLV-Audio-Service/1.0")
	
	// Perform request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}
	
	// Parse response
	var transcriptionResp TranscriptionResponse
	if err := json.Unmarshal(respBody, &transcriptionResp); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}
	
	// Set processed timestamp
	transcriptionResp.ProcessedAt = time.Now()
	
	return &transcriptionResp, nil
}

// createMultipartRequest creates a multipart/form-data request body
func (c *Client) createMultipartRequest(request *TranscriptionRequest) (io.Reader, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	
	// Add audio file
	if request.Chunk.AudioData != nil && len(request.Chunk.AudioData) > 0 {
		filename := fmt.Sprintf("%s.%s", request.Chunk.ChunkID, request.Chunk.Format)
		fileWriter, err := writer.CreateFormFile("file", filename)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create form file: %w", err)
		}
		
		if _, err := fileWriter.Write(request.Chunk.AudioData); err != nil {
			return nil, "", fmt.Errorf("failed to write audio data: %w", err)
		}
	}
	
	// Add comprehensive metadata fields
	fields := map[string]string{
		// Basic chunk information
		"chunk_id":    request.Chunk.ChunkID,
		"stream_id":   fmt.Sprintf("%d", request.Chunk.StreamID),
		"direction":   fmt.Sprintf("%d", request.Chunk.Direction),
		"sample_rate": fmt.Sprintf("%d", request.Chunk.SampleRate),
		"duration":    fmt.Sprintf("%.3f", request.Chunk.Duration.Seconds()),
		"confidence":  fmt.Sprintf("%.3f", request.Chunk.Confidence),
		"format":      request.Chunk.Format,
		"start_seq":   fmt.Sprintf("%d", request.Chunk.StartSeq),
		"end_seq":     fmt.Sprintf("%d", request.Chunk.EndSeq),
		
		// Call/Session metadata
		"caller_id":   request.Chunk.CallerID,
		"called_id":   request.Chunk.CalledID,
		"channel_id":  request.Chunk.ChannelID,
		"extension":   request.Chunk.Extension,
		
		// Session context
		"session_start_time": request.Chunk.SessionStartTime.Format(time.RFC3339),
		"session_duration":   fmt.Sprintf("%.3f", request.Chunk.SessionDuration.Seconds()),
		"chunk_start_time":   request.Chunk.StartTime.Format(time.RFC3339),
		"chunk_end_time":     request.Chunk.EndTime.Format(time.RFC3339),
		
		// Request metadata
		"request_id":     request.RequestID,
		"request_timestamp": request.Timestamp.Format(time.RFC3339),
		"service_name":   request.ServiceInfo.Name,
		"service_version": request.ServiceInfo.Version,
		
		// Configuration
		"response_format": c.config.OutputFormat,
	}
	
	// Add optional request parameters
	if request.Language != "" {
		fields["language"] = request.Language
	}
	if request.Model != "" {
		fields["model"] = request.Model
	}
	if request.Prompt != "" {
		fields["prompt"] = request.Prompt
	}
	if request.Temperature > 0 {
		fields["temperature"] = fmt.Sprintf("%.2f", request.Temperature)
	}
	
	// Write all form fields
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("failed to write field %s: %w", key, err)
		}
	}
	
	// Close writer
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}
	
	return &buf, writer.FormDataContentType(), nil
}

// isRetryableError determines if an error is retryable
func (c *Client) isRetryableError(err error) bool {
	// In a real implementation, you'd check for specific error types
	// For now, assume network errors and 5xx HTTP errors are retryable
	
	// Check for timeout errors
	if err == context.DeadlineExceeded {
		return true
	}
	
	// Check for HTTP errors (simplified)
	errStr := err.Error()
	
	// 5xx server errors are retryable
	if bytes.Contains([]byte(errStr), []byte("HTTP error 5")) {
		return true
	}
	
	// Rate limiting (429) is retryable
	if bytes.Contains([]byte(errStr), []byte("HTTP error 429")) {
		return true
	}
	
	// Network/connection errors are typically retryable
	if bytes.Contains([]byte(errStr), []byte("connection")) ||
	   bytes.Contains([]byte(errStr), []byte("timeout")) ||
	   bytes.Contains([]byte(errStr), []byte("refused")) {
		return true
	}
	
	return false
}

// Statistics methods
func (c *Client) incrementTotalRequests() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalRequests++
}

func (c *Client) incrementSuccessRequests() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.successRequests++
}

func (c *Client) incrementFailedRequests() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failedRequests++
}

func (c *Client) incrementTotalRetries() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalRetries++
}

func (c *Client) updateAvgResponseTime(responseTime time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Simple moving average
	if c.avgResponseTime == 0 {
		c.avgResponseTime = responseTime
	} else {
		c.avgResponseTime = (c.avgResponseTime + responseTime) / 2
	}
}

// GetStats returns current client statistics
func (c *Client) GetStats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	successRate := float64(0)
	if c.totalRequests > 0 {
		successRate = float64(c.successRequests) / float64(c.totalRequests) * 100
	}
	
	activeRequests := c.config.MaxConcurrent - len(c.semaphore)
	
	return ClientStats{
		TotalRequests:   c.totalRequests,
		SuccessRequests: c.successRequests,
		FailedRequests:  c.failedRequests,
		SuccessRate:     successRate,
		TotalRetries:    c.totalRetries,
		AvgResponseTime: c.avgResponseTime,
		ActiveRequests:  activeRequests,
	}
}

// Close gracefully shuts down the client
func (c *Client) Close() error {
	// Wait for all active requests to complete
	for i := 0; i < c.config.MaxConcurrent; i++ {
		c.semaphore <- struct{}{}
	}
	
	return nil
} 