#!/bin/bash
# KAE Lens Health Check Script
# Verifies all components are functioning properly

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔍 KAE LENS HEALTH CHECK"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 1. Check Go version
echo "📦 Checking Go installation..."
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed${NC}"
    exit 1
fi
GO_VERSION=$(go version | awk '{print $3}')
echo -e "${GREEN}✅ Go installed: $GO_VERSION${NC}"
echo

# 2. Check if Qdrant is running (both REST and gRPC)
echo "🗄️  Checking Qdrant connectivity..."
QDRANT_REST_URL="http://localhost:6333"
QDRANT_GRPC_HOST="localhost:6334"

# Check REST API
if curl -sf "$QDRANT_REST_URL/collections" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Qdrant REST API (port 6333) is reachable${NC}"
    
    # Get collections
    COLLECTIONS=$(curl -s "$QDRANT_REST_URL/collections" | jq -r '.result.collections[]? | "\(.name):\(.vectors_count // .points_count // 0)"' 2>/dev/null || echo "")
    if [ -n "$COLLECTIONS" ]; then
        echo "   Collections found:"
        while IFS=: read -r name count; do
            if [ -n "$name" ]; then
                echo "   - $name ($count points)"
            fi
        done <<< "$COLLECTIONS"
    else
        echo -e "${YELLOW}   ⚠️  No collections found or jq not installed${NC}"
    fi
else
    echo -e "${RED}❌ Qdrant REST API (port 6333) is NOT reachable${NC}"
    echo "   Run: docker ps -a | grep qdrant"
    echo "   If stopped, run: make qdrant-up"
fi
echo

# Check gRPC port (basic connectivity test)
if timeout 1 bash -c "cat < /dev/null > /dev/tcp/localhost/6334" 2>/dev/null; then
    echo -e "${GREEN}✅ Qdrant gRPC (port 6334) is listening${NC}"
else
    echo -e "${RED}❌ Qdrant gRPC (port 6334) is NOT reachable${NC}"
    echo "   Lens requires gRPC port 6334 for vector operations"
fi
echo

# 3. Check environment configuration
echo "🔑 Checking environment configuration..."
if [ -f "../.env" ]; then
    echo -e "${GREEN}✅ Found .env in parent directory (KAE root)${NC}"
    
    # Check for required API keys (don't print values)
    if grep -q "^OPENROUTER_API_KEY=." ../.env 2>/dev/null; then
        echo -e "${GREEN}✅ OPENROUTER_API_KEY is set${NC}"
    else
        echo -e "${YELLOW}⚠️  OPENROUTER_API_KEY not set (required for reasoning)${NC}"
    fi
    
    if grep -q "^OPENAI_API_KEY=." ../.env 2>/dev/null; then
        echo -e "${GREEN}✅ OPENAI_API_KEY is set${NC}"
    else
        echo -e "${YELLOW}⚠️  OPENAI_API_KEY not set (optional, used for embeddings)${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  No .env file found in parent directory${NC}"
    echo "   Lens will use config/lens.yaml values"
fi

# Check lens.yaml
if [ -f "config/lens.yaml" ]; then
    echo -e "${GREEN}✅ config/lens.yaml exists${NC}"
    
    # Parse key config values
    KNOWLEDGE_COLLECTION=$(grep "knowledge_collection:" config/lens.yaml | awk '{print $2}' | tr -d '"' | head -1)
    FINDINGS_COLLECTION=$(grep "findings_collection:" config/lens.yaml | awk '{print $2}' | tr -d '"' | head -1)
    POLL_INTERVAL=$(grep "poll_interval_seconds:" config/lens.yaml | awk '{print $2}' | head -1)
    BATCH_SIZE=$(grep "batch_size:" config/lens.yaml | awk '{print $2}' | head -1)
    
    echo "   Knowledge collection: $KNOWLEDGE_COLLECTION"
    echo "   Findings collection: $FINDINGS_COLLECTION"
    echo "   Poll interval: ${POLL_INTERVAL}s"
    echo "   Batch size: $BATCH_SIZE"
else
    echo -e "${RED}❌ config/lens.yaml not found${NC}"
fi
echo

# 4. Check if binary exists and is up to date
echo "🔨 Checking binary status..."
if [ -f "lens" ]; then
    BINARY_TIME=$(stat -c %Y lens 2>/dev/null || stat -f %m lens 2>/dev/null || echo "0")
    SOURCE_TIME=$(find internal cmd -type f -name "*.go" -exec stat -c %Y {} \; 2>/dev/null | sort -n | tail -1 || find internal cmd -type f -name "*.go" -exec stat -f %m {} \; 2>/dev/null | sort -n | tail -1 || echo "0")
    
    if [ "$SOURCE_TIME" -gt "$BINARY_TIME" ]; then
        echo -e "${YELLOW}⚠️  Binary is older than source files - rebuild recommended${NC}"
        echo "   Run: make build"
    else
        echo -e "${GREEN}✅ Binary is up to date${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  Binary not found - build required${NC}"
    echo "   Run: make build"
fi
echo

# 5. Test Go build (without running)
echo "🏗️  Testing Go build..."
if go build -o /tmp/lens-test ./cmd/lens > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Go build successful${NC}"
    rm -f /tmp/lens-test
