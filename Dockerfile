# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /scraper-ai .

# Runtime stage
FROM python:3.12-slim

# Install system dependencies for Playwright
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl wget gnupg ca-certificates \
    libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 \
    libcups2 libdrm2 libxkbcommon0 libxcomposite1 \
    libxdamage1 libxrandr2 libgbm1 libpango-1.0-0 \
    libcairo2 libasound2 libxshmfence1 libx11-xcb1 \
    fonts-liberation xdg-utils \
    && rm -rf /var/lib/apt/lists/*

# Install Python packages
RUN pip install --no-cache-dir 'markitdown[all]'

# Install crawl4ai
RUN pip install -U --no-cache-dir crawl4ai

# Setup crawl4ai (installs Playwright browsers)
RUN crawl4ai-setup

COPY --from=builder /scraper-ai /usr/local/bin/scraper-ai

WORKDIR /app
COPY static/ /app/static/
COPY scripts/ /app/scripts/

EXPOSE 8080

ENTRYPOINT ["scraper-ai"]
CMD ["serve"]
