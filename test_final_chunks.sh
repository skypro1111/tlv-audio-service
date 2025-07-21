#!/bin/bash

echo "🧪 Testing Final Chunk Processing"
echo "================================="

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}This test demonstrates final chunk processing when streams end${NC}"
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

# Check transcription server (read from config)
echo "2. Checking transcription endpoint..."
TRANSCRIPTION_ENDPOINT=$(grep "endpoint:" configs/debug_config.yaml | cut -d'"' -f2)
echo "   Endpoint: $TRANSCRIPTION_ENDPOINT"

if curl -s "$TRANSCRIPTION_ENDPOINT" -X POST --max-time 5 > /dev/null 2>&1; then
    echo -e "   ✅ ${GREEN}Transcription endpoint is reachable${NC}"
elif [[ "$TRANSCRIPTION_ENDPOINT" == *"localhost"* ]]; then
    echo -e "   ❌ ${RED}Local transcription server not found. Please start:${NC}"
    echo "      go run test_transcription_server.go"
    exit 1
else
    echo -e "   ⚠️  ${YELLOW}External endpoint (cannot verify, but configured)${NC}"
fi

echo ""
echo "📊 Monitoring Instructions:"
echo "========================="
echo ""
echo -e "${YELLOW}To test final chunk processing, watch the logs when a real call ends:${NC}"
echo ""
echo "📋 Expected logs when stream ends:"
echo "-----------------------------------"
echo '{
  "level": "INFO",
  "msg": "Finalizing stream session",
  "stream_id": 123456,
  "caller_id": "380936628576",
  "duration": "45.2s",
  "chunks_generated": 8,
  "chunks_sent": 8,
  "chunks_successful": 8
}'
echo ""
echo '{
  "level": "INFO", 
  "msg": "Final RX chunk generated on stream end",
  "stream_id": 123456,
  "chunk_id": "123456_0_1642781234",
  "duration": 2.3,
  "samples": 18400
}'
echo ""
echo '{
  "level": "INFO",
  "msg": "Final TX chunk generated on stream end", 
  "stream_id": 123456,
  "chunk_id": "123456_1_1642781235",
  "duration": 1.8,
  "samples": 14400
}'
echo ""

echo "🔍 Real-time Monitoring Commands:"
echo "================================="
echo ""
echo -e "${GREEN}1. Watch live metrics:${NC}"
echo "   curl -s http://localhost:8080/metrics | grep 'tlv_audio_chunks_generated\\|tlv_transcription_requests'"
echo ""
echo -e "${GREEN}2. Monitor active streams:${NC}"
echo "   curl -s http://localhost:8080/streams | jq '.streams[] | {stream_id, caller_id, chunks_generated, chunks_sent}'"
echo ""
echo -e "${GREEN}3. Check current session stats:${NC}"
echo "   curl -s http://localhost:8080/streams | jq '.total_streams'"
echo ""

echo "🎯 To Trigger Final Chunk Processing:"
echo "====================================="
echo ""
echo "Final chunks are automatically sent when:"
echo "  • Real phone call ends naturally"
echo "  • Stream timeout occurs (60 seconds of inactivity)"
echo "  • Service is gracefully shut down (Ctrl+C)"
echo ""
echo -e "${YELLOW}💡 Tip: Make a short test call and hang up to see final chunks!${NC}"

echo ""
echo "📈 Current Metrics:"
echo "=================="
curl -s http://localhost:8080/metrics | grep "tlv_audio_chunks_generated\|tlv_transcription_requests" | head -4

echo ""
echo "📱 Active Streams:"
echo "=================="
curl -s http://localhost:8080/streams | jq '{total_streams: .total_streams, active_streams: [.streams[] | {stream_id, caller_id, duration, chunks_generated}]}'

echo ""
echo -e "${GREEN}✅ Test environment ready!${NC}"
echo -e "   Monitor logs and make test calls to see final chunk processing in action." 