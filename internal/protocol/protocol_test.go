package protocol

import (
	"encoding/binary"
	"testing"
)

func TestParseHeader(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expected    *Header
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid signaling header",
			data: []byte{
				0x01,       // PacketType: Signaling
				0x00, 0xAC, // PacketLen: 172 (8 + 164)
				0x00, 0x00, 0x30, 0x39, // StreamID: 12345
				0x01, // Direction: RX
			},
			expected: &Header{
				PacketType: PacketTypeSignaling,
				PacketLen:  172,
				StreamID:   12345,
				Direction:  DirectionRX,
			},
			expectError: false,
		},
		{
			name: "valid audio header",
			data: []byte{
				0x02,       // PacketType: Audio
				0x01, 0x00, // PacketLen: 256
				0x12, 0x34, 0x56, 0x78, // StreamID: 305419896
				0x02, // Direction: TX
			},
			expected: &Header{
				PacketType: PacketTypeAudio,
				PacketLen:  256,
				StreamID:   305419896,
				Direction:  DirectionTX,
			},
			expectError: false,
		},
		{
			name:        "header too short",
			data:        []byte{0x01, 0x00},
			expected:    nil,
			expectError: true,
			errorMsg:    "header too short",
		},
		{
			name:        "empty data",
			data:        []byte{},
			expected:    nil,
			expectError: true,
			errorMsg:    "header too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseHeader(tt.data)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				} else if !headersEqual(result, tt.expected) {
					t.Errorf("Expected header %+v, got %+v", tt.expected, result)
				}
			}
		})
	}
}

func TestParseSignalingPayload(t *testing.T) {
	// Create test signaling payload (164 bytes total)
	data := make([]byte, SignalingPayloadSize)
	
	// ChannelID (64 bytes) - "SIP/1001-00000001"
	channelID := "SIP/1001-00000001"
	copy(data[0:], []byte(channelID))
	
	// Extension (32 bytes) - "1001"
	extension := "1001"
	copy(data[64:], []byte(extension))
	
	// CallerID (32 bytes) - "John Doe"
	callerID := "John Doe"
	copy(data[96:], []byte(callerID))
	
	// CalledID (32 bytes) - "Jane Smith"
	calledID := "Jane Smith"
	copy(data[128:], []byte(calledID))
	
	// Timestamp (4 bytes) - 1701234567
	binary.BigEndian.PutUint32(data[160:], 1701234567)

	tests := []struct {
		name        string
		data        []byte
		expectError bool
		errorMsg    string
		validate    func(*SignalingPayload) bool
	}{
		{
			name:        "valid signaling payload",
			data:        data,
			expectError: false,
			validate: func(p *SignalingPayload) bool {
				return p.GetChannelID() == channelID &&
					p.GetExtension() == extension &&
					p.GetCallerID() == callerID &&
					p.GetCalledID() == calledID &&
					p.Timestamp == 1701234567
			},
		},
		{
			name:        "payload too short",
			data:        data[:100],
			expectError: true,
			errorMsg:    "signaling payload too short",
		},
		{
			name:        "empty payload",
			data:        []byte{},
			expectError: true,
			errorMsg:    "signaling payload too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSignalingPayload(tt.data)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				} else if tt.validate != nil && !tt.validate(result) {
					t.Errorf("Validation failed for result: %+v", result)
				}
			}
		})
	}
}

func TestParseAudioPayload(t *testing.T) {
	// Create test audio data
	audioData := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	
	// Create complete payload: sequence + audio data
	data := make([]byte, 4+len(audioData))
	binary.BigEndian.PutUint32(data[0:], 12345) // Sequence number
	copy(data[4:], audioData)

	tests := []struct {
		name        string
		data        []byte
		expectError bool
		errorMsg    string
		validate    func(*AudioPayload) bool
	}{
		{
			name:        "valid audio payload with data",
			data:        data,
			expectError: false,
			validate: func(p *AudioPayload) bool {
				return p.Sequence == 12345 && 
					len(p.AudioData) == len(audioData) &&
					bytesEqual(p.AudioData, audioData)
			},
		},
		{
			name:        "audio payload with sequence only",
			data:        []byte{0x00, 0x00, 0x00, 0x01}, // Just sequence, no audio data
			expectError: false,
			validate: func(p *AudioPayload) bool {
				return p.Sequence == 1 && len(p.AudioData) == 0
			},
		},
		{
			name:        "payload too short",
			data:        []byte{0x00, 0x00}, // Only 2 bytes
			expectError: true,
			errorMsg:    "audio payload too short",
		},
		{
			name:        "empty payload",
			data:        []byte{},
			expectError: true,
			errorMsg:    "audio payload too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAudioPayload(tt.data)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				} else if tt.validate != nil && !tt.validate(result) {
					t.Errorf("Validation failed for result: %+v", result)
				}
			}
		})
	}
}

