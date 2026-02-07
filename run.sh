#!/bin/bash
set -euo pipefail

VIDEO_URL="https://storage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4"
VIDEO_FILE="bigbuckbunny.mp4"
CLIP_FILE="bigbuckbunny_30s.mp4"
DB_FILE="chunks.db"

# Build the ingest tool
echo "Building ingest tool..."
go build -o ingest .

# Download Big Buck Bunny if not present
if [ ! -f "$VIDEO_FILE" ]; then
    echo "Downloading Big Buck Bunny..."
    curl -L -o "$VIDEO_FILE" "$VIDEO_URL"
fi

# Extract a 30s clip from the middle for faster test runs
if [ ! -f "$CLIP_FILE" ]; then
    echo "Extracting 30s test clip from the middle..."
    ffmpeg -ss 270 -i "$VIDEO_FILE" -t 30 -c copy -v error "$CLIP_FILE"
fi

INPUT="${1:-$CLIP_FILE}"
echo "Ingesting $INPUT into $DB_FILE..."
echo "  ffmpeg -> yuv4mpegpipe (C444) -> ingest -> SQLite"

ffmpeg -i "$INPUT" -pix_fmt yuv444p -f yuv4mpegpipe -v error - | ./ingest "$DB_FILE"

echo ""
echo "Database size: $(du -h "$DB_FILE" | cut -f1)"
echo "Input size:    $(du -h "$INPUT" | cut -f1)"
