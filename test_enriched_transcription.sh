#!/bin/bash

echo "🧪 Testing Enriched Transcription Data"
echo "======================================"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}This test demonstrates the comprehensive data sent to transcription API${NC}"
echo ""

# Check if service is running
echo "1. Checking if TLV service is running..."
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "   ✅ ${GREEN}Service is running${NC}"
else
    echo -e "   ❌ ${RED}Service is not running. Please start:${NC}"
    echo "      go run cmd/server/main.go --config=configs/debug_config.yaml"
    exit 1
fi

echo ""
echo "📋 Enhanced Transcription Request Fields:"
echo "========================================"
echo ""
echo -e "${YELLOW}📊 Basic Information:${NC}"
echo "  • request_id        - Unique request identifier"
echo "  • chunk_id          - Audio chunk identifier"
echo "  • stream_id         - Stream session ID"
echo "  • duration          - Chunk duration in seconds"
echo "  • confidence        - VAD confidence score"
echo "  • start_seq/end_seq - Audio packet sequence range"
echo ""
echo -e "${YELLOW}📞 Call Context:${NC}"
echo "  • caller_id         - Calling party number"
echo "  • called_id         - Called party number"
echo "  • channel_id        - SIP/PJSIP channel identifier"
echo "  • extension         - Dialed extension/number"
echo ""
echo -e "${YELLOW}⏱️ Session Context:${NC}"
echo "  • session_start_time - When call started"
echo "  • session_duration   - Total call duration so far"
echo "  • chunk_start_time   - When this chunk started"
echo "  • chunk_end_time     - When this chunk ended"
echo ""
echo -e "${YELLOW}🎧 Audio Technical Details:${NC}"
echo "  • sample_rate       - Audio sample rate (8000 Hz)"
echo "  • format            - Audio format (wav/flac)"
echo "  • direction         - RX (0) or TX (1)"
echo "  • language          - Target language (uk)"
echo ""
echo -e "${YELLOW}🛠️ Service Metadata:${NC}"
echo "  • service_name      - tlv-audio-service"
echo "  • service_version   - 1.0.0"
echo "  • request_timestamp - When request was created"
echo ""

echo "🔍 Expected Transcription Server Logs:"
echo "======================================"
echo ""
echo -e "${GREEN}When a chunk is sent, you'll see:${NC}"
echo ""
cat << 'EOF'
🎤 TRANSCRIPTION REQUEST RECEIVED:
  ═══════════════════════════════════
  📊 Basic Info:
    Request ID: 1753144936_0_1753100248_1642781234567890
    Chunk ID: 1753144936_0_1753100248
    Stream ID: 1753144936
    Duration: 2.450 seconds
    Confidence: 0.850
  ═══════════════════════════════════
  📞 Call Context:
    Caller ID: 380936628576
    Called ID: 380931755514
    Channel ID: PJSIP/trunk_asterisk112-00000005
    Extension: 380931755514
  ═══════════════════════════════════
  ⏱️ Session Context:
    Session Start: 2025-07-21T14:15:30Z
    Session Duration: 45.234
    Chunk Start: 2025-07-21T14:16:12Z
  ═══════════════════════════════════
  🎧 Audio Info:
    Filename: 1753144936_0_1753100248.wav
    Audio Size: 19600 bytes
    Content-Type: audio/wav
    Language: uk
  ═══════════════════════════════════
  🛠️ Service Info:
    Service: tlv-audio-service v1.0.0
EOF

echo ""
echo "📈 Enhanced Service Logs:"
echo "========================"
echo ""
echo -e "${GREEN}Service logs now include:${NC}"
echo ""
cat << 'EOF'
{
  "level": "INFO",
  "msg": "Sending enriched chunk for transcription",
  "stream_id": 1753144936,
  "chunk_id": "1753144936_0_1753100248",
  "request_id": "1753144936_0_1753100248_1642781234567890",
  "caller_id": "380936628576",
  "called_id": "380931755514", 
  "channel_id": "PJSIP/trunk_asterisk112-00000005",
  "extension": "380931755514",
  "duration": 2.45,
  "session_duration": 45.234,
  "format": "wav",
  "language": "uk",
  "audio_data_size": 19600
}
EOF

echo ""
echo "🎯 Benefits of Enhanced Data:"
echo "============================"
echo ""
echo "✅ Complete call context for better transcription accuracy"
echo "✅ Session metadata for analytics and debugging"
echo "✅ Technical details for quality control"
echo "✅ Unique identifiers for tracking and correlation"
echo "✅ Temporal context for conversation flow analysis"
echo ""
echo -e "${YELLOW}💡 This comprehensive data enables advanced AI processing and analytics!${NC}"

echo ""
echo "📱 Current Active Streams:"
echo "========================="
curl -s http://localhost:8080/streams | jq '.streams[] | {stream_id, caller_id, called_id, channel_id, extension, duration}'

echo ""
echo -e "${GREEN}✅ Ready to test with real calls!${NC}"
echo -e "   Make a call and watch the enriched transcription data flow." 