func TestParsePacket(t *testing.T) {
	// Create a valid signaling packet
	signalingData := createTestSignalingPacket(t)
	
	// Create a valid audio packet
	audioData := createTestAudioPacket(t)

	tests := []struct {
		name        string
		data        []byte
		expectError bool
		errorMsg    string
		validate    func(*ParsedPacket) bool
	}{
		{
			name:        "valid signaling packet",
			data:        signalingData,
			expectError: false,
			validate: func(p *ParsedPacket) bool {
				return p.Header != nil &&
					p.Header.PacketType == PacketTypeSignaling &&
					p.Signaling != nil &&
					p.Audio == nil
			},
		},
		{
			name:        "valid audio packet",
			data:        audioData,
			expectError: false,
			validate: func(p *ParsedPacket) bool {
				return p.Header != nil &&
					p.Header.PacketType == PacketTypeAudio &&
					p.Audio != nil &&
					p.Signaling == nil
			},
		},
		{
			name:        "packet too short",
			data:        []byte{0x01, 0x00},
			expectError: true,
			errorMsg:    "packet too short",
		},
		{
			name:        "invalid packet type",
			data:        createInvalidPacketTypePacket(),
			expectError: true,
			errorMsg:    "invalid packet type",
		},
		{
			name:        "packet length mismatch",
			data:        createPacketLengthMismatch(),
			expectError: true,
			errorMsg:    "packet length mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePacket(tt.data)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				} else if tt.validate != nil && !tt.validate(result) {
					t.Errorf("Validation failed for result: %+v", result)
				}
			}
		})
	}
}

