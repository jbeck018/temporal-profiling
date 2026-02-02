# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev) -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo none) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /temporal-profiler \
    ./cmd/temporal-profiler

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 profiler && \
    adduser -u 1000 -G profiler -s /bin/sh -D profiler

WORKDIR /app

# Copy binary from builder
COPY --from=builder /temporal-profiler /app/temporal-profiler

# Create log directory
RUN mkdir -p /var/log/temporal-profiler && \
    chown -R profiler:profiler /var/log/temporal-profiler

USER profiler

# Expose ports
# 7234 - Proxy (SDKs connect here)
# 8080 - Admin (health, metrics, readiness)
EXPOSE 7234 8080

ENTRYPOINT ["/app/temporal-profiler"]
CMD ["start"]
