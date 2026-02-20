#!/bin/bash

set -e

echo "🚀 Docker validation script starting..."

# Create test directory structure
echo "📂 Creating test directories..."
mkdir -p test/movies test/animations test/teleplays test/shows test/documentaries

# Copy test files if they exist
if [ -f "test.srt" ]; then
    echo "📝 Copying test subtitle file..."
    cp test.srt test/movies/demo-movie.srt
fi

# Build the image
echo "🔨 Building Docker image..."
docker-compose build

# Validate image contents
echo "🔍 Validating image contents..."
docker run --rm -it ctxtrans:latest ffmpeg -version || echo "❌ ffmpeg not properly installed"
docker run --rm -it ctxtrans:latest ffprobe -version || echo "❌ ffprobe not properly installed"
docker run --rm ctxtrans:latest sh -c "ffmpeg -hide_banner -decoders | grep -qi 'libaribb24.*arib_caption'" \
  && echo "✅ libaribb24 decoder available" \
  || echo "❌ libaribb24 decoder missing"

# Check environment variables
echo "⚙️  Checking environment variables..."
docker run --rm ctxtrans:latest env | grep LLM_

# Run tests if API key is provided
test_api_key() {
    if [ -n "$LLM_API_KEY" ]; then
        echo "🔑 LLM_API_KEY is set, running service test..."
        docker-compose up -d

        sleep 5

        # Check service status
        docker-compose ps

        # Get logs
        echo "📋 Service logs:"
        docker-compose logs --tail=50 ctxtrans

        # Cleanup
        docker-compose down
    else
        echo "ℹ️  LLM_API_KEY not set, skipping service runtime test."
        echo "   You can set LLM_API_KEY environment variable to test full service:"
        echo "   export LLM_API_KEY=your_actual_api_key"
        echo "   ./verify-docker.sh"
    fi
}

# Manual testing examples
echo "📋 Manual testing examples:"
echo "  1. Run interactive shell: docker-compose run --rm ctxtrans sh"
echo "  2. Check ffmpeg: docker-compose run --rm ctxtrans ffmpeg -version"
echo "  3. Check ARIB decoder: docker-compose run --rm ctxtrans sh -c 'ffmpeg -hide_banner -decoders | grep -i libaribb24'"
echo "  4. Check filesystem: docker-compose run --rm ctxtrans ls -la /movies"
echo "  5. Environment variables test: docker-compose run --rm ctxtrans printenv"

test_api_key

echo "✅ Docker validation complete!"
