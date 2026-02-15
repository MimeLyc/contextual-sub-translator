# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main cmd/main.go

# Final stage
FROM debian:bullseye-slim

# Install ca-certificates for HTTPS and ffmpeg/ffprobe
RUN apt-get update && apt-get install -y \
    ca-certificates \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Set environment variables with defaults
ENV LLM_API_URL=https://openrouter.ai/api/v1
ENV LLM_MODEL=openai/gpt-3.5-turbo
ENV LLM_MAX_TOKENS=8000
ENV LLM_TEMPERATURE=0.7
ENV LLM_TIMEOUT=30
ENV LLM_SITE_URL=""
ENV LLM_APP_NAME=""

ENV MOVIE_DIR=/movies
ENV ANIMATION_DIR=/animations
ENV TELEPLAY_DIR=/teleplays
ENV SHOW_DIR=/shows
ENV DOCUMENTARY_DIR=/documentaries

ENV PUID=1000
ENV PGID=1000
ENV TZ=UTC
ENV ZONE=local

ENV CRON_EXPR="0 0 * * *"

# Create volume mount points
VOLUME ["/movies", "/animations", "/teleplays", "/shows", "/documentaries"]

# Run the application
CMD ["./main"]

