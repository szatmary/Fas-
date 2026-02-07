#!/bin/bash
set -euo pipefail

VIDEO_URL="https://storage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4"
VIDEO_FILE="bigbuckbunny.mp4"
DB_FILE="chunks.db"

# Build the ingest tool
echo "Building ingest tool..."
go build -o ingest .

# Download Big Buck Bunny if not present
if [ ! -f "$VIDEO_FILE" ]; then
    echo "Downloading Big Buck Bunny..."
    curl -L -o "$VIDEO_FILE" "$VIDEO_URL"
fi

echo "Ingesting video into $DB_FILE..."
echo "  ffmpeg -> yuv4mpegpipe (C444) -> ingest -> SQLite"

# Convert to YUV 4:4:4 Y4M and pipe directly into the ingest tool
ffmpeg -i "$VIDEO_FILE" -pix_fmt yuv444p -f yuv4mpegpipe -v error - | ./ingest "$DB_FILE"

echo ""
echo "Database size: $(du -h "$DB_FILE" | cut -f1)"
echo "Video size:    $(du -h "$VIDEO_FILE" | cut -f1)"
