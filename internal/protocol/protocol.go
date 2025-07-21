package protocol

import (
	"encoding/binary"
	"fmt"
)

// Protocol constants from specification
const (
	// Packet types
	PacketTypeSignaling = 0x01
	PacketTypeAudio     = 0x02

	// Direction types
	DirectionRX = 0x01 // Received audio
	DirectionTX = 0x02 // Transmitted audio

	// Packet structure sizes
	HeaderSize          = 8   // 1 + 2 + 4 + 1 bytes
	SignalingPayloadSize = 164 // 64 + 32 + 32 + 32 + 4 bytes
	AudioPayloadHeaderSize = 4   // Sequence number (4 bytes)
	
	// String field sizes in signaling payload
	ChannelIDSize = 64
	ExtensionSize = 32
	CallerIDSize  = 32
	CalledIDSize  = 32
	TimestampSize = 4
)

// Header represents the 8-byte TLV packet header
// Layout: [PacketType:1][PacketLen:2][StreamID:4][Direction:1]
type Header struct {
	PacketType uint8  // 0x01=Signaling, 0x02=Audio
	PacketLen  uint16 // Total packet size (header + payload)
	StreamID   uint32 // Unique stream identifier
	Direction  uint8  // 0x01=RX, 0x02=TX
}

// SignalingPayload represents the 164-byte signaling packet payload
// Layout: [ChannelID:64][Extension:32][CallerID:32][CalledID:32][Timestamp:4]
type SignalingPayload struct {
	ChannelID [ChannelIDSize]byte // Null-terminated string (64 bytes)
	Extension [ExtensionSize]byte // Null-terminated string (32 bytes)
	CallerID  [CallerIDSize]byte  // Null-terminated string (32 bytes)
	CalledID  [CalledIDSize]byte  // Null-terminated string (32 bytes)
	Timestamp uint32              // Unix timestamp (4 bytes)
}

// AudioPayload represents the audio packet payload
// Layout: [Sequence:4][AudioData:N]
type AudioPayload struct {
	Sequence  uint32 // Packet sequence number
	AudioData []byte // PCM audio data (variable length)
}

// ParsedPacket represents a fully parsed TLV packet
type ParsedPacket struct {
	Header    *Header
	Signaling *SignalingPayload // Only set for signaling packets
	Audio     *AudioPayload     // Only set for audio packets
}

// ParseHeader parses the 8-byte TLV packet header
func ParseHeader(data []byte) (*Header, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("header too short: expected %d bytes, got %d", HeaderSize, len(data))
	}

	header := &Header{
		PacketType: data[0],
		PacketLen:  binary.BigEndian.Uint16(data[1:3]),
		StreamID:   binary.BigEndian.Uint32(data[3:7]),
		Direction:  data[7],
	}

	return header, nil
}

// ParseSignalingPayload parses the 164-byte signaling packet payload
func ParseSignalingPayload(data []byte) (*SignalingPayload, error) {
	if len(data) < SignalingPayloadSize {
		return nil, fmt.Errorf("signaling payload too short: expected %d bytes, got %d", 
			SignalingPayloadSize, len(data))
	}

	payload := &SignalingPayload{}
	
	// Copy fixed-size byte arrays
	copy(payload.ChannelID[:], data[0:ChannelIDSize])
	copy(payload.Extension[:], data[ChannelIDSize:ChannelIDSize+ExtensionSize])
	copy(payload.CallerID[:], data[ChannelIDSize+ExtensionSize:ChannelIDSize+ExtensionSize+CallerIDSize])
	copy(payload.CalledID[:], data[ChannelIDSize+ExtensionSize+CallerIDSize:ChannelIDSize+ExtensionSize+CallerIDSize+CalledIDSize])
	
	// Parse timestamp (last 4 bytes)
	timestampOffset := ChannelIDSize + ExtensionSize + CallerIDSize + CalledIDSize
	payload.Timestamp = binary.BigEndian.Uint32(data[timestampOffset:timestampOffset+TimestampSize])

	return payload, nil
}

// ParseAudioPayload parses the audio packet payload (4-byte sequence + audio data)
func ParseAudioPayload(data []byte) (*AudioPayload, error) {
	if len(data) < AudioPayloadHeaderSize {
		return nil, fmt.Errorf("audio payload too short: expected at least %d bytes, got %d", 
			AudioPayloadHeaderSize, len(data))
	}

	payload := &AudioPayload{
		Sequence: binary.BigEndian.Uint32(data[0:4]),
	}

	// Copy audio data (remaining bytes after sequence)
	if len(data) > AudioPayloadHeaderSize {
		payload.AudioData = make([]byte, len(data)-AudioPayloadHeaderSize)
		copy(payload.AudioData, data[AudioPayloadHeaderSize:])
	}

	return payload, nil
}