func TestValidateHeader(t *testing.T) {
	tests := []struct {
		name        string
		header      *Header
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid signaling header",
			header: &Header{
				PacketType: PacketTypeSignaling,
				PacketLen:  172, // 8 + 164
				StreamID:   12345,
				Direction:  DirectionRX,
			},
			expectError: false,
		},
		{
			name: "valid audio header",
			header: &Header{
				PacketType: PacketTypeAudio,
				PacketLen:  100,
				StreamID:   67890,
				Direction:  DirectionTX,
			},
			expectError: false,
		},
		{
			name: "invalid packet type",
			header: &Header{
				PacketType: 0x99,
				PacketLen:  172,
				StreamID:   12345,
				Direction:  DirectionRX,
			},
			expectError: true,
			errorMsg:    "invalid packet type",
		},
		{
			name: "invalid direction",
			header: &Header{
				PacketType: PacketTypeSignaling,
				PacketLen:  172,
				StreamID:   12345,
				Direction:  0x99,
			},
			expectError: true,
			errorMsg:    "invalid direction",
		},
		{
			name: "packet length too small",
			header: &Header{
				PacketType: PacketTypeSignaling,
				PacketLen:  5, // Less than header size
				StreamID:   12345,
				Direction:  DirectionRX,
			},
			expectError: true,
			errorMsg:    "packet length too small",
		},
		{
			name: "signaling packet wrong payload size",
			header: &Header{
				PacketType: PacketTypeSignaling,
				PacketLen:  100, // Wrong size for signaling
				StreamID:   12345,
				Direction:  DirectionRX,
			},
			expectError: true,
			errorMsg:    "signaling packet payload size mismatch",
		},
		{
			name: "audio packet payload too small",
			header: &Header{
				PacketType: PacketTypeAudio,
				PacketLen:  10, // Too small for audio payload
				StreamID:   12345,
				Direction:  DirectionRX,
			},
			expectError: true,
			errorMsg:    "audio packet payload too small",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHeader(tt.header)

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

func TestIsValidPacketType(t *testing.T) {
	tests := []struct {
		packetType uint8
		expected   bool
	}{
		{PacketTypeSignaling, true},
		{PacketTypeAudio, true},
		{0x00, false},
		{0x03, false},
		{0xFF, false},
	}

	for _, tt := range tests {
		result := IsValidPacketType(tt.packetType)
		if result != tt.expected {
			t.Errorf("IsValidPacketType(0x%02x) = %v, expected %v", tt.packetType, result, tt.expected)
		}
	}
}

func TestIsValidDirection(t *testing.T) {
	tests := []struct {
		direction uint8
		expected  bool
	}{
		{DirectionRX, true},
		{DirectionTX, true},
		{0x00, false},
		{0x03, false},
		{0xFF, false},
	}

	for _, tt := range tests {
		result := IsValidDirection(tt.direction)
		if result != tt.expected {
			t.Errorf("IsValidDirection(0x%02x) = %v, expected %v", tt.direction, result, tt.expected)
		}
	}
}

func TestExtractString(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "normal string with null terminator",
			input:    []byte("hello\x00world\x00\x00\x00"),
			expected: "hello",
		},
		{
			name:     "string without null terminator",
			input:    []byte("hello"),
			expected: "hello",
		},
		{
			name:     "empty string",
			input:    []byte("\x00\x00\x00\x00"),
			expected: "",
		},
		{
			name:     "string with unicode",
			input:    []byte("héllo\x00test"),
			expected: "héllo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractString(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractString(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSignalingPayloadGetters(t *testing.T) {
	payload := &SignalingPayload{}
	
	// Set test data
	copy(payload.ChannelID[:], []byte("SIP/1001-00000001"))
	copy(payload.Extension[:], []byte("1001"))
	copy(payload.CallerID[:], []byte("John Doe"))
	copy(payload.CalledID[:], []byte("Jane Smith"))
	payload.Timestamp = 1701234567

	// Test getters
	if payload.GetChannelID() != "SIP/1001-00000001" {
		t.Errorf("GetChannelID() = %q, expected %q", payload.GetChannelID(), "SIP/1001-00000001")
	}
	if payload.GetExtension() != "1001" {
		t.Errorf("GetExtension() = %q, expected %q", payload.GetExtension(), "1001")
	}
	if payload.GetCallerID() != "John Doe" {
		t.Errorf("GetCallerID() = %q, expected %q", payload.GetCallerID(), "John Doe")
	}
	if payload.GetCalledID() != "Jane Smith" {
		t.Errorf("GetCalledID() = %q, expected %q", payload.GetCalledID(), "Jane Smith")
	}
}

func TestStringMethods(t *testing.T) {
	// Test Header.String()
	header := &Header{
		PacketType: PacketTypeSignaling,
		PacketLen:  172,
		StreamID:   12345,
		Direction:  DirectionRX,
	}
	headerStr := header.String()
	if !contains(headerStr, "Signaling") || !contains(headerStr, "12345") || !contains(headerStr, "RX") {
		t.Errorf("Header.String() missing expected content: %s", headerStr)
	}

	// Test SignalingPayload.String()
	signaling := &SignalingPayload{}
	copy(signaling.ChannelID[:], []byte("SIP/1001"))
	copy(signaling.Extension[:], []byte("1001"))
	signalingStr := signaling.String()
	if !contains(signalingStr, "SIP/1001") || !contains(signalingStr, "1001") {
		t.Errorf("SignalingPayload.String() missing expected content: %s", signalingStr)
	}

	// Test AudioPayload.String()
	audio := &AudioPayload{
		Sequence:  12345,
		AudioData: make([]byte, 160),
	}
	audioStr := audio.String()
	if !contains(audioStr, "12345") || !contains(audioStr, "160") {
		t.Errorf("AudioPayload.String() missing expected content: %s", audioStr)
	}
}

// Helper functions for tests

func createTestSignalingPacket(t *testing.T) []byte {
	t.Helper()
	
	// Create header
	header := make([]byte, HeaderSize)
	header[0] = PacketTypeSignaling
	binary.BigEndian.PutUint16(header[1:], HeaderSize+SignalingPayloadSize)
	binary.BigEndian.PutUint32(header[3:], 12345)
	header[7] = DirectionRX

	// Create payload
	payload := make([]byte, SignalingPayloadSize)
	copy(payload[0:], []byte("SIP/1001-00000001"))
	copy(payload[64:], []byte("1001"))
	copy(payload[96:], []byte("John Doe"))
	copy(payload[128:], []byte("Jane Smith"))
	binary.BigEndian.PutUint32(payload[160:], 1701234567)

	// Combine header and payload
	packet := append(header, payload...)
	return packet
}

func createTestAudioPacket(t *testing.T) []byte {
	t.Helper()
	
	audioData := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	packetLen := HeaderSize + AudioPayloadHeaderSize + len(audioData)

	// Create header
	header := make([]byte, HeaderSize)
	header[0] = PacketTypeAudio
	binary.BigEndian.PutUint16(header[1:], uint16(packetLen))
	binary.BigEndian.PutUint32(header[3:], 67890)
	header[7] = DirectionTX

	// Create payload
	payload := make([]byte, AudioPayloadHeaderSize+len(audioData))
	binary.BigEndian.PutUint32(payload[0:], 12345) // Sequence
	copy(payload[4:], audioData)

	// Combine header and payload
	packet := append(header, payload...)
	return packet
}

func createInvalidPacketTypePacket() []byte {
	data := make([]byte, HeaderSize+4)
	data[0] = 0x99 // Invalid packet type
	binary.BigEndian.PutUint16(data[1:], uint16(len(data)))
	binary.BigEndian.PutUint32(data[3:], 12345)
	data[7] = DirectionRX
	return data
}

func createPacketLengthMismatch() []byte {
	data := make([]byte, HeaderSize+4)
	data[0] = PacketTypeAudio
	binary.BigEndian.PutUint16(data[1:], 999) // Wrong length
	binary.BigEndian.PutUint32(data[3:], 12345)
	data[7] = DirectionRX
	return data
}

func headersEqual(a, b *Header) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.PacketType == b.PacketType &&
		a.PacketLen == b.PacketLen &&
		a.StreamID == b.StreamID &&
		a.Direction == b.Direction
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

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