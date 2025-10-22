# Dockerfile for Gomenarr
# Multi-stage build for minimal final image

# Stage 1: Build
FROM golang:alpine AS builder

RUN apk add --no-cache build-base

# Set working directory
WORKDIR /build

# Copy source code
COPY . .

RUN rm go.mod && rm go.sum

RUN go mod init github.com/amaumene/gomenarr && go mod tidy

# Generate Wire dependency injection code
RUN cd internal/infra && go run github.com/google/wire/cmd/wire

# Build the binary with optimizations
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o gomenarr-server \
    ./cmd/server

# Stage 2: Runtime
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

#COPY --from=builder /usr/lib/libsqlite3.a /usr/lib/libsqlite3.a

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gomenarr-server /app/

# Copy migrations
COPY --from=builder /build/migrations /app/migrations/

# Copy example config (can be overridden via volume mount)
COPY --from=builder /build/config.example.yaml /app/config.example.yaml

# Expose HTTP port
EXPOSE 3000

VOLUME /data

# Run the server
ENTRYPOINT ["/app/gomenarr-server"]
