FROM node:20-alpine AS web-builder

WORKDIR /web
COPY web/package*.json ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS go-builder

WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /out/ctxtrans ./cmd/main.go

FROM debian:bullseye-slim AS ffmpeg-builder

ARG FFMPEG_VERSION=6.1.1
WORKDIR /tmp/ffmpeg-build

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    build-essential \
    curl \
    libaribb24-dev \
    nasm \
    pkg-config \
    xz-utils \
    yasm \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    curl -fsSLO "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz"; \
    tar -xJf "ffmpeg-${FFMPEG_VERSION}.tar.xz"; \
    cd "ffmpeg-${FFMPEG_VERSION}"; \
    ./configure \
      --prefix=/usr/local \
      --disable-debug \
      --disable-doc \
      --disable-ffplay \
      --disable-static \
      --enable-shared \
      --enable-gpl \
      --enable-version3 \
      --enable-libaribb24; \
    make -j"$(nproc)"; \
    make install

FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    libaribb24-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=ffmpeg-builder /usr/local/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=ffmpeg-builder /usr/local/bin/ffprobe /usr/local/bin/ffprobe
COPY --from=ffmpeg-builder /usr/local/lib/ /usr/local/lib/
RUN ldconfig

COPY --from=go-builder /out/ctxtrans /app/ctxtrans
COPY --from=web-builder /web/dist /app/web

ENV LLM_API_KEY=""
ENV LLM_API_URL=https://openrouter.ai/api/v1
ENV LLM_MODEL=openai/gpt-3.5-turbo
ENV LLM_MAX_TOKENS=8000
ENV LLM_TEMPERATURE=0.7
ENV LLM_TIMEOUT=30
ENV SEARCH_API_KEY=""
ENV SEARCH_API_URL=https://api.tavily.com/search
ENV AGENT_MAX_ITERATIONS=10
ENV AGENT_BUNDLE_CONCURRENCY=1
ENV LOG_LEVEL=INFO
ENV HTTP_ADDR=:8080
ENV UI_STATIC_DIR=/app/web
ENV UI_ENABLE=true

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

VOLUME ["/movies", "/animations", "/teleplays", "/shows", "/documentaries"]
EXPOSE 8080

CMD ["/app/ctxtrans"]
