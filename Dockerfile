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

FROM debian:bookworm-slim AS ffmpeg-builder

ARG FFMPEG_VERSION=7.1.1

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    libaribb24-dev \
    libbz2-dev \
    liblzma-dev \
    nasm \
    pkg-config \
    xz-utils \
    yasm \
    zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp/ffmpeg

RUN curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" -o ffmpeg.tar.xz \
    && mkdir src \
    && tar -xJf ffmpeg.tar.xz -C src --strip-components=1 \
    && cd src \
    && ./configure \
        --prefix=/opt/ffmpeg \
        --disable-debug \
        --disable-doc \
        --disable-ffplay \
        --enable-ffmpeg \
        --enable-ffprobe \
        --enable-gpl \
        --enable-version3 \
        --enable-libaribb24 \
    && make -j"$(nproc)" \
    && make install \
    && /opt/ffmpeg/bin/ffmpeg -hide_banner -decoders | grep -q "libaribb24.*arib_caption"

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libaribb24-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=ffmpeg-builder /opt/ffmpeg /opt/ffmpeg
COPY --from=go-builder /out/ctxtrans /app/ctxtrans
COPY --from=web-builder /web/dist /app/web

ENV PATH=/opt/ffmpeg/bin:${PATH}
ENV LD_LIBRARY_PATH=/opt/ffmpeg/lib

RUN ffmpeg -hide_banner -decoders | grep -q "libaribb24.*arib_caption"

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
