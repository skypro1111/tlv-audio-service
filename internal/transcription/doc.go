// Package transcription implements the HTTP client for the transcription API.
// It handles multipart form data requests with audio chunks and metadata,
// implements retry logic with exponential backoff, and manages rate limiting.
package transcription 