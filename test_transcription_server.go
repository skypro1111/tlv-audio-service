package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type TranscriptionResponse struct {
	ChunkID     string    `json:"chunk_id"`
	StreamID    uint32    `json:"stream_id"`
	Text        string    `json:"text"`
	Confidence  float32   `json:"confidence"`
	Language    string    `json:"language"`
	Duration    float64   `json:"duration"`
	ProcessedAt time.Time `json:"processed_at"`
}

func transcribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	// Get basic form fields
	chunkID := r.FormValue("chunk_id")
	streamID := r.FormValue("stream_id")
	duration := r.FormValue("duration")
	confidence := r.FormValue("confidence")
	
	// Get call/session metadata
	callerID := r.FormValue("caller_id")
	calledID := r.FormValue("called_id")
	channelID := r.FormValue("channel_id")
	extension := r.FormValue("extension")
	
	// Get session context
	sessionStartTime := r.FormValue("session_start_time")
	sessionDuration := r.FormValue("session_duration")
	chunkStartTime := r.FormValue("chunk_start_time")
	
	// Get request metadata
	requestID := r.FormValue("request_id")
	serviceName := r.FormValue("service_name")
	serviceVersion := r.FormValue("service_version")
	language := r.FormValue("language")

	// Get audio file
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error getting audio file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read file content to get size
	audioData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading audio file", http.StatusInternalServerError)
		return
	}

	// Log comprehensive request information
	log.Printf("🎤 TRANSCRIPTION REQUEST RECEIVED:")
	log.Printf("  ═══════════════════════════════════")
	log.Printf("  📊 Basic Info:")
	log.Printf("    Request ID: %s", requestID)
	log.Printf("    Chunk ID: %s", chunkID) 
	log.Printf("    Stream ID: %s", streamID)
	log.Printf("    Duration: %s seconds", duration)
	log.Printf("    Confidence: %s", confidence)
	log.Printf("  ═══════════════════════════════════")
	log.Printf("  📞 Call Context:")
	log.Printf("    Caller ID: %s", callerID)
	log.Printf("    Called ID: %s", calledID)
	log.Printf("    Channel ID: %s", channelID)
	log.Printf("    Extension: %s", extension)
	log.Printf("  ═══════════════════════════════════")
	log.Printf("  ⏱️  Session Context:")
	log.Printf("    Session Start: %s", sessionStartTime)
	log.Printf("    Session Duration: %s", sessionDuration)
	log.Printf("    Chunk Start: %s", chunkStartTime)
	log.Printf("  ═══════════════════════════════════")
	log.Printf("  🎧 Audio Info:")
	log.Printf("    Filename: %s", header.Filename)
	log.Printf("    Audio Size: %d bytes", len(audioData))
	log.Printf("    Content-Type: %s", header.Header.Get("Content-Type"))
	log.Printf("    Language: %s", language)
	log.Printf("  ═══════════════════════════════════")
	log.Printf("  🛠️  Service Info:")
	log.Printf("    Service: %s v%s", serviceName, serviceVersion)

	// Simulate processing time
	time.Sleep(200 * time.Millisecond)

	// Create fake transcription response
	response := TranscriptionResponse{
		ChunkID:     chunkID,
		StreamID:    parseUint32(streamID),
		Text:        "Це тестова транскрипція аудіо фрагменту з українською мовою",
		Confidence:  0.95,
		Language:    "uk",
		Duration:    parseFloat64(duration),
		ProcessedAt: time.Now(),
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("✅ TRANSCRIPTION RESPONSE SENT: '%s'", response.Text)
	log.Println("---")
}

func parseUint32(s string) uint32 {
	var val uint32
	fmt.Sscanf(s, "%d", &val)
	return val
}

func parseFloat64(s string) float64 {
	var val float64
	fmt.Sscanf(s, "%f", &val)
	return val
}

func main() {
	http.HandleFunc("/transcribe", transcribeHandler)
	
	port := ":9000"
	log.Printf("🚀 Test Transcription Server starting on port %s", port)
	log.Printf("📡 Endpoint: http://localhost%s/transcribe", port)
	log.Println("💡 Update your config to use: http://localhost:9000/transcribe")
	
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
} 