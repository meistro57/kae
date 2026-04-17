#!/bin/bash
# Quick KAE Lens status check - run anytime to see current state

QDRANT="http://localhost:6333"

echo "🔍 KAE LENS STATUS"
echo ""

# Qdrant status
if curl -sf "$QDRANT/collections" > /dev/null 2>&1; then
    echo "✅ Qdrant: Running"
    
    # Get collection stats
    CHUNKS=$(curl -s "$QDRANT/collections/kae_chunks" 2>/dev/null | jq -r '.result.points_count // 0')
    FINDINGS=$(curl -s "$QDRANT/collections/kae_lens_findings" 2>/dev/null | jq -r '.result.points_count // 0')
    
    # Get unprocessed count
    UNPROCESSED=$(curl -s "$QDRANT/collections/kae_chunks/points/scroll" \
        -H "Content-Type: application/json" \
        -d '{"limit":10000,"filter":{"must_not":[{"key":"lens_processed","match":{"value":true}}]},"with_payload":false}' \
        2>/dev/null | jq -r '.result.points | length' || echo "?")
    
    echo "  📦 kae_chunks: $CHUNKS points"
    echo "  🔍 kae_lens_findings: $FINDINGS findings"
    echo "  ⏳ Unprocessed: $UNPROCESSED points"
else
    echo "❌ Qdrant: Not running"
    echo "   Run: make qdrant-up"
fi

echo ""

# Lens process status
if pgrep -x "lens" > /dev/null; then
    echo "✅ Lens: Running (PID: $(pgrep -x lens))"
    
    # Check if web dashboard is accessible
    if timeout 1 bash -c "cat < /dev/null > /dev/tcp/localhost/8080" 2>/dev/null; then
        echo "  🌐 Dashboard: http://localhost:8080"
    fi
else
    echo "⏸️  Lens: Not running"
    echo "   Run: make run-lens"
fi

echo ""

# KAE process status
if pgrep -x "kae" > /dev/null; then
    echo "✅ KAE: Running (PID: $(pgrep -x kae))"
else
    echo "⏸️  KAE: Not running"
    if [ "$CHUNKS" = "0" ] || [ -z "$CHUNKS" ]; then
        echo "   No data ingested yet. Run: make run-kae"
    fi
fi

echo ""

# Recent findings
if [ -n "$FINDINGS" ] && [ "$FINDINGS" -gt 0 ]; then
    echo "📊 Recent Findings (last 5):"
    curl -s "$QDRANT/collections/kae_lens_findings/points/scroll" \
        -H "Content-Type: application/json" \
        -d '{"limit":5,"with_payload":true,"order_by":{"key":"created_at","direction":"desc"}}' \
        2>/dev/null | jq -r '.result.points[]? | "  • \(.payload.type // "finding") (confidence: \((.payload.confidence // 0) | . * 100 | round / 100))"' \
        || echo "  (Unable to fetch)"
fi

echo ""
