# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /temporal-profiler \
    ./cmd/temporal-profiler

# Runtime stage
FROM alpine:3.21

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