// ParsePacket parses a complete TLV packet (header + payload)
func ParsePacket(data []byte) (*ParsedPacket, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("packet too short: expected at least %d bytes, got %d", HeaderSize, len(data))
	}

	// Parse header first
	header, err := ParseHeader(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	// Validate packet length matches actual data
	if int(header.PacketLen) != len(data) {
		return nil, fmt.Errorf("packet length mismatch: header says %d bytes, got %d bytes", 
			header.PacketLen, len(data))
	}

	// Validate header fields
	if err := ValidateHeader(header); err != nil {
		return nil, fmt.Errorf("invalid header: %w", err)
	}

	packet := &ParsedPacket{Header: header}
	payloadData := data[HeaderSize:]

	// Parse payload based on packet type
	switch header.PacketType {
	case PacketTypeSignaling:
		payload, err := ParseSignalingPayload(payloadData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse signaling payload: %w", err)
		}
		packet.Signaling = payload

	case PacketTypeAudio:
		payload, err := ParseAudioPayload(payloadData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse audio payload: %w", err)
		}
		packet.Audio = payload

	default:
		return nil, fmt.Errorf("unknown packet type: 0x%02x", header.PacketType)
	}

	return packet, nil
}

// ValidateHeader validates the packet header fields
func ValidateHeader(header *Header) error {
	if !IsValidPacketType(header.PacketType) {
		return fmt.Errorf("invalid packet type: 0x%02x", header.PacketType)
	}

	if !IsValidDirection(header.Direction) {
		return fmt.Errorf("invalid direction: 0x%02x", header.Direction)
	}

	if header.PacketLen < HeaderSize {
		return fmt.Errorf("packet length too small: %d (minimum %d)", header.PacketLen, HeaderSize)
	}

	// Validate expected payload sizes
	expectedPayloadSize := int(header.PacketLen) - HeaderSize
	switch header.PacketType {
	case PacketTypeSignaling:
		if expectedPayloadSize != SignalingPayloadSize {
			return fmt.Errorf("signaling packet payload size mismatch: expected %d, got %d", 
				SignalingPayloadSize, expectedPayloadSize)
		}
	case PacketTypeAudio:
		if expectedPayloadSize < AudioPayloadHeaderSize {
			return fmt.Errorf("audio packet payload too small: expected at least %d, got %d", 
				AudioPayloadHeaderSize, expectedPayloadSize)
		}
	}

	return nil
}

// IsValidPacketType checks if the packet type is valid
func IsValidPacketType(ptype uint8) bool {
	return ptype == PacketTypeSignaling || ptype == PacketTypeAudio
}

// IsValidDirection checks if the direction is valid
func IsValidDirection(dir uint8) bool {
	return dir == DirectionRX || dir == DirectionTX
}

// ExtractString extracts a null-terminated string from a fixed-size byte array
func ExtractString(buf []byte) string {
	// Find null terminator
	nullPos := len(buf)
	for i, b := range buf {
		if b == 0 {
			nullPos = i
			break
		}
	}
	return string(buf[:nullPos])
}

// GetChannelID extracts the channel ID as a string
func (s *SignalingPayload) GetChannelID() string {
	return ExtractString(s.ChannelID[:])
}

// GetExtension extracts the extension as a string
func (s *SignalingPayload) GetExtension() string {
	return ExtractString(s.Extension[:])
}

// GetCallerID extracts the caller ID as a string
func (s *SignalingPayload) GetCallerID() string {
	return ExtractString(s.CallerID[:])
}

// GetCalledID extracts the called ID as a string
func (s *SignalingPayload) GetCalledID() string {
	return ExtractString(s.CalledID[:])
}

// String returns a human-readable representation of the header
func (h *Header) String() string {
	var packetType, direction string
	
	switch h.PacketType {
	case PacketTypeSignaling:
		packetType = "Signaling"
	case PacketTypeAudio:
		packetType = "Audio"
	default:
		packetType = fmt.Sprintf("Unknown(0x%02x)", h.PacketType)
	}

	switch h.Direction {
	case DirectionRX:
		direction = "RX"
	case DirectionTX:
		direction = "TX"
	default:
		direction = fmt.Sprintf("Unknown(0x%02x)", h.Direction)
	}

	return fmt.Sprintf("Header{Type:%s, Len:%d, StreamID:%d, Direction:%s}", 
		packetType, h.PacketLen, h.StreamID, direction)
}

// String returns a human-readable representation of the signaling payload
func (s *SignalingPayload) String() string {
	return fmt.Sprintf("SignalingPayload{ChannelID:%q, Extension:%q, CallerID:%q, CalledID:%q, Timestamp:%d}",
		s.GetChannelID(), s.GetExtension(), s.GetCallerID(), s.GetCalledID(), s.Timestamp)
}

// String returns a human-readable representation of the audio payload
func (a *AudioPayload) String() string {
	return fmt.Sprintf("AudioPayload{Sequence:%d, AudioDataLen:%d}", a.Sequence, len(a.AudioData))
} 