else
    echo -e "${RED}❌ Go build failed${NC}"
    echo "   Run: go build ./cmd/lens"
    exit 1
fi
echo

# 6. Run Go tests
echo "🧪 Running tests..."
TEST_OUTPUT=$(go test ./internal/lens/... -v 2>&1)
if echo "$TEST_OUTPUT" | grep -q "PASS"; then
    PASS_COUNT=$(echo "$TEST_OUTPUT" | grep -c "^=== RUN" || echo "0")
    echo -e "${GREEN}✅ All tests passed ($PASS_COUNT test cases)${NC}"
else
    echo -e "${RED}❌ Tests failed${NC}"
    echo "$TEST_OUTPUT"
    exit 1
fi
echo

# 7. Check Qdrant collections detail
echo "🗂️  Checking Qdrant collection schemas..."
if curl -sf "$QDRANT_REST_URL/collections" > /dev/null 2>&1; then
    # Check kae_chunks collection
    if curl -sf "$QDRANT_REST_URL/collections/kae_chunks" > /dev/null 2>&1; then
        CHUNK_INFO=$(curl -s "$QDRANT_REST_URL/collections/kae_chunks")
        CHUNK_COUNT=$(echo "$CHUNK_INFO" | jq -r '.result.points_count // 0' 2>/dev/null || echo "0")
        UNPROCESSED=$(curl -s "$QDRANT_REST_URL/collections/kae_chunks/points/scroll" \
            -H "Content-Type: application/json" \
            -d '{"limit":1000,"filter":{"must_not":[{"key":"lens_processed","match":{"value":true}}]},"with_payload":false}' \
            | jq -r '.result.points | length' 2>/dev/null || echo "unknown")
        echo -e "${GREEN}✅ kae_chunks collection exists${NC}"
        echo "   Total points: $CHUNK_COUNT"
        echo "   Unprocessed: $UNPROCESSED"
    else
        echo -e "${YELLOW}⚠️  kae_chunks collection not found (will be created by KAE)${NC}"
    fi
    echo
    
    # Check kae_lens_findings collection
    if curl -sf "$QDRANT_REST_URL/collections/kae_lens_findings" > /dev/null 2>&1; then
        FINDING_INFO=$(curl -s "$QDRANT_REST_URL/collections/kae_lens_findings")
        FINDING_COUNT=$(echo "$FINDING_INFO" | jq -r '.result.points_count // 0' 2>/dev/null || echo "0")
        echo -e "${GREEN}✅ kae_lens_findings collection exists${NC}"
        echo "   Total findings: $FINDING_COUNT"
        
        # Count by type
        if [ "$FINDING_COUNT" -gt "0" ]; then
            FINDINGS_BY_TYPE=$(curl -s "$QDRANT_REST_URL/collections/kae_lens_findings/points/scroll" \
                -H "Content-Type: application/json" \
                -d '{"limit":1000,"with_payload":true}' \
                | jq -r '.result.points[]?.payload.type.string_value' 2>/dev/null | sort | uniq -c | sort -rn || echo "")
            if [ -n "$FINDINGS_BY_TYPE" ]; then
                echo "   By type:"
                echo "$FINDINGS_BY_TYPE" | while read count type; do
                    echo "     $type: $count"
                done
            fi
        fi
    else
        echo -e "${YELLOW}⚠️  kae_lens_findings collection not found (will be created on first write)${NC}"
    fi
fi
echo

# 8. Check web dashboard accessibility
echo "🌐 Checking web dashboard..."
WEB_PORT=$(grep "port:" config/lens.yaml | awk '{print $2}' | head -1)
if [ -z "$WEB_PORT" ]; then
    WEB_PORT=8080
fi

if timeout 1 bash -c "cat < /dev/null > /dev/tcp/localhost/$WEB_PORT" 2>/dev/null; then
    echo -e "${GREEN}✅ Web dashboard port $WEB_PORT is in use (Lens may be running)${NC}"
else
    echo -e "${YELLOW}⚠️  Web dashboard port $WEB_PORT is available (Lens not running)${NC}"
fi
echo

# 9. Summary
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 HEALTH CHECK SUMMARY"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo

# Count critical issues
CRITICAL_ISSUES=0
if ! command -v go &> /dev/null; then ((CRITICAL_ISSUES++)); fi
if ! curl -sf "$QDRANT_REST_URL/collections" > /dev/null 2>&1; then ((CRITICAL_ISSUES++)); fi
if ! timeout 1 bash -c "cat < /dev/null > /dev/tcp/localhost/6334" 2>/dev/null; then ((CRITICAL_ISSUES++)); fi

if [ $CRITICAL_ISSUES -eq 0 ]; then
    echo -e "${GREEN}✅ ALL CRITICAL SYSTEMS OPERATIONAL${NC}"
    echo ""
    echo "Ready to run:"
    echo "  make run-lens     # Start Lens with TUI + web dashboard"
    echo "  make run-kae      # Start KAE ingestion (separate terminal)"
    echo ""
    echo "Web dashboard will be at http://localhost:$WEB_PORT"
    exit 0
else
    echo -e "${RED}❌ $CRITICAL_ISSUES CRITICAL ISSUES FOUND${NC}"
    echo ""
    echo "Fix the issues above before running Lens."
    exit 1
fi
