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

RUN go mod init github.com/amaumene/gomenarr && go mod tidy && go mod download

# Download Wire tool and its dependencies
RUN go get github.com/google/wire/cmd/wire@latest

# Generate Wire dependency injection code
RUN cd internal/infra && go run github.com/google/wire/cmd/wire

# Update dependencies after wire generation
RUN go mod tidy

# Build the binary with optimizations and proper static linking
RUN CGO_ENABLED=1 GOOS=linux go build \
    -tags 'osusergo netgo sqlite_omit_load_extension' \
    -ldflags="-w -s -linkmode external -extldflags '-static'" \
    -o gomenarr-server \
    ./cmd/server

# Verify static binary
RUN ldd gomenarr-server 2>&1 | grep -q "not a dynamic executable" || echo "Warning: Binary may not be fully static"

# Stage 2: Runtime
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# WORKDIR creates the directory structure in scratch
WORKDIR /data
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gomenarr-server /app/

# Copy example config (can be overridden via volume mount)
COPY --from=builder /build/config.example.yaml /app/config.example.yaml

# Expose HTTP port
EXPOSE 3000

# Declare volume
VOLUME /data

# Run the server
ENTRYPOINT ["/app/gomenarr-server"]
