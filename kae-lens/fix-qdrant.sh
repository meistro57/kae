#!/bin/bash
# Fix Qdrant gRPC connectivity issue

echo "🔧 Checking Qdrant Docker container..."

CONTAINER_ID=$(docker ps --filter "name=qdrant" --format "{{.ID}}" | head -1)

if [ -z "$CONTAINER_ID" ]; then
    echo "❌ No Qdrant container running"
    echo "   Starting Qdrant..."
    make qdrant-up
    sleep 5
    CONTAINER_ID=$(docker ps --filter "name=qdrant" --format "{{.ID}}" | head -1)
fi

echo "✅ Qdrant container: $CONTAINER_ID"
echo ""

# Check port mappings
echo "📡 Port mappings:"
docker port $CONTAINER_ID

echo ""
echo "🔍 Testing connectivity..."

# Test REST (6333)
if curl -sf http://localhost:6333/collections > /dev/null 2>&1; then
    echo "✅ REST API (6333): Reachable"
else
    echo "❌ REST API (6333): NOT reachable"
fi

# Test gRPC (6334)
if timeout 1 bash -c "cat < /dev/null > /dev/tcp/localhost/6334" 2>/dev/null; then
    echo "✅ gRPC (6334): Reachable"
else
    echo "❌ gRPC (6334): NOT reachable"
    echo ""
    echo "⚠️  gRPC port may not be exposed. Recreating container..."
    
    docker stop $CONTAINER_ID
    docker rm $CONTAINER_ID
    
    echo "Starting Qdrant with both ports exposed..."
    docker run -d --name kae-qdrant \
        -p 6333:6333 \
        -p 6334:6334 \
        -v $(pwd)/qdrant_storage:/qdrant/storage:z \
        qdrant/qdrant:v1.17.1
    
    echo "Waiting for Qdrant to start..."
    sleep 5
    
    if timeout 1 bash -c "cat < /dev/null > /dev/tcp/localhost/6334" 2>/dev/null; then
        echo "✅ gRPC port now reachable!"
    else
        echo "❌ Still having issues. Check Docker logs:"
        echo "   docker logs kae-qdrant"
    fi
fi

echo ""
echo "📊 Current state:"
./status.sh
