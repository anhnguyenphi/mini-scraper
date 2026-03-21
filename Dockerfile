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

RUN pip install --no-cache-dir 'markitdown[all]'

COPY --from=builder /scraper-ai /usr/local/bin/scraper-ai

WORKDIR /app
COPY static/ /app/static/

EXPOSE 8080

ENTRYPOINT ["scraper-ai"]
CMD ["serve"]
